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
	ws "github.com/mhsanaei/3x-ui/v2/web/websocket"
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
	logger.Infof("PushConfig: retrieved template for slave %d, length: %d", slaveId, len(templateJson))

	// 2. Parse Template into xray.Config struct
	var xrayConfig xray.Config
	if err := json.Unmarshal([]byte(templateJson), &xrayConfig); err != nil {
		return fmt.Errorf("failed to unmarshal xray template config: %v", err)
	}

	// 3. Clean up config (remove helper fields like slaveId from routing/outbounds)
	// Process Routing Rules
	if len(xrayConfig.RouterConfig) > 0 {
		var routerConfig map[string]interface{}
		if err := json.Unmarshal(xrayConfig.RouterConfig, &routerConfig); err == nil {
			if rules, ok := routerConfig["rules"].([]interface{}); ok {
				for _, ruleFn := range rules {
					if rule, ok := ruleFn.(map[string]interface{}); ok {
						delete(rule, "slaveId")
					}
				}
				if newBytes, err := json.Marshal(routerConfig); err == nil {
					xrayConfig.RouterConfig = newBytes
				}
			}
		}
	}

	// Process Outbounds
	if len(xrayConfig.OutboundConfigs) > 0 {
		var outbounds []map[string]interface{}
		if err := json.Unmarshal(xrayConfig.OutboundConfigs, &outbounds); err == nil {
			for _, outbound := range outbounds {
				delete(outbound, "slaveId")
			}
			if newBytes, err := json.Marshal(outbounds); err == nil {
				xrayConfig.OutboundConfigs = newBytes
			}
		}
	}

	// 4. Fetch Inbounds from Database for this Slave
	inbounds, err := s.InboundService.GetInboundsForSlave(slaveId)
	if err != nil {
		return fmt.Errorf("failed to get inbounds for slave %d: %v", slaveId, err)
	}

	// 4. Convert DB Inbounds to Xray InboundConfigs and Append to Template's Inbounds
	// Note: We keep existing inbounds from the template (like 'api' inbound)
	for _, inbound := range inbounds {
		if inbound.Enable {
			// Filter out disabled clients before generating config
			filteredInbound, err := s.filterDisabledClients(inbound)
			if err != nil {
				logger.Warningf("Failed to filter clients for inbound %d: %v", inbound.Id, err)
				// Use original inbound if filtering fails
				filteredInbound = inbound
			}
			xrayInbound := filteredInbound.GenXrayInboundConfig()
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

	logger.Infof("PushConfig: sending update_config_full to slave %d, payload size: %d", slaveId, len(data))
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
		
		// 1. Get all inbounds belonging to this slave
		var inbounds []*model.Inbound
		if err := tx.Where("slave_id = ?", id).Find(&inbounds).Error; err != nil {
			logger.Errorf("Failed to fetch inbounds for slave %d: %v", id, err)
			return err
		}
		
		logger.Infof("Found %d inbounds for slave %d", len(inbounds), id)
		
		// 2. Delete related data for each inbound
		for _, inbound := range inbounds {
			logger.Infof("Deleting data for inbound %d (tag: %s, slave: %d)", inbound.Id, inbound.Tag, id)
			
			// Get all client emails from this inbound to delete account associations
			inboundService := InboundService{}
			clients, err := inboundService.GetClients(inbound)
			if err == nil {
				for _, client := range clients {
					// Delete account_clients associations
					if err := tx.Where("client_email = ?", client.Email).Delete(&model.AccountClient{}).Error; err != nil {
						logger.Warningf("Failed to delete account_clients for email %s: %v", client.Email, err)
					}
					// Delete client IPs
					if err := tx.Where("client_email = ?", client.Email).Delete(&model.InboundClientIps{}).Error; err != nil {
						logger.Warningf("Failed to delete client IPs for email %s: %v", client.Email, err)
					}
				}
			}
			
			// Delete client traffics for this inbound
			if err := tx.Where("inbound_id = ?", inbound.Id).Delete(&xray.ClientTraffic{}).Error; err != nil {
				logger.Warningf("Failed to delete client traffics for inbound %d: %v", inbound.Id, err)
			}
		}
		
		// 3. Delete all inbounds
		if len(inbounds) > 0 {
			if err := tx.Where("slave_id = ?", id).Delete(&model.Inbound{}).Error; err != nil {
				logger.Errorf("Failed to delete inbounds for slave %d: %v", id, err)
				return err
			}
		}
		
		// 4. Delete slave certificates
		// 4. Delete slave certificates
		logger.Infof("Deleting certificates for slave %d", id)
		if err := tx.Where("slave_id = ?", id).Delete(&model.SlaveCert{}).Error; err != nil {
			logger.Errorf("Failed to delete certificates for slave %d: %v", id, err)
			return err
		}
		
		// 5. Delete outbound traffics
		logger.Infof("Deleting outbound traffics for slave %d", id)
		if err := tx.Where("slave_id = ?", id).Delete(&model.OutboundTraffics{}).Error; err != nil {
			logger.Errorf("Failed to delete outbound traffics for slave %d: %v", id, err)
			return err
		}
		
		// 6. Delete XrayOutbounds (if stored in database)
		logger.Infof("Deleting xray outbounds for slave %d", id)
		if err := tx.Where("slave_id = ?", id).Delete(&model.XrayOutbound{}).Error; err != nil {
			logger.Errorf("Failed to delete xray outbounds for slave %d: %v", id, err)
			return err
		}
		
		// 7. Delete XrayRoutingRules
		logger.Infof("Deleting xray routing rules for slave %d", id)
		if err := tx.Where("slave_id = ?", id).Delete(&model.XrayRoutingRule{}).Error; err != nil {
			logger.Errorf("Failed to delete xray routing rules for slave %d: %v", id, err)
			return err
		}
		
		// 8. Delete slave settings
		logger.Infof("Deleting settings for slave %d", id)
		if err := tx.Where("slave_id = ?", id).Delete(&model.SlaveSetting{}).Error; err != nil {
			logger.Errorf("Failed to delete settings for slave %d: %v", id, err)
			return err
		}
		
		// 9. Finally, delete the slave itself
		logger.Infof("Deleting slave record %d", id)
		if err := tx.Delete(&model.Slave{}, id).Error; err != nil {
			logger.Errorf("Failed to delete slave %d: %v", id, err)
			return err
		}
		
		// 10. Remove websocket connection (outside transaction)
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
            
            // Extract versions if present
            xrayVersion, _ := statsData["xrayVersion"].(string)
            uiVersion, _ := statsData["uiVersion"].(string)
            
            if xrayVersion != "" || uiVersion != "" {
                if xrayVersion == "" { xrayVersion = "Unknown" }
                if uiVersion == "" { uiVersion = "Unknown" }
                updates["version"] = fmt.Sprintf("Xray: %s / 3x-ui: %s", xrayVersion, uiVersion)
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
		
		// Sync account traffic: aggregate from all clients belonging to each account
		accountTrafficMap := make(map[int]struct {
			Up   int64
			Down int64
		})
		
		for _, userInterface := range users {
			userData, ok := userInterface.(map[string]interface{})
			if !ok {
				continue
			}
			
			email, _ := userData["email"].(string)
			if email == "" {
				continue
			}
			
			// Get account association
			var clientTraffic xray.ClientTraffic
			if err := db.Where("email = ?", email).First(&clientTraffic).Error; err == nil && clientTraffic.AccountId > 0 {
				uplink, _ := userData["uplink"].(float64)
				downlink, _ := userData["downlink"].(float64)
				
				at := accountTrafficMap[clientTraffic.AccountId]
				at.Up += int64(uplink)
				at.Down += int64(downlink)
				accountTrafficMap[clientTraffic.AccountId] = at
			}
		}
		
		// Update account traffic by aggregating from all its clients
		for accountId := range accountTrafficMap {
			var totalUp, totalDown int64
			err := db.Model(&xray.ClientTraffic{}).
				Select("COALESCE(SUM(up), 0) as up, COALESCE(SUM(down), 0) as down").
				Where("account_id = ?", accountId).
				Row().Scan(&totalUp, &totalDown)
			if err == nil {
				db.Model(&model.Account{}).Where("id = ?", accountId).
					Updates(map[string]interface{}{
						"up":        totalUp,
						"down":      totalDown,
						"updatedAt": now.UnixMilli(),
					})
				logger.Debugf("Updated account %d traffic: up=%d, down=%d", accountId, totalUp, totalDown)
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

	// Check and disable clients that exceeded traffic or expiry limits
	inboundService := InboundService{}
	accountService := AccountService{}
	needConfigPush := false
	
	// 1. Check individual client limits (legacy support)
	disabledClientCount, err := s.checkAndDisableInvalidClients(db, slaveId)
	if err != nil {
		logger.Warning("Error checking invalid clients:", err)
	} else if disabledClientCount > 0 {
		logger.Infof("Disabled %d clients on slave %d due to individual traffic/expiry limits", disabledClientCount, slaveId)
		needConfigPush = true
	}
	
	// 2. Check account-level traffic limits
	trafficLimitSlaves, err := accountService.DisableClientsExceedingAccountLimit()
	if err != nil {
		logger.Warning("Error checking account traffic limits:", err)
	} else if len(trafficLimitSlaves) > 0 {
		logger.Infof("Detected accounts disabled due to traffic limits on slaves: %v", trafficLimitSlaves)
		needConfigPush = true
	}
	
	// 3. Check account-level expiry
	expirySlaves, err := accountService.DisableExpiredAccountClients()
	if err != nil {
		logger.Warning("Error checking account expiry:", err)
	} else if len(expirySlaves) > 0 {
		logger.Infof("Detected accounts disabled due to expiry on slaves: %v", expirySlaves)
		needConfigPush = true
	}
	
	// Push updated config to slave if any clients/accounts were disabled
	if needConfigPush {
		if err := s.PushConfig(slaveId); err != nil {
			logger.Errorf("Failed to push config after disabling clients on slave %d: %v", slaveId, err)
		} else {
			logger.Infof("Pushed updated config to slave %d after disabling clients/accounts", slaveId)
		}
	}
	
	// Broadcast updates to frontend via WebSocket for real-time display
	// Get updated inbounds with accumulated traffic from database
	// IMPORTANT: Create a new InboundService instance to force fresh database query
	// This ensures we don't get cached data from the previous operations
	freshInboundService := InboundService{}
	updatedInbounds, err := freshInboundService.GetAllInbounds()
	if err != nil {
		logger.Warning("Failed to get inbounds for websocket broadcast:", err)
	} else if updatedInbounds == nil {
		logger.Warning("GetAllInbounds returned nil (no error)")
	} else {
		logger.Infof("GetAllInbounds returned %d inbounds", len(updatedInbounds))
		if len(updatedInbounds) > 0 {
			// Log sample data from first inbound for verification
			logger.Infof("Sample inbound data - id=%d, tag=%s, up=%d, down=%d, clientStats=%d",
				updatedInbounds[0].Id, updatedInbounds[0].Tag, updatedInbounds[0].Up, 
				updatedInbounds[0].Down, len(updatedInbounds[0].ClientStats))
			// Also log the inbound that was just updated if it exists
			for _, inbound := range updatedInbounds {
				if inbound.SlaveId == slaveId {
					logger.Infof("Slave %d inbound - id=%d, tag=%s, up=%d, down=%d",
						slaveId, inbound.Id, inbound.Tag, inbound.Up, inbound.Down)
				}
			}
		}
		logger.Infof("Calling BroadcastInbounds with %d inbounds", len(updatedInbounds))
		ws.BroadcastInbounds(updatedInbounds)
		logger.Infof("BroadcastInbounds completed (broadcasted %d inbounds to frontend)", len(updatedInbounds))
	}

	
	// Get online clients and last online map
	onlineClients := s.GetAllOnlineClients()
	lastOnlineMap, err := inboundService.GetClientsLastOnline()
	if err != nil {
		logger.Warning("Failed to get last online map:", err)
		lastOnlineMap = make(map[string]int64)
	}
	
	// Broadcast traffic update with online status
	trafficUpdate := map[string]any{
		"onlineClients": onlineClients,
		"lastOnlineMap": lastOnlineMap,
	}
	ws.BroadcastTraffic(trafficUpdate)
	logger.Debugf("Broadcasted traffic update: %d online clients", len(onlineClients))
	
	// Get and broadcast outbounds if any
	outboundService := OutboundService{}
	updatedOutbounds, err := outboundService.GetOutboundsTraffic()
	if err != nil {
		logger.Warning("Failed to get outbounds for websocket broadcast:", err)
	} else if updatedOutbounds != nil && len(updatedOutbounds) > 0 {
		ws.BroadcastOutbounds(updatedOutbounds)
		logger.Debugf("Broadcasted %d outbounds to frontend", len(updatedOutbounds))
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
	command := fmt.Sprintf("bash <(curl -Ls https://raw.githubusercontent.com/Copperchaleu/3x-ui-cluster/main/install.sh) slave %s %s",
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

// checkAndDisableInvalidClients checks for clients that exceeded limits and disables them in the database
func (s *SlaveService) checkAndDisableInvalidClients(db *gorm.DB, slaveId int) (int64, error) {
	now := time.Now().Unix() * 1000

	// Find all clients on this slave that exceeded traffic or expiry limits
	result := db.Model(&xray.ClientTraffic{}).
		Where(`inbound_id IN (
			SELECT id FROM inbounds WHERE slave_id = ?
		) AND ((total > 0 AND up + down >= total) OR (expiry_time > 0 AND expiry_time <= ?)) AND enable = ?`,
			slaveId, now, true).
		Update("enable", false)

	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// filterDisabledClients removes disabled clients from inbound settings based on client_traffics table
func (s *SlaveService) filterDisabledClients(inbound *model.Inbound) (*model.Inbound, error) {
	db := database.GetDB()
	
	// Parse inbound settings
	var settings map[string]interface{}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return inbound, err
	}
	
	// Get clients array
	clientsInterface, ok := settings["clients"]
	if !ok {
		// No clients in settings, return as is
		return inbound, nil
	}
	
	clients, ok := clientsInterface.([]interface{})
	if !ok || len(clients) == 0 {
		return inbound, nil
	}
	
	// Get all client traffic with account associations
	var clientTraffics []xray.ClientTraffic
	if err := db.Where("inbound_id = ?", inbound.Id).Find(&clientTraffics).Error; err != nil {
		return inbound, err
	}
	
	// Get all accounts to check their enable status
	accountIds := make([]int, 0)
	for _, ct := range clientTraffics {
		if ct.AccountId > 0 {
			accountIds = append(accountIds, ct.AccountId)
		}
	}
	
	accountEnableMap := make(map[int]bool)
	if len(accountIds) > 0 {
		var accounts []model.Account
		if err := db.Where("id IN ?", accountIds).Find(&accounts).Error; err == nil {
			for _, acc := range accounts {
				accountEnableMap[acc.Id] = acc.Enable
			}
		}
	}
	
	// Create a map of email -> enable status
	// Priority: If client is associated with account, use account's enable status
	// Otherwise, use client's own enable status
	enableMap := make(map[string]bool)
	for _, ct := range clientTraffics {
		var finalEnabled bool
		
		// If client is associated with an account, prioritize account status
		if ct.AccountId > 0 {
			if accountEnabled, exists := accountEnableMap[ct.AccountId]; exists {
				// Use account's enable status as the authoritative source
				finalEnabled = accountEnabled
			} else {
				// Account not found, fallback to client's own status
				finalEnabled = ct.Enable
			}
		} else {
			// No account association, use client's own enable status
			finalEnabled = ct.Enable
		}
		
		enableMap[ct.Email] = finalEnabled
	}
	
	// Filter clients - only keep enabled ones
	// Initialize as empty slice (not nil) to ensure JSON encodes as [] instead of null
	filteredClients := make([]interface{}, 0)
	for _, clientInterface := range clients {
		client, ok := clientInterface.(map[string]interface{})
		if !ok {
			continue
		}
		
		email, hasEmail := client["email"].(string)
		if !hasEmail || email == "" {
			// No email, keep the client (shouldn't happen normally)
			filteredClients = append(filteredClients, clientInterface)
			continue
		}
		
		// Check if client is enabled
		if enabled, exists := enableMap[email]; exists && !enabled {
			// Client is disabled, skip it
			logger.Debugf("Filtering out disabled client: %s from inbound %d", email, inbound.Id)
			continue
		}
		
		// Client is enabled or not found in traffic table, keep it
		filteredClients = append(filteredClients, clientInterface)
	}
	
	// Update settings with filtered clients
	settings["clients"] = filteredClients
	
	// Marshal back to JSON
	filteredSettings, err := json.Marshal(settings)
	if err != nil {
		return inbound, err
	}
	
	// Create a copy of inbound with filtered settings
	filteredInbound := *inbound
	filteredInbound.Settings = string(filteredSettings)
	
	logger.Debugf("Filtered inbound %d: %d total clients, %d enabled clients", 
		inbound.Id, len(clients), len(filteredClients))
	
	return &filteredInbound, nil
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
