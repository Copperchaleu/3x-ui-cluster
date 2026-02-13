package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/session"
	"github.com/mhsanaei/3x-ui/v2/logger"
)

type SlaveController struct {
	slaveService service.SlaveService
}

func NewSlaveController(g *gin.RouterGroup, slaveService service.SlaveService) *SlaveController {
	s := &SlaveController{slaveService: slaveService}
	s.initRouter(g)
	return s
}

func (s *SlaveController) initRouter(g *gin.RouterGroup) {
	g.GET("/list", s.getSlaves)
	g.POST("/add", s.addSlave)
	g.POST("/del/:id", s.delSlave)
	g.GET("/install/:id", s.getInstallCommand)
}

func (s *SlaveController) getSlaves(c *gin.Context) {
	if !session.IsLogin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "unauthorized"})
		return
	}
    slaves, err := s.slaveService.GetAllSlavesWithTraffic()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "msg": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"success": true, "obj": slaves})
}

func (s *SlaveController) addSlave(c *gin.Context) {
	if !session.IsLogin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "unauthorized"})
		return
	}
	
    var slave model.Slave
    if err := c.ShouldBindJSON(&slave); err != nil {
         logger.Errorf("Failed to bind slave JSON: %v", err)
         c.JSON(http.StatusBadRequest, gin.H{"success": false, "msg": fmt.Sprintf("Invalid request data: %v", err)})
         return
    }
    
    logger.Infof("Adding slave: name=%s", slave.Name)
    if err := s.slaveService.AddSlave(&slave); err != nil {
         logger.Errorf("Failed to add slave: %v", err)
         c.JSON(http.StatusInternalServerError, gin.H{"success": false, "msg": err.Error()})
         return
    }
    
    // Initialize slave settings with defaults (xrayTemplateConfig)
    slaveSettingService := service.SlaveSettingService{}
    if err := slaveSettingService.InitializeSlaveWithDefaults(slave.Id); err != nil {
         logger.Warningf("Failed to initialize settings for new slave %d: %v", slave.Id, err)
    }
    
    logger.Infof("Slave added successfully: id=%d, name=%s", slave.Id, slave.Name)
    c.JSON(http.StatusOK, gin.H{"success": true, "msg": "Slave added", "obj": slave})
}

func (s *SlaveController) delSlave(c *gin.Context) {
	if !session.IsLogin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "unauthorized"})
		return
	}
    id, _ := strconv.Atoi(c.Param("id"))
    
    logger.Infof("Deleting slave %d with cascade", id)
    
    // DeleteSlave now handles all cascade deletions:
    // - inbounds, clients, traffics, IPs, account associations
    // - slave certs, outbound traffics, xray outbounds/routing rules
    // - slave settings
    if err := s.slaveService.DeleteSlave(id); err != nil {
         logger.Errorf("Failed to delete slave %d: %v", id, err)
         c.JSON(http.StatusInternalServerError, gin.H{"success": false, "msg": err.Error()})
         return
    }
    
    logger.Infof("Successfully deleted slave %d", id)
    c.JSON(http.StatusOK, gin.H{"success": true, "msg": "Slave deleted"})
}

func (s *SlaveController) getInstallCommand(c *gin.Context) {
	if !session.IsLogin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "unauthorized"})
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	
	// Get basePath from context (set by web.go middleware)
	basePath := "/"
	if bp, exists := c.Get("base_path"); exists {
		basePath = bp.(string)
	}
	
	command, err := s.slaveService.GenerateInstallCommand(id, c.Request, basePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": gin.H{"command": command}})
}

var slaveUpgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *SlaveController) connectSlave(c *gin.Context) {
    secret := c.Query("secret")
    slave, err := s.slaveService.GetSlaveBySecret(secret)
    if err != nil {
         c.JSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "Invalid secret"})
         return
    }
    
    ws, err := slaveUpgrader.Upgrade(c.Writer, c.Request, nil)
    if err != nil {
        return
    }
    
    s.slaveService.AddSlaveConn(slave.Id, ws)
    
    // Initial Config Push
    s.slaveService.PushConfig(slave.Id)

    for {
        _, msg, err := ws.ReadMessage()
        if err != nil {
            break
        }
        
        // Try to parse message as JSON
        var msgData map[string]interface{}
        if err := json.Unmarshal(msg, &msgData); err == nil {
            // Check message type
            if msgType, ok := msgData["type"].(string); ok {
                switch msgType {
                case "traffic_stats":
                    s.slaveService.ProcessTrafficStats(slave.Id, msgData)
                    continue
                case "cert_report":
                    s.slaveService.ProcessCertReport(slave.Id, msgData)
                    continue
                }
            }
        }
        
        // Otherwise treat as system stats
        s.slaveService.UpdateSlaveStatus(slave.Id, "online", string(msg))
        logger.Debug("Received from slave %d: %s", slave.Id, string(msg))
    }
    
    s.slaveService.RemoveSlaveConn(slave.Id)
    s.slaveService.UpdateSlaveStatus(slave.Id, "offline", "")
}
