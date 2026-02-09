package slave

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/util/json_util"
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
		defer ticker.Stop()
		defer trafficTicker.Stop()
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
		case "update_config":
			// Handle Config Update
			var inbounds []*model.Inbound
			if inboundsRaw, ok := msg["inbounds"]; ok {
				data, _ := json.Marshal(inboundsRaw)
				json.Unmarshal(data, &inbounds)
			}
            
            var outbounds []interface{}
            if outboundsRaw, ok := msg["outbounds"]; ok {
                 outbounds = outboundsRaw.([]interface{})
            }
            
            var routingRules []interface{}
            if rulesRaw, ok := msg["routingRules"]; ok {
                 routingRules = rulesRaw.([]interface{})
            }

			s.applyConfig(inbounds, outbounds, routingRules)
			
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
		Type      string                       `json:"type"`
		Inbounds  map[string]map[string]int64  `json:"inbounds"`
		Outbounds map[string]map[string]int64  `json:"outbounds"`
		Users     []map[string]interface{}     `json:"users"`
	}
	
	data := TrafficData{
		Type:      "traffic_stats",
		Inbounds:  make(map[string]map[string]int64),
		Outbounds: make(map[string]map[string]int64),
		Users:     make([]map[string]interface{}, 0),
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
	
	// Collect user traffic
	for _, clientTraffic := range clientTraffics {
		if clientTraffic.Email != "" {
			data.Users = append(data.Users, map[string]interface{}{
				"email":    clientTraffic.Email,
				"uplink":   clientTraffic.Up,
				"downlink": clientTraffic.Down,
			})
		}
	}
	
	if len(data.Inbounds) == 0 && len(data.Outbounds) == 0 && len(data.Users) == 0 {
		logger.Debug("collectTrafficStats: No traffic data")
		return ""
	}
	
	jsonData, err := json.Marshal(data)
	if err != nil {
		logger.Error("Failed to marshal traffic data:", err)
		return ""
	}
	
	logger.Infof("Sending traffic stats: %d inbounds, %d outbounds, %d users", 
		len(data.Inbounds), len(data.Outbounds), len(data.Users))
	return string(jsonData)
}

func (s *Slave) applyConfig(inbounds []*model.Inbound, outbounds []interface{}, routingRules []interface{}) {
	logger.Info("Applying new configuration...")
	xrayConfig := &xray.Config{}

	// Basic Xray Config structure (log, api, etc.)
    // Note: In local specific package versions, fields might be json_util.RawMessage.
    // We construct simple JSONs and convert them.

    logCfg := map[string]interface{}{"loglevel": "warning"}
    logBytes, _ := json.Marshal(logCfg)
	xrayConfig.LogConfig = logBytes

    apiCfg := map[string]interface{}{
		"tag":      "api",
		"services": []string{"HandlerService", "LoggerService", "StatsService"},
	}
    apiBytes, _ := json.Marshal(apiCfg)
	xrayConfig.API = apiBytes

    statsCfg := map[string]interface{}{}
    statsBytes, _ := json.Marshal(statsCfg)
	xrayConfig.Stats = statsBytes

	// Outbounds
    if len(outbounds) > 0 {
         outBytes, _ := json.Marshal(outbounds)
         xrayConfig.OutboundConfigs = outBytes
    }

	// Routing - add API routing rule
	apiRoutingRule := map[string]interface{}{
		"type":        "field",
		"inboundTag":  []string{"api"},
		"outboundTag": "api",
	}
	allRoutingRules := append([]interface{}{apiRoutingRule}, routingRules...)
	
	routerCfg := map[string]interface{}{
		"domainStrategy": "AsIs",
		"rules":          allRoutingRules,
	}
	routerBytes, _ := json.Marshal(routerCfg)
	xrayConfig.RouterConfig = routerBytes

    policyCfg := map[string]interface{}{
		"levels": map[string]interface{}{
			"0": map[string]bool{"statsUserUplink": true, "statsUserDownlink": true},
		},
		"system": map[string]bool{
			"statsInboundUplink":   true,
			"statsInboundDownlink": true,
		},
	}
    policyBytes, _ := json.Marshal(policyCfg)
	xrayConfig.Policy = policyBytes

	// Convert inbounds
	for _, inbound := range inbounds {
		if !inbound.Enable {
			continue
		}
		if config := inbound.GenXrayInboundConfig(); config != nil {
			xrayConfig.InboundConfigs = append(xrayConfig.InboundConfigs, *config)
		}
	}
	
	// Add API inbound for stats
	apiInbound := xray.InboundConfig{
		Listen:   json_util.RawMessage(`"127.0.0.1"`),
		Port:     10085,
		Protocol: "dokodemo-door",
		Tag:      "api",
		Settings: json_util.RawMessage(`{"address": "127.0.0.1"}`),
	}
	xrayConfig.InboundConfigs = append(xrayConfig.InboundConfigs, apiInbound)

	// Stop previous process if running
	if s.process != nil && s.process.IsRunning() {
		s.process.Stop()
	}

	// Start new process
    // Use default xray path or find it
    // xray.NewProcess takes *Config
    // But xray.NewProcess inside expects to find binary itself via config.GetBinFolderPath() + ...
    // We might need to mock or set config paths if we run as slave.
    
    // However, looking at source `xray/process.go`:
    // func NewProcess(xrayConfig *Config) *Process
    
    // We should rely on `xray.NewProcess` to handle binary path if we set up environment correctly.
    // Or we might need to modify `xray` package to allow custom binary path, but for now let's assume standard path.
    
	proc := xray.NewProcess(xrayConfig)

	if err := proc.Start(); err != nil {
		logger.Error("Failed to start Xray:", err)
	} else {
		s.process = proc
		logger.Info("Xray started successfully")
		
		// Initialize Xray API for traffic stats
		time.Sleep(2 * time.Second) // Wait for Xray to fully start
		if s.xrayAPI == nil {
			s.xrayAPI = &xray.XrayAPI{}
		}
		if err := s.xrayAPI.Init(10085); err != nil {
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
