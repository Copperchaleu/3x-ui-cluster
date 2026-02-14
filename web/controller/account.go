package controller

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

// AccountController handles HTTP requests for account management operations.
type AccountController struct {
	BaseController

	accountService service.AccountService
	slaveService   service.SlaveService
}

// NewAccountController creates a new account controller instance.
func NewAccountController(g *gin.RouterGroup) *AccountController {
	a := &AccountController{}
	a.initRouter(g)
	return a
}

func (a *AccountController) initRouter(g *gin.RouterGroup) {
	// Account CRUD
	g.GET("/list", a.getAccounts)
	g.POST("/add", a.addAccount)
	g.POST("/update/:id", a.updateAccount)
	g.POST("/del/:id", a.delAccount)
	g.GET("/get/:id", a.getAccount)

	// Client management
	g.GET("/:id/clients", a.getAccountClients)
	g.POST("/:id/clients/add", a.addClientToAccount)
	g.POST("/:id/clients/remove/:clientEmail", a.removeClientFromAccount)

	// Traffic management
	g.GET("/:id/traffic", a.getAccountTraffic)
	g.POST("/:id/traffic/reset", a.resetAccountTraffic)
}

// getAccounts retrieves all accounts.
// @route GET /panel/api/account/list
func (a *AccountController) getAccounts(c *gin.Context) {
	accounts, err := a.accountService.GetAccounts()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.getAccounts"), err)
		return
	}
	jsonObj(c, accounts, nil)
}

// getAccount retrieves a single account by ID.
// @route GET /panel/api/account/get/:id
func (a *AccountController) getAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.getAccount"), err)
		return
	}

	account, err := a.accountService.GetAccount(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.getAccount"), err)
		return
	}

	jsonObj(c, account, nil)
}

// addAccount creates a new account.
// @route POST /panel/api/account/add
func (a *AccountController) addAccount(c *gin.Context) {
	account := &model.Account{}
	err := c.ShouldBind(account)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.addAccount"), err)
		return
	}

	err = a.accountService.AddAccount(account)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.addAccount"), err)
		return
	}

	jsonMsgObj(c, I18nWeb(c, "pages.accounts.toasts.addAccount"), account, nil)
}

// updateAccount updates an existing account.
// @route POST /panel/api/account/update/:id
func (a *AccountController) updateAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.updateAccount"), err)
		return
	}

	account := &model.Account{}
	err = c.ShouldBind(account)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.updateAccount"), err)
		return
	}

	account.Id = id
	err = a.accountService.UpdateAccount(account)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.updateAccount"), err)
		return
	}

	// Push config to all slaves that have clients associated with this account
	affectedSlaves, err := a.accountService.GetAccountAffectedSlaves(account.Id)
	if err == nil {
		for _, slaveId := range affectedSlaves {
			if pushErr := a.slaveService.PushConfig(slaveId); pushErr != nil {
				logger.Errorf("Failed to push config to slave %d after account update: %v", slaveId, pushErr)
			} else {
				logger.Infof("Pushed config to slave %d after updating account %d", slaveId, account.Id)
			}
		}
	} else {
		logger.Warningf("Failed to get affected slaves for account %d: %v", account.Id, err)
	}

	jsonMsgObj(c, I18nWeb(c, "pages.accounts.toasts.updateAccount"), account, nil)
}

// delAccount deletes an account and its associations.
// @route POST /panel/api/account/del/:id
func (a *AccountController) delAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.delAccount"), err)
		return
	}

	// Get affected slaves before deletion
	affectedSlaves, _ := a.accountService.GetAccountAffectedSlaves(id)

	err = a.accountService.DelAccount(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.delAccount"), err)
		return
	}

	// Push config to affected slaves after deletion
	for _, slaveId := range affectedSlaves {
		if pushErr := a.slaveService.PushConfig(slaveId); pushErr != nil {
			logger.Errorf("Failed to push config to slave %d after account deletion: %v", slaveId, pushErr)
		} else {
			logger.Infof("Pushed config to slave %d after deleting account %d", slaveId, id)
		}
	}

	jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.delAccount"), nil)
}

