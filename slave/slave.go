package slave

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/xray"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type Slave struct {
	MasterUrl string
	Secret    string
	process   *xray.Process
	xrayAPI   *xray.XrayAPI
	slaveId   int
}

func NewSlave(masterUrl, secret string) *Slave {
	return &Slave{
		MasterUrl: masterUrl,
		Secret:    secret,
	}
}

func Run(masterUrl, secret string) {
	slave := NewSlave(masterUrl, secret)
	slave.Run()
}

func (s *Slave) Run() {
	logger.Info("Starting Slave...")

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	go func() {
		for {
			s.connectAndLoop()
			logger.Info("Disconnected, reconnecting in 5s...")
			time.Sleep(5 * time.Second)
		}
	}()

	<-interrupt
	if s.process != nil {
		s.process.Stop()
	}
	logger.Info("Slave stopped")
}

func (s *Slave) connectAndLoop() {
	// Build the URL - check if path already contains the endpoint
	baseUrl := s.MasterUrl
	var url string
	
	// If the URL already has the connect path, just append the secret
	if strings.Contains(baseUrl, "/panel/api/slave/connect") {
		if strings.Contains(baseUrl, "?") {
			url = baseUrl + "&secret=" + s.Secret
		} else {
			url = baseUrl + "?secret=" + s.Secret
		}
	} else {
		// Need to append the path
		if baseUrl[len(baseUrl)-1] != '/' {
			baseUrl += "/"
		}
		url = fmt.Sprintf("%spanel/api/slave/connect?secret=%s", baseUrl, s.Secret)
	}
	logger.Infof("Connecting to %s", url)
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		logger.Error("Connect failed:", err)
		return
	}
	defer c.Close()
	logger.Info("Connected to Master")

	done := make(chan struct{})

	// heartbeat / stats loop
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		trafficTicker := time.NewTicker(10 * time.Second)
		certTicker := time.NewTicker(60 * time.Minute) // Check certs every hour
		defer ticker.Stop()
		defer trafficTicker.Stop()
		defer certTicker.Stop()
		
		// Send certs immediately on connect
		if certData := s.collectCertificates(); certData != "" {
			if err := c.WriteMessage(websocket.TextMessage, []byte(certData)); err != nil {
				logger.Error("Failed to send initial certificates:", err)
			}
		}
		
		for {
			select {
			case <-ticker.C:
				stats := s.collectStats()
				if err := c.WriteMessage(websocket.TextMessage, []byte(stats)); err != nil {
					close(done)
					return
				}
			case <-trafficTicker.C:
				// Send traffic stats
				if trafficData := s.collectTrafficStats(); trafficData != "" {
					if err := c.WriteMessage(websocket.TextMessage, []byte(trafficData)); err != nil {
						logger.Error("Failed to send traffic stats:", err)
					}
				}
			case <-certTicker.C:
				// Send certificate info periodically
				if certData := s.collectCertificates(); certData != "" {
					if err := c.WriteMessage(websocket.TextMessage, []byte(certData)); err != nil {
						logger.Error("Failed to send certificates:", err)
					}
				}
			case <-done:
				return
			}
		}
	}()

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			logger.Error("Read error:", err)
			close(done)
			break
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		typeStr, ok := msg["type"].(string)
		if !ok {
			continue
		}

		switch typeStr {
		case "update_config_full":
			configStr, ok := msg["config"].(string)
			if !ok {
				logger.Error("Invalid config format")
				continue
			}

			var xrayConfig xray.Config
			if err := json.Unmarshal([]byte(configStr), &xrayConfig); err != nil {
				logger.Error("Failed to unmarshal config:", err)
				continue
			}

			s.applyFullConfig(&xrayConfig)

		case "restart_xray":
			// Handle Xray Restart Request
			s.restartXray()
		}
	}
}

func (s *Slave) collectStats() string {
	v, _ := mem.VirtualMemory()
	c, _ := cpu.Percent(0, false)
	cpuVal := 0.0
	if len(c) > 0 {
		cpuVal = c[0]
	}

	ip := s.getPublicIP()
	return fmt.Sprintf(`{"cpu": %.2f, "mem": %.2f, "address": "%s"}`, cpuVal, v.UsedPercent, ip)
}

// getPublicIP fetches the public IP address of this slave
func (s *Slave) getPublicIP() string {
	// Try multiple services for reliability
	services := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for _, url := range services {
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		ip := strings.TrimSpace(string(body))
		if ip != "" {
			return ip
		}
	}

	return ""
}

