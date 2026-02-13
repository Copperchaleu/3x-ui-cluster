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
	InboundService      InboundService
	SlaveSettingService SlaveSettingService
}

// In-memory store for active connections
var (
	slaveConns      = make(map[int]*websocket.Conn)
	slaveLock       sync.RWMutex
	slaveOnlineClients = make(map[int][]string) // Store online clients per slave
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
	// Clear online clients for this slave
	delete(slaveOnlineClients, slaveId)
	logger.Infof("Slave %d disconnected", slaveId)
}

func (s *SlaveService) PushConfig(slaveId int) error {
	// 1. Get the Full Template from Slave Settings (contains Log, API, DNS, Outbounds/Routing)
	templateJson, err := s.SlaveSettingService.GetXrayConfigForSlave(slaveId)
	if err != nil {
		return fmt.Errorf("failed to get xray template config for slave %d: %v", slaveId, err)
	}

	// 2. Parse Template into xray.Config struct
	var xrayConfig xray.Config
	if err := json.Unmarshal([]byte(templateJson), &xrayConfig); err != nil {
		return fmt.Errorf("failed to unmarshal xray template config: %v", err)
	}

	// 3. Fetch Inbounds from Database for this Slave
	inbounds, err := s.InboundService.GetInboundsForSlave(slaveId)
	if err != nil {
		return fmt.Errorf("failed to get inbounds for slave %d: %v", slaveId, err)
	}

	// 4. Convert DB Inbounds to Xray InboundConfigs and Append to Template's Inbounds
	// Note: We keep existing inbounds from the template (like 'api' inbound)
	for _, inbound := range inbounds {
		if inbound.Enable {
			xrayInbound := inbound.GenXrayInboundConfig()
			xrayConfig.InboundConfigs = append(xrayConfig.InboundConfigs, *xrayInbound)
		}
	}

	// 5. Marshal the Final Config to JSON
	finalConfigBytes, err := json.Marshal(xrayConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal final xray config: %v", err)
	}

	// 6. Send to Slave
	data, err := json.Marshal(map[string]interface{}{
		"type":   "update_config_full",
		"config": string(finalConfigBytes),
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
	
	// Use transaction to ensure all deletes succeed or none
	return db.Transaction(func(tx *gorm.DB) error {
		logger.Infof("Starting cascade delete for slave %d", id)
		
		// 1. Delete all inbounds belonging to this slave
		// This will cascade delete: client_traffics, inbound_client_ips, account_clients
		var inbounds []*model.Inbound
		if err := tx.Where("slave_id = ?", id).Find(&inbounds).Error; err != nil {
			logger.Errorf("Failed to fetch inbounds for slave %d: %v", id, err)
			return err
		}
		
		logger.Infof("Found %d inbounds for slave %d", len(inbounds), id)
		inboundService := InboundService{}
		for _, inbound := range inbounds {
			logger.Infof("Deleting inbound %d (tag: %s) for slave %d", inbound.Id, inbound.Tag, id)
			if _, err := inboundService.DelInbound(inbound.Id); err != nil {
				logger.Errorf("Failed to delete inbound %d for slave %d: %v", inbound.Id, id, err)
				return err
			}
		}
		
		// 2. Delete slave certificates
		logger.Infof("Deleting certificates for slave %d", id)
		slaveCertService := SlaveCertService{}
		if err := slaveCertService.DeleteCertsForSlave(id); err != nil {
			logger.Warningf("Failed to delete certificates for slave %d: %v", id, err)
			// Continue anyway, as certs might not exist
		}
		
		// 3. Delete outbound traffics
		logger.Infof("Deleting outbound traffics for slave %d", id)
		if err := tx.Where("slave_id = ?", id).Delete(&model.OutboundTraffics{}).Error; err != nil {
			logger.Errorf("Failed to delete outbound traffics for slave %d: %v", id, err)
			return err
		}
		
		// 4. Delete XrayOutbounds (if stored in database)
		logger.Infof("Deleting xray outbounds for slave %d", id)
		if err := tx.Where("slave_id = ?", id).Delete(&model.XrayOutbound{}).Error; err != nil {
			logger.Errorf("Failed to delete xray outbounds for slave %d: %v", id, err)
			return err
		}
		
		// 5. Delete XrayRoutingRules
		logger.Infof("Deleting xray routing rules for slave %d", id)
		if err := tx.Where("slave_id = ?", id).Delete(&model.XrayRoutingRule{}).Error; err != nil {
			logger.Errorf("Failed to delete xray routing rules for slave %d: %v", id, err)
			return err
		}
		
		// 6. Delete slave settings
		logger.Infof("Deleting settings for slave %d", id)
		if err := tx.Where("slave_id = ?", id).Delete(&model.SlaveSetting{}).Error; err != nil {
			logger.Errorf("Failed to delete settings for slave %d: %v", id, err)
			return err
		}
		
		// 7. Finally, delete the slave itself
		logger.Infof("Deleting slave record %d", id)
		if err := tx.Delete(&model.Slave{}, id).Error; err != nil {
			logger.Errorf("Failed to delete slave %d: %v", id, err)
			return err
		}
		
		// 8. Remove websocket connection (outside transaction)
		// This is safe to do even if transaction fails
		go func() {
			s.RemoveSlaveConn(id)
		}()
		
		logger.Infof("Successfully completed cascade delete for slave %d", id)
		return nil
	})
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

	// Process online clients list
	if onlineClients, ok := data["online_clients"].([]interface{}); ok {
		clients := make([]string, 0, len(onlineClients))
		for _, client := range onlineClients {
			if email, ok := client.(string); ok && email != "" {
				clients = append(clients, email)
			}
		}
		
		// Update the global online clients map for this slave
		slaveLock.Lock()
		slaveOnlineClients[slaveId] = clients
		slaveLock.Unlock()
		
		logger.Debugf("Updated online clients for slave %d: %d clients", slaveId, len(clients))
	}

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

	// Process user traffic stats

	
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



				logger.Infof("Updated user traffic: email=%s, up=%d, down=%d, inbound_id=%d",
					email, int64(uplink), int64(downlink), clientTraffic.InboundId)
			} else {
				logger.Debugf("User not found in database: %s", email)
			}
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

// ProcessCertReport processes certificate information reported by slave
func (s *SlaveService) ProcessCertReport(slaveId int, data map[string]interface{}) error {
	certs, ok := data["certs"].([]interface{})
	if !ok || len(certs) == 0 {
		logger.Debugf("No certificates in report from slave %d", slaveId)
		return nil
	}
	
	logger.Infof("Processing certificate report from slave %d: %d certificates", slaveId, len(certs))
	
	certService := SlaveCertService{}
	var certModels []model.SlaveCert
	
	for _, certInterface := range certs {
		certData, ok := certInterface.(map[string]interface{})
		if !ok {
			continue
		}
		
		domain, _ := certData["domain"].(string)
		certPath, _ := certData["certPath"].(string)
		keyPath, _ := certData["keyPath"].(string)
		expiryTime, _ := certData["expiryTime"].(float64)
		
		if domain == "" || certPath == "" || keyPath == "" {
			continue
		}
		
		certModels = append(certModels, model.SlaveCert{
			SlaveId:    slaveId,
			Domain:     domain,
			CertPath:   certPath,
			KeyPath:    keyPath,
			ExpiryTime: int64(expiryTime),
		})
		
		logger.Infof("Certificate reported: slave=%d, domain=%s, cert=%s", slaveId, domain, certPath)
	}
	
	if len(certModels) > 0 {
		if err := certService.BatchUpsertCerts(slaveId, certModels); err != nil {
			logger.Errorf("Failed to save certificates for slave %d: %v", slaveId, err)
			return err
		}
		logger.Infof("Successfully saved %d certificates for slave %d", len(certModels), slaveId)
	}
	
	return nil
}

// GetAllOnlineClients returns all online clients from all connected slaves
func (s *SlaveService) GetAllOnlineClients() []string {
	slaveLock.RLock()
	defer slaveLock.RUnlock()
	
	// Use a map to deduplicate clients (in case a client appears on multiple slaves)
	clientMap := make(map[string]bool)
	for _, clients := range slaveOnlineClients {
		for _, email := range clients {
			clientMap[email] = true
		}
	}
	
	// Convert map keys to slice
	result := make([]string, 0, len(clientMap))
	for email := range clientMap {
		result = append(result, email)
	}
	
	return result
}
