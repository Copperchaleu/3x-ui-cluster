package controller

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

type OutboundController struct {
	outboundService service.OutboundService
}

func NewOutboundController(g *gin.RouterGroup) *OutboundController {
	a := &OutboundController{}
	a.initRouter(g)
	return a
}

func (a *OutboundController) initRouter(g *gin.RouterGroup) {
	g.POST("/list", a.getOutbounds)
	g.POST("/add", a.addOutbound)
	g.POST("/update", a.updateOutbound)
	g.POST("/del/:id", a.deleteOutbound)
}

func (a *OutboundController) getOutbounds(c *gin.Context) {
	slaveIdStr := c.DefaultQuery("slaveId", "-1")
	slaveId, _ := strconv.Atoi(slaveIdStr)

	var list []*model.XrayOutbound
	var err error

	if slaveId == -1 {
		list, err = a.outboundService.GetAllOutbounds()
	} else {
		list, err = a.outboundService.GetOutbounds(slaveId)
	}

	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, list, nil)
}

func (a *OutboundController) addOutbound(c *gin.Context) {
	var outbound model.XrayOutbound
	if err := c.ShouldBindJSON(&outbound); err != nil {
		jsonMsg(c, I18nWeb(c, "error"), err)
		return
	}
	err := a.outboundService.AddOutbound(&outbound)
	if err == nil {
		// Push config to slave if it's not master (slaveId != 0)
		if outbound.SlaveId != 0 {
			slaveService := service.SlaveService{}
			slaveService.PushConfig(outbound.SlaveId)
		}
	}
	jsonMsg(c, I18nWeb(c, "success"), err)
}

func (a *OutboundController) updateOutbound(c *gin.Context) {
	var outbound model.XrayOutbound
	if err := c.ShouldBindJSON(&outbound); err != nil {
		jsonMsg(c, I18nWeb(c, "error"), err)
		return
	}
	
	fmt.Printf("DEBUG: Updating outbound with ID: %d, SlaveId: %d, Tag: %s\n", outbound.Id, outbound.SlaveId, outbound.Tag)
	
	// Check if the outbound exists before updating
	existingOutbound, err := a.outboundService.GetOutboundById(outbound.Id)
	if err != nil {
		fmt.Printf("DEBUG: GetOutboundById failed for ID %d: %v\n", outbound.Id, err)
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), fmt.Errorf("outbound not found: %v", err))
		return
	}
	
	fmt.Printf("DEBUG: Found existing outbound: ID=%d, Tag=%s\n", existingOutbound.Id, existingOutbound.Tag)
	
	err = a.outboundService.UpdateOutbound(&outbound)
	if err != nil {
		fmt.Printf("DEBUG: UpdateOutbound failed: %v\n", err)
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	
	fmt.Printf("DEBUG: UpdateOutbound successful\n")
	jsonMsg(c, I18nWeb(c, "success"), err)
}

func (a *OutboundController) deleteOutbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "error"), err)
		return
	}
	// Get the outbound to find its slaveId before deleting
	outbound, err := a.outboundService.GetOutboundById(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "error"), err)
		return
	}
	
	err = a.outboundService.DeleteOutbound(id)
	if err == nil && outbound.SlaveId != 0 {
		// Push config to slave if it's not master (slaveId != 0)
		slaveService := service.SlaveService{}
		slaveService.PushConfig(outbound.SlaveId)
	}
	jsonMsg(c, I18nWeb(c, "success"), err)
}
