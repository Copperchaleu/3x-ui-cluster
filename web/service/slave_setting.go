package service

import (
	"fmt"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
)

// SlaveSettingService provides business logic for slave-specific settings management.
type SlaveSettingService struct {
	SettingService
}

// GetSettingForSlave retrieves a specific setting value for a slave.
// If the setting doesn't exist for the slave, returns the global default.
func (s *SlaveSettingService) GetSettingForSlave(slaveId int, key string) (string, error) {
	db := database.GetDB()
	
	var slaveSetting model.SlaveSetting
	err := db.Where("slave_id = ? AND setting_key = ?", slaveId, key).
		First(&slaveSetting).Error
	
	if err != nil {
		// If not found for this slave, try to get global setting as fallback
		logger.Infof("Setting %s not found for slave %d, falling back to global setting", key, slaveId)
		return s.SettingService.getString(key)
	}
	
	return slaveSetting.SettingValue, nil
}

// SaveSettingForSlave saves or updates a setting for a specific slave.
func (s *SlaveSettingService) SaveSettingForSlave(slaveId int, key string, value string) error {
	if slaveId <= 0 {
		return fmt.Errorf("invalid slaveId: %d", slaveId)
	}
	
	db := database.GetDB()
	
	var slaveSetting model.SlaveSetting
	err := db.Where("slave_id = ? AND setting_key = ?", slaveId, key).
		First(&slaveSetting).Error
	
	if err != nil {
		// Create new setting
		slaveSetting = model.SlaveSetting{
			SlaveId:      slaveId,
			SettingKey:   key,
			SettingValue: value,
		}
		return db.Create(&slaveSetting).Error
	}
	
	// Update existing setting
	slaveSetting.SettingValue = value
	return db.Save(&slaveSetting).Error
}

// GetXrayConfigForSlave retrieves the xrayTemplateConfig for a specific slave.
func (s *SlaveSettingService) GetXrayConfigForSlave(slaveId int) (string, error) {
	return s.GetSettingForSlave(slaveId, "xrayTemplateConfig")
}

// SaveXrayConfigForSlave saves the xrayTemplateConfig for a specific slave.
func (s *SlaveSettingService) SaveXrayConfigForSlave(slaveId int, config string) error {
	return s.SaveSettingForSlave(slaveId, "xrayTemplateConfig", config)
}

// DeleteAllSettingsForSlave deletes all settings for a specific slave.
// This should be called when a slave is deleted.
func (s *SlaveSettingService) DeleteAllSettingsForSlave(slaveId int) error {
	db := database.GetDB()
	return db.Where("slave_id = ?", slaveId).Delete(&model.SlaveSetting{}).Error
}

// CopySettingsToNewSlave copies all settings from one slave to another.
// Useful when creating a new slave based on an existing one.
func (s *SlaveSettingService) CopySettingsToNewSlave(fromSlaveId, toSlaveId int) error {
	if toSlaveId <= 0 {
		return fmt.Errorf("invalid target slaveId: %d", toSlaveId)
	}
	
	db := database.GetDB()
	
	var sourceSettings []model.SlaveSetting
	err := db.Where("slave_id = ?", fromSlaveId).Find(&sourceSettings).Error
	if err != nil {
		return fmt.Errorf("failed to get source slave settings: %v", err)
	}
	
	for _, setting := range sourceSettings {
		newSetting := model.SlaveSetting{
			SlaveId:      toSlaveId,
			SettingKey:   setting.SettingKey,
			SettingValue: setting.SettingValue,
		}
		if err := db.Create(&newSetting).Error; err != nil {
			logger.Warningf("Failed to copy setting %s to slave %d: %v", setting.SettingKey, toSlaveId, err)
		}
	}
	
	return nil
}

// InitializeSlaveWithDefaults initializes a new slave with default settings.
// Copies the global xrayTemplateConfig to the new slave.
func (s *SlaveSettingService) InitializeSlaveWithDefaults(slaveId int) error {
	if slaveId <= 0 {
		return fmt.Errorf("invalid slaveId: %d", slaveId)
	}
	
	// Get global xrayTemplateConfig
	globalConfig, err := s.SettingService.GetXrayConfigTemplate()
	if err != nil {
		return fmt.Errorf("failed to get global xrayTemplateConfig: %v", err)
	}
	
	// Save to slave_settings
	return s.SaveXrayConfigForSlave(slaveId, globalConfig)
}