// getAccountClients retrieves all clients associated with an account.
// @route GET /panel/api/account/:id/clients
func (a *AccountController) getAccountClients(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.getClients"), err)
		return
	}

	clients, err := a.accountService.GetAccountClients(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.getClients"), err)
		return
	}

	jsonObj(c, clients, nil)
}

// addClientToAccount associates a client with an account.
// @route POST /panel/api/account/:id/clients/add
func (a *AccountController) addClientToAccount(c *gin.Context) {
	accountId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.addClient"), err)
		return
	}

	data := struct {
		InboundId   int          `json:"inboundId" form:"inboundId"`
		Client      model.Client `json:"client" form:"client"`
		ClientEmail string       `json:"clientEmail" form:"clientEmail"` // For existing clients
	}{}

	err = c.ShouldBind(&data)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.addClient"), err)
		return
	}

	// If clientEmail is provided, use it (for existing clients)
	if data.ClientEmail != "" {
		data.Client.Email = data.ClientEmail
	}

	err = a.accountService.AddClientToAccount(accountId, data.InboundId, &data.Client)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.addClient"), err)
		return
	}

	// Push config to the slave after adding client
	inboundService := &service.InboundService{}
	inbound, getErr := inboundService.GetInbound(data.InboundId)
	if getErr == nil && inbound.SlaveId > 0 {
		if pushErr := a.slaveService.PushConfig(inbound.SlaveId); pushErr != nil {
			logger.Errorf("Failed to push config to slave %d after adding client to account: %v", inbound.SlaveId, pushErr)
		} else {
			logger.Infof("Pushed config to slave %d after adding client to account %d", inbound.SlaveId, accountId)
		}
	}

	jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.addClient"), nil)
}

// removeClientFromAccount removes a client from an account.
// @route POST /panel/api/account/:id/clients/remove/:clientEmail
func (a *AccountController) removeClientFromAccount(c *gin.Context) {
	accountId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.removeClient"), err)
		return
	}

	clientEmail := c.Param("clientEmail")
	if clientEmail == "" {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.removeClient"), errors.New("Client email is required"))
		return
	}

	// Get affected slaves before removal
	affectedSlaves, _ := a.accountService.GetAccountAffectedSlaves(accountId)

	err = a.accountService.RemoveClientFromAccount(accountId, clientEmail)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.removeClient"), err)
		return
	}

	// Push config to affected slaves after removal
	for _, slaveId := range affectedSlaves {
		if pushErr := a.slaveService.PushConfig(slaveId); pushErr != nil {
			logger.Errorf("Failed to push config to slave %d after removing client from account: %v", slaveId, pushErr)
		} else {
			logger.Infof("Pushed config to slave %d after removing client from account %d", slaveId, accountId)
		}
	}

	jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.removeClient"), nil)
}

// getAccountTraffic retrieves aggregated traffic statistics for an account.
// @route GET /panel/api/account/:id/traffic
func (a *AccountController) getAccountTraffic(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.getTraffic"), err)
		return
	}

	up, down, err := a.accountService.GetAccountTraffic(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.getTraffic"), err)
		return
	}

	jsonObj(c, map[string]interface{}{
		"up":   up,
		"down": down,
		"total": up + down,
	}, nil)
}

// resetAccountTraffic resets traffic for an account.
// @route POST /panel/api/account/:id/traffic/reset
func (a *AccountController) resetAccountTraffic(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.resetTraffic"), err)
		return
	}

	err = a.accountService.ResetAccountTraffic(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.resetTraffic"), err)
		return
	}

	logger.Infof("Reset traffic for account %d", id)
	jsonMsg(c, I18nWeb(c, "pages.accounts.toasts.resetTraffic"), nil)
}