func (s *Slave) collectTrafficStats() string {
	if s.xrayAPI == nil || s.process == nil || !s.process.IsRunning() {
		logger.Debug("collectTrafficStats: Xray API or process not ready")
		return ""
	}
	
	traffics, clientTraffics, err := s.xrayAPI.GetTraffic(true)
	if err != nil {
		logger.Debug("Failed to get traffic stats:", err)
		return ""
	}
	
	logger.Debugf("collectTrafficStats: Got %d inbound/outbound entries, %d user entries", len(traffics), len(clientTraffics))
	
	if len(traffics) == 0 && len(clientTraffics) == 0 {
		return ""
	}
	
	// Build traffic stats message with inbound, outbound and user stats
	type TrafficData struct {
		Type          string                       `json:"type"`
		Inbounds      map[string]map[string]int64  `json:"inbounds"`
		Outbounds     map[string]map[string]int64  `json:"outbounds"`
		Users         []map[string]interface{}     `json:"users"`
		OnlineClients []string                     `json:"online_clients"`
	}
	
	data := TrafficData{
		Type:          "traffic_stats",
		Inbounds:      make(map[string]map[string]int64),
		Outbounds:     make(map[string]map[string]int64),
		Users:         make([]map[string]interface{}, 0),
		OnlineClients: make([]string, 0),
	}
	
	// Collect inbound and outbound traffic
	for _, traffic := range traffics {
		if traffic.IsInbound && traffic.Tag != "api" {
			data.Inbounds[traffic.Tag] = map[string]int64{
				"uplink":   traffic.Up,
				"downlink": traffic.Down,
			}
		} else if traffic.IsOutbound {
			data.Outbounds[traffic.Tag] = map[string]int64{
				"uplink":   traffic.Up,
				"downlink": traffic.Down,
			}
		}
	}
	
	// Collect user traffic and online clients
	for _, clientTraffic := range clientTraffics {
		if clientTraffic.Email != "" {
			// Only include user in traffic data if they have actual traffic this period
			if clientTraffic.Up > 0 || clientTraffic.Down > 0 {
				data.Users = append(data.Users, map[string]interface{}{
					"email":    clientTraffic.Email,
					"uplink":   clientTraffic.Up,
					"downlink": clientTraffic.Down,
				})
				data.OnlineClients = append(data.OnlineClients, clientTraffic.Email)
			}
		}
	}
	
	// Always send traffic stats message, even if no traffic occurred this period
	// This ensures frontend receives regular updates about online status and accumulated traffic
	if len(data.Inbounds) == 0 && len(data.Outbounds) == 0 && len(data.Users) == 0 {
		// Still send message with online clients list (even if empty)
		// This triggers frontend updates from database values
		logger.Debug("collectTrafficStats: No new traffic this period, sending status update")
	}
	
	jsonData, err := json.Marshal(data)
	if err != nil {
		logger.Error("Failed to marshal traffic data:", err)
		return ""
	}
	
	logger.Infof("Sending traffic stats: %d inbounds, %d outbounds, %d users, %d online", 
		len(data.Inbounds), len(data.Outbounds), len(data.Users), len(data.OnlineClients))
	return string(jsonData)
}

func (s *Slave) applyFullConfig(xrayConfig *xray.Config) {
	logger.Info("Applying new full configuration...")

	// Stop previous process if running
	if s.process != nil && s.process.IsRunning() {
		s.process.Stop()
	}

	// Start new process
	proc := xray.NewProcess(xrayConfig)

	if err := proc.Start(); err != nil {
		logger.Error("Failed to start Xray:", err)
	} else {
		s.process = proc
		logger.Info("Xray started successfully")
		
		// Initialize Xray API for traffic stats
		// Dynamic API port extraction is handled by `proc.Start()` -> `proc.refreshAPIPort()`
		apiPort := proc.GetAPIPort()
		logger.Infof("Xray API Port discovered: %d", apiPort)

		time.Sleep(2 * time.Second) // Wait for Xray to fully start
		if s.xrayAPI == nil {
			s.xrayAPI = &xray.XrayAPI{}
		}
		if err := s.xrayAPI.Init(apiPort); err != nil {
			logger.Error("Failed to initialize Xray API:", err)
		} else {
			logger.Info("Xray API initialized successfully")
		}
	}
}

func (s *Slave) restartXray() {
	logger.Info("Restarting Xray...")
	
	if s.process != nil && s.process.IsRunning() {
		if err := s.process.Stop(); err != nil {
			logger.Error("Failed to stop Xray:", err)
			return
		}
	}
	
	if s.process != nil {
		if err := s.process.Start(); err != nil {
			logger.Error("Failed to restart Xray:", err)
		} else {
			logger.Info("Xray restarted successfully")
		}
	} else {
		logger.Warning("No Xray process to restart")
	}
}

// collectCertificates scans /root/cert directory and reports certificate paths
func (s *Slave) collectCertificates() string {
	certBaseDir := "/root/cert"
	
	if _, err := os.Stat(certBaseDir); os.IsNotExist(err) {
		logger.Debug("Certificate directory does not exist:", certBaseDir)
		return ""
	}
	
	type CertInfo struct {
		Domain      string `json:"domain"`
		CertPath    string `json:"certPath"`
		KeyPath     string `json:"keyPath"`
		ExpiryTime  int64  `json:"expiryTime"`
	}
	
	type CertData struct {
		Type  string     `json:"type"`
		Certs []CertInfo `json:"certs"`
	}
	
	data := CertData{
		Type:  "cert_report",
		Certs: make([]CertInfo, 0),
	}
	
	// Scan subdirectories in /root/cert
	entries, err := os.ReadDir(certBaseDir)
	if err != nil {
		logger.Error("Failed to read cert directory:", err)
		return ""
	}
	
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		domain := entry.Name()
		certDir := filepath.Join(certBaseDir, domain)
		certFile := filepath.Join(certDir, "fullchain.pem")
		keyFile := filepath.Join(certDir, "privkey.pem")
		
		// Check if both files exist
		if _, err := os.Stat(certFile); err != nil {
			continue
		}
		if _, err := os.Stat(keyFile); err != nil {
			continue
		}
		
		// Get certificate expiry (optional, requires parsing cert)
		var expiryTime int64 = 0
		// TODO: Parse certificate and extract expiry time using crypto/x509
		// For now, we'll leave it as 0
		
		data.Certs = append(data.Certs, CertInfo{
			Domain:     domain,
			CertPath:   certFile,
			KeyPath:    keyFile,
			ExpiryTime: expiryTime,
		})
	}
	
	if len(data.Certs) == 0 {
		logger.Debug("No certificates found")
		return ""
	}
	
	jsonData, err := json.Marshal(data)
	if err != nil {
		logger.Error("Failed to marshal cert data:", err)
		return ""
	}
	
	logger.Infof("Reporting %d certificates to master", len(data.Certs))
	return string(jsonData)
}
