package controller

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

type RoutingController struct {
	routingService service.RoutingService
}

func NewRoutingController(g *gin.RouterGroup) *RoutingController {
	a := &RoutingController{}
	a.initRouter(g)
	return a
}

func (a *RoutingController) initRouter(g *gin.RouterGroup) {
	g.GET("/list", a.getRoutingRules)
	g.POST("/add", a.addRoutingRule)
	g.POST("/update", a.updateRoutingRule)
	g.POST("/del/:id", a.deleteRoutingRule)
}

func (a *RoutingController) getRoutingRules(c *gin.Context) {
	slaveIdStr := c.DefaultQuery("slaveId", "-1")
	slaveId, _ := strconv.Atoi(slaveIdStr)

	var list []*model.XrayRoutingRule
	var err error

	if slaveId == -1 {
		list, err = a.routingService.GetAllRoutingRules()
	} else {
		list, err = a.routingService.GetRoutingRules(slaveId)
	}

	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, list, nil)
}

func (a *RoutingController) addRoutingRule(c *gin.Context) {
	var rule model.XrayRoutingRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		jsonMsg(c, I18nWeb(c, "error"), err)
		return
	}
	err := a.routingService.AddRoutingRule(&rule)
	if err == nil {
		// Push config to slave if it's not master (slaveId != 0)
		if rule.SlaveId != 0 {
			slaveService := service.SlaveService{}
			slaveService.PushConfig(rule.SlaveId)
		}
	}
	jsonMsg(c, I18nWeb(c, "success"), err)
}

func (a *RoutingController) updateRoutingRule(c *gin.Context) {
	var rule model.XrayRoutingRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		jsonMsg(c, I18nWeb(c, "error"), err)
		return
	}
	
	// Check if the rule exists before updating
	_, err := a.routingService.GetRoutingRuleById(rule.Id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), fmt.Errorf("routing rule not found: %v", err))
		return
	}
	
	err = a.routingService.UpdateRoutingRule(&rule)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	
	jsonMsg(c, I18nWeb(c, "success"), err)
}

func (a *RoutingController) deleteRoutingRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "error"), err)
		return
	}
	// Get the rule to find its slaveId before deleting
	rule, err := a.routingService.GetRoutingRuleById(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "error"), err)
		return
	}
	
	err = a.routingService.DeleteRoutingRule(id)
	if err == nil && rule.SlaveId != 0 {
		// Push config to slave if it's not master (slaveId != 0)
		slaveService := service.SlaveService{}
		slaveService.PushConfig(rule.SlaveId)
	}
	jsonMsg(c, I18nWeb(c, "success"), err)
}
