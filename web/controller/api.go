package controller

import (
	"net/http"

	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/session"

	"github.com/gin-gonic/gin"
)

// APIController handles the main API routes for the 3x-ui panel, including inbounds and server management.
type APIController struct {
	BaseController
	inboundController   *InboundController
	outboundController  *OutboundController
	routingController   *RoutingController
	serverController    *ServerController
	slaveController     *SlaveController
	slaveCertController *SlaveCertController
	accountController   *AccountController
	Tgbot               service.Tgbot
	slaveService        service.SlaveService
}

// NewAPIController creates a new APIController instance and initializes its routes.
func NewAPIController(g *gin.RouterGroup, slaveService service.SlaveService) *APIController {
	a := &APIController{slaveService: slaveService}
	a.initRouter(g)
	return a
}

// checkAPIAuth is a middleware that returns 404 for unauthenticated API requests
// to hide the existence of API endpoints from unauthorized users
func (a *APIController) checkAPIAuth(c *gin.Context) {
	if !session.IsLogin(c) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Next()
}

// initRouter sets up the API routes for inbounds, server, and other endpoints.
func (a *APIController) initRouter(g *gin.RouterGroup) {
	// Slave connect without auth
	slaveController := &SlaveController{slaveService: a.slaveService}
	g.GET("/panel/api/slave/connect", slaveController.connectSlave)

	// Main API group
	api := g.Group("/panel/api")
	api.Use(a.checkAPIAuth)

	// Inbounds API
	inbounds := api.Group("/inbounds")
	a.inboundController = NewInboundController(inbounds)

	// Outbounds API
	outbounds := api.Group("/outbounds")
	a.outboundController = NewOutboundController(outbounds)

	// Routing API
	routing := api.Group("/routing")
	a.routingController = NewRoutingController(routing)

	// Slave API
	slave := api.Group("/slave")
	a.slaveController = NewSlaveController(slave, a.slaveService)

	// Slave Certificate API
	slaveCerts := api.Group("/slave-certs")
	a.slaveCertController = NewSlaveCertController(slaveCerts)

	// Account API (multi-inbound user management)
	accounts := api.Group("/account")
	a.accountController = NewAccountController(accounts)

	// Server API
	server := api.Group("/server")
	a.serverController = NewServerController(server)

	// Extra routes
	api.GET("/backuptotgbot", a.BackuptoTgbot)
}

// BackuptoTgbot sends a backup of the panel data to Telegram bot admins.
func (a *APIController) BackuptoTgbot(c *gin.Context) {
	a.Tgbot.SendBackupToAdmins()
}
