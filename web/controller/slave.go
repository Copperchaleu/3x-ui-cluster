package controller

import (
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

func NewSlaveController(g *gin.RouterGroup) *SlaveController {
	s := &SlaveController{}
	s.initRouter(g)
	return s
}

func (s *SlaveController) initRouter(g *gin.RouterGroup) {
	g.GET("/list", s.getSlaves)
	g.POST("/add", s.addSlave)
	g.POST("/del/:id", s.delSlave)
	g.GET("/install/:id", s.getInstallCommand)
    g.GET("/connect", s.connectSlave) 
}

func (s *SlaveController) getSlaves(c *gin.Context) {
	if !session.IsLogin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "unauthorized"})
		return
	}
    slaves, err := s.slaveService.GetAllSlaves()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "msg": err.Error()})
        return
    }
	if !session.IsLogin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "unauthorized"})
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
    
    logger.Infof("Slave added successfully: id=%d, name=%s", slave.Id, slave.Name)
    c.JSON(http.StatusOK, gin.H{"success": true, "msg": "Slave added", "obj": slave})
}

func (s *SlaveController) delSlave(c *gin.Context) {
	if !session.IsLogin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "unauthorized"})
		return
	}
    id, _ := strconv.Atoi(c.Param("id"))
    if err := s.slaveService.DeleteSlave(id); err != nil {
         c.JSON(http.StatusInternalServerError, gin.H{"success": false, "msg": err.Error()})
         return
    }
    c.JSON(http.StatusOK, gin.H{"success": true, "msg": "Slave deleted"})
}

func (s *SlaveController) getInstallCommand(c *gin.Context) {
	if !session.IsLogin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "msg": "unauthorized"})
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	command, err := s.slaveService.GenerateInstallCommand(id, c.Request)
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
        // Assuming Slave sends stats JSON
        s.slaveService.UpdateSlaveStatus(slave.Id, "online", string(msg))
        logger.Debug("Received from slave %d: %s", slave.Id, string(msg))
    }
    
    s.slaveService.RemoveSlaveConn(slave.Id)
    s.slaveService.UpdateSlaveStatus(slave.Id, "offline", "")
}
