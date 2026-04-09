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

// getSlaves retrieves all slave nodes with traffic info.
// @Summary List slaves
// @Description Returns all slave nodes with their system stats and traffic
// @Tags Slaves
// @Produce json
// @Success 200 {object} entity.Msg
// @Router /panel/api/slave/list [get]
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

// addSlave adds a new slave node.
// @Summary Add slave
// @Description Registers a new slave node
// @Tags Slaves
// @Accept json
// @Produce json
// @Param slave body model.Slave true "Slave data"
// @Success 200 {object} entity.Msg
// @Router /panel/api/slave/add [post]
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

// delSlave deletes a slave node and all associated data.
// @Summary Delete slave
// @Description Deletes a slave node with cascade deletion of all associated data
// @Tags Slaves
// @Produce json
// @Param id path int true "Slave ID"
// @Success 200 {object} entity.Msg
// @Router /panel/api/slave/del/{id} [post]
func (s *SlaveController) delSlave(c *gin.Context) {
	if !session.IsLogin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "unauthorized"})
		return
	}
    id, _ := strconv.Atoi(c.Param("id"))
    
    logger.Infof("Deleting slave %d with cascade", id)
    
    // DeleteSlave now handles all cascade deletions:
    // - inbounds, clients, traffics, IPs, account associations
    // - slave certs, outbound traffics, slave settings
    if err := s.slaveService.DeleteSlave(id); err != nil {
         logger.Errorf("Failed to delete slave %d: %v", id, err)
         c.JSON(http.StatusInternalServerError, gin.H{"success": false, "msg": err.Error()})
         return
    }
    
    logger.Infof("Successfully deleted slave %d", id)
    c.JSON(http.StatusOK, gin.H{"success": true, "msg": "Slave deleted"})
}

// getInstallCommand generates an install command for a slave node.
// @Summary Get install command
// @Description Generates a bash install command for setting up a slave node
// @Tags Slaves
// @Produce json
// @Param id path int true "Slave ID"
// @Success 200 {object} entity.Msg
// @Router /panel/api/slave/install/{id} [get]
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

// connectSlave handles WebSocket connections from slave nodes.
// @Summary Connect slave (WebSocket)
// @Description WebSocket endpoint for slave-to-master communication
// @Tags Slaves
// @Param secret query string true "Slave secret key"
// @Router /panel/api/slave/connect [get]
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
