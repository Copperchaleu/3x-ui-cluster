package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
)

type SlaveService struct {
	InboundService InboundService
}

// In-memory store for active connections
var (
	slaveConns = make(map[int]*websocket.Conn)
	slaveLock  sync.RWMutex
)

func (s *SlaveService) AddSlaveConn(slaveId int, conn *websocket.Conn) {
	slaveLock.Lock()
	defer slaveLock.Unlock()
	if old, ok := slaveConns[slaveId]; ok {
		old.Close()
	}
	slaveConns[slaveId] = conn
	logger.Infof("Slave %d connected", slaveId)
}

func (s *SlaveService) RemoveSlaveConn(slaveId int) {
	slaveLock.Lock()
	defer slaveLock.Unlock()
	if conn, ok := slaveConns[slaveId]; ok {
		conn.Close()
		delete(slaveConns, slaveId)
	}
	logger.Infof("Slave %d disconnected", slaveId)
}

func (s *SlaveService) PushConfig(slaveId int) error {
	inbounds, err := s.InboundService.GetInboundsForSlave(slaveId)
	if err != nil {
		return err
	}

	// Fetch Outbounds and Routing Rules
	outboundService := &OutboundService{}
	outbounds, err := outboundService.GetOutbounds(slaveId)
	if err != nil {
		return err
	}

	routingService := &RoutingService{}
	routingRules, err := routingService.GetRoutingRules(slaveId)
	if err != nil {
		return err
	}

	// Helper to convert to map for JSON marshaling if needed, 
    // or rely on model struct json tags if they match Xray config format.
    // However, model XrayOutbound has JSON strings. We need to parse them or send them as is and let Slave parse?
    // Slave expects ready-to-use config or raw?
    // If Slave is another 3x-ui instance running in slave mode, we need to check what it expects.
    // Assuming we send "outbounds" and "routingRules" arrays.

    // We need to convert our DB models to Xray Config structures
    xrayOutbounds := make([]interface{}, 0)
    for _, o := range outbounds {
        xrayOutbounds = append(xrayOutbounds, o.GenXrayOutboundConfig())
    }

    xrayRoutingRules := make([]interface{}, 0)
    for _, r := range routingRules {
        xrayRoutingRules = append(xrayRoutingRules, r.GenXrayRoutingRuleConfig())
    }

	data, err := json.Marshal(map[string]interface{}{
		"type":         "update_config",
		"inbounds":     inbounds,
		"outbounds":    xrayOutbounds,
		"routingRules": xrayRoutingRules,
	})
	if err != nil {
		return err
	}

	slaveLock.RLock()
	conn, ok := slaveConns[slaveId]
	slaveLock.RUnlock()

	if !ok {
		return fmt.Errorf("slave %d not connected", slaveId)
	}

	return conn.WriteMessage(websocket.TextMessage, data)
}

func (s *SlaveService) RestartSlaveXray(slaveId int) error {
	data, err := json.Marshal(map[string]interface{}{
		"type": "restart_xray",
	})
	if err != nil {
		return err
	}

	slaveLock.RLock()
	conn, ok := slaveConns[slaveId]
	slaveLock.RUnlock()

	if !ok {
		return fmt.Errorf("slave %d not connected", slaveId)
	}

	return conn.WriteMessage(websocket.TextMessage, data)
}

func (s *SlaveService) GetAllSlaves() ([]*model.Slave, error) {
	db := database.GetDB()
	var slaves []*model.Slave
	err := db.Model(model.Slave{}).Find(&slaves).Error
	return slaves, err
}

func (s *SlaveService) GetSlave(id int) (*model.Slave, error) {
	db := database.GetDB()
	var slave model.Slave
	err := db.First(&slave, id).Error
	return &slave, err
}

func (s *SlaveService) GetSlaveBySecret(secret string) (*model.Slave, error) {
	db := database.GetDB()
	var slave model.Slave
	err := db.Where("secret = ?", secret).First(&slave).Error
	return &slave, err
}

func (s *SlaveService) AddSlave(slave *model.Slave) error {
	// Auto-generate secret if not provided
	if slave.Secret == "" {
		slave.Secret = generateRandomSecret(32)
	}
	slave.Status = "offline"
	slave.LastSeen = time.Now().Unix()
	
	db := database.GetDB()
	return db.Create(slave).Error
}

func generateRandomSecret(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

func (s *SlaveService) DeleteSlave(id int) error {
	db := database.GetDB()
	return db.Delete(&model.Slave{}, id).Error
}

func (s *SlaveService) UpdateSlaveStatus(id int, status string, stats string) error {
    db := database.GetDB()
    return db.Model(&model.Slave{}).Where("id = ?", id).Updates(map[string]interface{}{
        "status": status,
        "systemStats": stats,
        "lastSeen": time.Now().Unix(),
    }).Error
}

func (s *SlaveService) GenerateInstallCommand(slaveId int, req *http.Request) (string, error) {
	slave, err := s.GetSlave(slaveId)
	if err != nil {
		return "", err
	}
	
	// Get master server address from request
	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	host := req.Host
	
	// Generate install command
	command := fmt.Sprintf("bash <(curl -Ls https://raw.githubusercontent.com/mhsanaei/3x-ui/master/install.sh) slave %s://%s %s",
		scheme, host, slave.Secret)
	
	return command, nil
}
