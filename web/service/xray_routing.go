package service

import (
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

type RoutingService struct{}

func (s *RoutingService) GetRoutingRules(slaveId int) ([]*model.XrayRoutingRule, error) {
	db := database.GetDB()
	var rules []*model.XrayRoutingRule
	err := db.Model(&model.XrayRoutingRule{}).Where("slave_id = ?", slaveId).Order("sort asc").Find(&rules).Error
	return rules, err
}

func (s *RoutingService) GetAllRoutingRules() ([]*model.XrayRoutingRule, error) {
	db := database.GetDB()
	var rules []*model.XrayRoutingRule
	err := db.Model(&model.XrayRoutingRule{}).Order("sort asc").Find(&rules).Error
	return rules, err
}

func (s *RoutingService) GetRoutingRuleById(id int) (*model.XrayRoutingRule, error) {
	db := database.GetDB()
	var rule model.XrayRoutingRule
	err := db.First(&rule, id).Error
	return &rule, err
}

func (s *RoutingService) AddRoutingRule(rule *model.XrayRoutingRule) error {
	db := database.GetDB()
	return db.Create(rule).Error
}

func (s *RoutingService) UpdateRoutingRule(rule *model.XrayRoutingRule) error {
	db := database.GetDB()
	
	// First get the existing record to check if it exists and get the old slaveId
	var oldRule model.XrayRoutingRule
	err := db.Where("id = ?", rule.Id).First(&oldRule).Error
	if err != nil {
		return err // Return the "record not found" error
	}
	
	// Save the old slaveId for pushing config later
	oldSlaveId := oldRule.SlaveId
	
	// Check if slaveId changed
	if oldRule.SlaveId != rule.SlaveId {
		// Delete the old record
		err = db.Delete(&oldRule).Error
		if err != nil {
			return err
		}
		
		// Create new record (ID will be auto-assigned)
		err = db.Create(rule).Error
		if err != nil {
			return err
		}
	} else {
		// Same slaveId, just update the record
		err = db.Model(&model.XrayRoutingRule{}).Where("id = ?", rule.Id).Save(rule).Error
		if err != nil {
			return err
		}
	}
	
	// Push config to both old and new slave if they're different and not master
	slaveService := SlaveService{}
	if oldSlaveId != 0 {
		slaveService.PushConfig(oldSlaveId)
	}
	if rule.SlaveId != 0 && rule.SlaveId != oldSlaveId {
		slaveService.PushConfig(rule.SlaveId)
	}
	return err
}

func (s *RoutingService) DeleteRoutingRule(id int) error {
	db := database.GetDB()
	return db.Delete(&model.XrayRoutingRule{}, id).Error
}
