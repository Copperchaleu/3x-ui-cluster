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
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
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

func (s *SlaveService) GetAllSlavesWithTraffic() ([]map[string]interface{}, error) {
	db := database.GetDB()
	var slaves []*model.Slave
	if err := db.Model(model.Slave{}).Find(&slaves).Error; err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, len(slaves))
	for i, slave := range slaves {
		// Get traffic stats from inbounds table
		var totalUplink, totalDownlink int64
		type TrafficSum struct {
			TotalUp   int64
			TotalDown int64
		}
		var trafficSum TrafficSum
		db.Model(&model.Inbound{}).
			Where("slave_id = ?", slave.Id).
			Select("COALESCE(SUM(up), 0) as total_up, COALESCE(SUM(down), 0) as total_down").
			Scan(&trafficSum)
		
		totalUplink = trafficSum.TotalUp
		totalDownlink = trafficSum.TotalDown

		result[i] = map[string]interface{}{
			"id":           slave.Id,
			"name":         slave.Name,
			"address":      slave.Address,
			"port":         slave.Port,
			"secret":       slave.Secret,
			"status":       slave.Status,
			"lastSeen":     slave.LastSeen,
			"version":      slave.Version,
			"systemStats":  slave.SystemStats,
			"totalUplink":  totalUplink,
			"totalDownlink": totalDownlink,
		}
	}

	return result, nil
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
    
    updates := map[string]interface{}{
        "status":      status,
        "systemStats": stats,
        "lastSeen":    time.Now().Unix(),
    }
    
    // Extract address from stats JSON if present
    if stats != "" {
        var statsData map[string]interface{}
        if err := json.Unmarshal([]byte(stats), &statsData); err == nil {
            if address, ok := statsData["address"].(string); ok && address != "" {
                updates["address"] = address
            }
        }
    }
    
    return db.Model(&model.Slave{}).Where("id = ?", id).Updates(updates).Error
}

