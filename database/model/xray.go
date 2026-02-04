package model

import (
	"strings"

	"github.com/mhsanaei/3x-ui/v2/util/json_util"
)

// XrayOutbound represents a single Xray outbound configuration in the database
type XrayOutbound struct {
	Id             int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	SlaveId        int    `json:"slaveId" form:"slaveId" gorm:"not null;uniqueIndex:idx_outbound_slave_tag"`
	Tag            string `json:"tag" form:"tag" gorm:"uniqueIndex:idx_outbound_slave_tag"`
	Protocol       string `json:"protocol" form:"protocol"`
	Settings       string `json:"settings" form:"settings"`             // JSON
	StreamSettings string `json:"streamSettings" form:"streamSettings"` // JSON
	Mux            string `json:"mux" form:"mux"`                       // JSON
	ProxySettings  string `json:"proxySettings" form:"proxySettings"`   // JSON
	SendThrough    string `json:"sendThrough" form:"sendThrough"`
	Enable         bool   `json:"enable" form:"enable" gorm:"default:true"`
}

func (XrayOutbound) TableName() string {
	return "xray_outbounds"
}

// GenXrayOutboundConfig generates an Xray outbound configuration struct
func (o *XrayOutbound) GenXrayOutboundConfig() map[string]interface{} {
	config := make(map[string]interface{})
	config["tag"] = o.Tag
	config["protocol"] = o.Protocol

	// Only add sendThrough if it's not empty
	if o.SendThrough != "" {
		config["sendThrough"] = o.SendThrough
	}

	if o.Settings != "" {
		config["settings"] = json_util.RawMessage(o.Settings)
	}
	if o.StreamSettings != "" {
		config["streamSettings"] = json_util.RawMessage(o.StreamSettings)
	}
	if o.Mux != "" {
		config["mux"] = json_util.RawMessage(o.Mux)
	}
	// Only add proxySettings if it's not empty and not just "{}"
	if o.ProxySettings != "" && strings.TrimSpace(o.ProxySettings) != "{}" {
		config["proxySettings"] = json_util.RawMessage(o.ProxySettings)
	}
	return config
}

// XrayRoutingRule represents a single Xray routing rule in the database
type XrayRoutingRule struct {
	Id          int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	SlaveId     int    `json:"slaveId" form:"slaveId" gorm:"not null"`
	Type        string `json:"type" form:"type"`
	Domain      string `json:"domain" form:"domain"`         // JSON array
	Ip          string `json:"ip" form:"ip"`                 // JSON array
	Port        string `json:"port" form:"port"`
	Network     string `json:"network" form:"network"`
	Source      string `json:"source" form:"source"`         // JSON array
	User        string `json:"user" form:"user"`             // JSON array
	InboundTag  string `json:"inboundTag" form:"inboundTag"` // JSON array
	OutboundTag string `json:"outboundTag" form:"outboundTag"`
	BalancerTag string `json:"balancerTag" form:"balancerTag"`
	Attributes  string `json:"attributes" form:"attributes"`
	Sort        int    `json:"sort" form:"sort" gorm:"default:0"`
}

func (XrayRoutingRule) TableName() string {
	return "xray_routing_rules"
}

// GenXrayRoutingRuleConfig generates an Xray routing rule configuration struct
func (r *XrayRoutingRule) GenXrayRoutingRuleConfig() map[string]interface{} {
	rule := map[string]interface{}{
		"type":        r.Type,
		"port":        r.Port,
		"network":     r.Network,
		"outboundTag": r.OutboundTag,
		"balancerTag": r.BalancerTag,
		// "attrs":    r.Attributes, // Needs parsing if string
	}

	if r.Domain != "" {
		rule["domain"] = json_util.RawMessage(r.Domain)
	}
	if r.Ip != "" {
		rule["ip"] = json_util.RawMessage(r.Ip)
	}
	if r.Source != "" {
		rule["source"] = json_util.RawMessage(r.Source)
	}
	if r.User != "" {
		rule["user"] = json_util.RawMessage(r.User)
	}
	if r.InboundTag != "" {
		rule["inboundTag"] = json_util.RawMessage(r.InboundTag)
	}

	return rule
}
