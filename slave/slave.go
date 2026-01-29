package slave

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/xray"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type Slave struct {
	MasterUrl string
	Secret    string
	process   *xray.Process
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
	url := fmt.Sprintf("%s?secret=%s", s.MasterUrl, s.Secret)
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
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				stats := s.collectStats()
				if err := c.WriteMessage(websocket.TextMessage, []byte(stats)); err != nil {
					close(done)
					return
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

		if typeStr, ok := msg["type"].(string); ok && typeStr == "update_config" {
			// Handle Config Update
			if inboundsRaw, ok := msg["inbounds"]; ok {
				data, _ := json.Marshal(inboundsRaw)
				var inbounds []*model.Inbound
				json.Unmarshal(data, &inbounds)

				s.applyConfig(inbounds)
			}
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

	return fmt.Sprintf(`{"cpu": %.2f, "mem": %.2f}`, cpuVal, v.UsedPercent)
}

func (s *Slave) applyConfig(inbounds []*model.Inbound) {
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
	}
}