func (s *SlaveService) ProcessTrafficStats(slaveId int, data map[string]interface{}) error {
	db := database.GetDB()
	now := time.Now()

	// Process inbound traffic stats
	if inbounds, ok := data["inbounds"].(map[string]interface{}); ok {
		logger.Infof("ProcessTrafficStats: Processing %d inbounds for slave %d", len(inbounds), slaveId)
		
		for inboundTag, statsInterface := range inbounds {
			stats, ok := statsInterface.(map[string]interface{})
			if !ok {
				continue
			}

			uplink, _ := stats["uplink"].(float64)
			downlink, _ := stats["downlink"].(float64)

			// Update inbounds table directly
			result := db.Model(&model.Inbound{}).
				Where("tag = ? AND slave_id = ?", inboundTag, slaveId).
				Updates(map[string]interface{}{
					"up":       gorm.Expr("up + ?", int64(uplink)),
					"down":     gorm.Expr("down + ?", int64(downlink)),
					"all_time": gorm.Expr("COALESCE(all_time, 0) + ?", int64(uplink+downlink)),
				})

			if result.Error != nil {
				logger.Errorf("Failed to update inbound traffic: slave=%d, tag=%s, error=%v",
					slaveId, inboundTag, result.Error)
			} else {
				logger.Infof("Updated inbound traffic: slave=%d, tag=%s, up=%d, down=%d, rows=%d",
					slaveId, inboundTag, int64(uplink), int64(downlink), result.RowsAffected)
			}
		}
	}

	// Process user traffic stats and aggregate to inbound
	inboundTrafficMap := make(map[int]struct{ up, down int64 })
	
	if users, ok := data["users"].([]interface{}); ok {
		logger.Infof("ProcessTrafficStats: Processing %d users for slave %d", len(users), slaveId)
		
		for _, userInterface := range users {
			userData, ok := userInterface.(map[string]interface{})
			if !ok {
				continue
			}

			email, _ := userData["email"].(string)
			uplink, _ := userData["uplink"].(float64)
			downlink, _ := userData["downlink"].(float64)

			if email == "" || (uplink == 0 && downlink == 0) {
				continue
			}

			// Update client traffic
			var clientTraffic xray.ClientTraffic
			result := db.Where("email = ?", email).First(&clientTraffic)

			if result.Error == nil {
				// Update existing client
				clientTraffic.Up += int64(uplink)
				clientTraffic.Down += int64(downlink)
				clientTraffic.AllTime += int64(uplink) + int64(downlink)
				clientTraffic.LastOnline = now.Unix()
				db.Save(&clientTraffic)

				// Aggregate traffic by inbound_id
				if traffic, exists := inboundTrafficMap[clientTraffic.InboundId]; exists {
					traffic.up += int64(uplink)
					traffic.down += int64(downlink)
					inboundTrafficMap[clientTraffic.InboundId] = traffic
				} else {
					inboundTrafficMap[clientTraffic.InboundId] = struct{ up, down int64 }{
						up:   int64(uplink),
						down: int64(downlink),
					}
				}

				logger.Infof("Updated user traffic: email=%s, up=%d, down=%d, inbound_id=%d",
					email, int64(uplink), int64(downlink), clientTraffic.InboundId)
			} else {
				logger.Debugf("User not found in database: %s", email)
			}
		}
	}

	// Update inbound traffic based on aggregated user traffic
	for inboundId, traffic := range inboundTrafficMap {
		result := db.Model(&model.Inbound{}).
			Where("id = ? AND slave_id = ?", inboundId, slaveId).
			Updates(map[string]interface{}{
				"up":       gorm.Expr("up + ?", traffic.up),
				"down":     gorm.Expr("down + ?", traffic.down),
				"all_time": gorm.Expr("COALESCE(all_time, 0) + ?", traffic.up+traffic.down),
			})

		if result.Error != nil {
			logger.Errorf("Failed to update inbound traffic from users: inbound_id=%d, error=%v",
				inboundId, result.Error)
		} else if result.RowsAffected > 0 {
			logger.Infof("Updated inbound traffic from users: inbound_id=%d, up=%d, down=%d",
				inboundId, traffic.up, traffic.down)
		}
	}

	// Process outbound traffic stats
	if outbounds, ok := data["outbounds"].(map[string]interface{}); ok {
		logger.Infof("ProcessTrafficStats: Processing %d outbounds for slave %d", len(outbounds), slaveId)
		
		for outboundTag, statsInterface := range outbounds {
			stats, ok := statsInterface.(map[string]interface{})
			if !ok {
				continue
			}

			uplink, _ := stats["uplink"].(float64)
			downlink, _ := stats["downlink"].(float64)

			if uplink == 0 && downlink == 0 {
				continue
			}

			// Update or create outbound traffic record
			var outbound model.OutboundTraffics
			result := db.Where("tag = ? AND slave_id = ?", outboundTag, slaveId).
				FirstOrCreate(&outbound, model.OutboundTraffics{Tag: outboundTag, SlaveId: slaveId})

			if result.Error == nil {
				outbound.Up += int64(uplink)
				outbound.Down += int64(downlink)
				outbound.Total = outbound.Up + outbound.Down
				db.Save(&outbound)

				logger.Infof("Updated outbound traffic: slave=%d, tag=%s, up=%d, down=%d, total=%d",
					slaveId, outboundTag, int64(uplink), int64(downlink), outbound.Total)
			} else {
				logger.Errorf("Failed to update outbound traffic: slave=%d, tag=%s, error=%v",
					slaveId, outboundTag, result.Error)
			}
		}
	}

	return nil
}

func (s *SlaveService) GenerateInstallCommand(slaveId int, req *http.Request, basePath string) (string, error) {
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
	
	// Build the full URL with basePath
	// basePath already includes leading and trailing slashes (e.g., "/ixUwrIpIWgOzE7ZS9w/")
	masterUrl := fmt.Sprintf("%s://%s%s", scheme, host, basePath)
	
	// Generate install command
	command := fmt.Sprintf("bash <(curl -Ls https://raw.githubusercontent.com/GrayPaul0320/3x-ui-cluster/main/install.sh) slave %s %s",
		masterUrl, slave.Secret)
	
	return command, nil
}
