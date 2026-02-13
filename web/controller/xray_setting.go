package controller

import (
	"fmt"
	"strconv"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"

	"github.com/gin-gonic/gin"
)

// XraySettingController handles Xray configuration and settings operations.
type XraySettingController struct {
	XraySettingService  service.XraySettingService
	SlaveSettingService service.SlaveSettingService
	SettingService      service.SettingService
	InboundService      service.InboundService
	OutboundService     service.OutboundService
	XrayService         service.XrayService
	WarpService         service.WarpService
}

// NewXraySettingController creates a new XraySettingController and initializes its routes.
func NewXraySettingController(g *gin.RouterGroup) *XraySettingController {
	a := &XraySettingController{}
	a.initRouter(g)
	return a
}

// initRouter sets up the routes for Xray settings management.
func (a *XraySettingController) initRouter(g *gin.RouterGroup) {
	g.GET("/getDefaultJsonConfig", a.getDefaultXrayConfig)
	g.GET("/getOutboundsTraffic", a.getOutboundsTraffic)
	g.GET("/getXrayResult", a.getXrayResult)

	g.POST("/", a.getXraySetting)
	g.POST("/warp/:action", a.warp)
	g.POST("/update", a.updateSetting)
	g.POST("/resetOutboundsTraffic", a.resetOutboundsTraffic)
	g.POST("/testOutbound", a.testOutbound)
}

// getXraySetting retrieves the Xray configuration template, inbound tags, and outbound test URL.
func (a *XraySettingController) getXraySetting(c *gin.Context) {
	slaveIdStr := c.PostForm("slaveId")
	slaveId, _ := strconv.Atoi(slaveIdStr)
	
	if slaveId <= 0 {
		jsonMsg(c, "请选择一个Slave节点", fmt.Errorf("slaveId is required"))
		return
	}
	
	// Use SlaveSettingService to get per-slave configuration
	xraySetting, err := a.SlaveSettingService.GetXrayConfigForSlave(slaveId)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	inboundTags, err := a.InboundService.GetInboundTags()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	outboundTestUrl, _ := a.SettingService.GetXrayOutboundTestUrl()
	if outboundTestUrl == "" {
		outboundTestUrl = "https://www.google.com/generate_204"
	}
	xrayResponse := map[string]interface{}{
		"xraySetting":     json.RawMessage(xraySetting),
		"inboundTags":     json.RawMessage(inboundTags),
		"outboundTestUrl": outboundTestUrl,
	}
	result, err := json.Marshal(xrayResponse)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, string(result), nil)
}

// updateSetting updates the Xray configuration settings.
func (a *XraySettingController) updateSetting(c *gin.Context) {
	slaveIdStr := c.PostForm("slaveId")
	slaveId, _ := strconv.Atoi(slaveIdStr)
	
	if slaveId <= 0 {
		jsonMsg(c, "请选择一个Slave节点", fmt.Errorf("slaveId is required"))
		return
	}
	
	// Use SlaveSettingService to save per-slave configuration
	xraySetting := c.PostForm("xraySetting")
	
	// Validate config first
	if err := a.XraySettingService.CheckXrayConfig(xraySetting); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	
	err := a.SlaveSettingService.SaveXrayConfigForSlave(slaveId, xraySetting)
	jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
}

// getDefaultXrayConfig retrieves the default Xray configuration.
func (a *XraySettingController) getDefaultXrayConfig(c *gin.Context) {
	defaultJsonConfig, err := a.SettingService.GetDefaultXrayConfig()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, defaultJsonConfig, nil)
}

// getXrayResult retrieves the current Xray service result.
func (a *XraySettingController) getXrayResult(c *gin.Context) {
	jsonObj(c, a.XrayService.GetXrayResult(), nil)
}

// warp handles Warp-related operations based on the action parameter.
func (a *XraySettingController) warp(c *gin.Context) {
	action := c.Param("action")
	var resp string
	var err error
	switch action {
	case "data":
		resp, err = a.WarpService.GetWarpData()
	case "del":
		err = a.WarpService.DelWarpData()
	case "config":
		resp, err = a.WarpService.GetWarpConfig()
	case "reg":
		skey := c.PostForm("privateKey")
		pkey := c.PostForm("publicKey")
		resp, err = a.WarpService.RegWarp(skey, pkey)
	case "license":
		license := c.PostForm("license")
		resp, err = a.WarpService.SetWarpLicense(license)
	}

	jsonObj(c, resp, err)
}

// getOutboundsTraffic retrieves the traffic statistics for outbounds.
func (a *XraySettingController) getOutboundsTraffic(c *gin.Context) {
	slaveIdStr := c.DefaultQuery("slaveId", "-1")
	slaveId, _ := strconv.Atoi(slaveIdStr)
	
	var outboundsTraffic []*model.OutboundTraffics
	var err error
	
	if slaveId == -1 {
		outboundsTraffic, err = a.OutboundService.GetOutboundsTraffic()
	} else {
		outboundsTraffic, err = a.OutboundService.GetOutboundsTrafficForSlave(slaveId)
	}
	
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getOutboundTrafficError"), err)
		return
	}
	jsonObj(c, outboundsTraffic, nil)
}

// resetOutboundsTraffic resets the traffic statistics for the specified outbound tag.
func (a *XraySettingController) resetOutboundsTraffic(c *gin.Context) {
	tag := c.PostForm("tag")
	slaveIdStr := c.PostForm("slaveId")
	slaveId, _ := strconv.Atoi(slaveIdStr)
	
	if slaveId <= 0 {
		jsonMsg(c, "请选择一个Slave节点", fmt.Errorf("slaveId is required"))
		return
	}
	
	err := a.OutboundService.ResetOutboundTrafficForSlave(slaveId, tag)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.resetOutboundTrafficError"), err)
		return
	}
	jsonObj(c, "", nil)
}

// testOutbound tests an outbound configuration and returns the delay/response time.
// Optional form "allOutbounds": JSON array of all outbounds; used to resolve sockopt.dialerProxy dependencies.
func (a *XraySettingController) testOutbound(c *gin.Context) {
	outboundJSON := c.PostForm("outbound")
	allOutboundsJSON := c.PostForm("allOutbounds")

	if outboundJSON == "" {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewError("outbound parameter is required"))
		return
	}

	// Load the test URL from server settings to prevent SSRF via user-controlled URLs
	testURL, _ := a.SettingService.GetXrayOutboundTestUrl()

	result, err := a.OutboundService.TestOutbound(outboundJSON, testURL, allOutboundsJSON)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}

	jsonObj(c, result, nil)
}
