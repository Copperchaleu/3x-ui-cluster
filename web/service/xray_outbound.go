package service

import (
	"fmt"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func (s *OutboundService) GetOutbounds(slaveId int) ([]*model.XrayOutbound, error) {
	db := database.GetDB()
	var outbounds []*model.XrayOutbound
	err := db.Model(&model.XrayOutbound{}).Where("slave_id = ?", slaveId).Find(&outbounds).Error
	return outbounds, err
}

func (s *OutboundService) GetAllOutbounds() ([]*model.XrayOutbound, error) {
	db := database.GetDB()
	var outbounds []*model.XrayOutbound
	err := db.Model(&model.XrayOutbound{}).Find(&outbounds).Error
	return outbounds, err
}

func (s *OutboundService) GetOutboundById(id int) (*model.XrayOutbound, error) {
	db := database.GetDB()
	var outbound model.XrayOutbound
	err := db.First(&outbound, id).Error
	return &outbound, err
}

func (s *OutboundService) AddOutbound(outbound *model.XrayOutbound) error {
	db := database.GetDB()
	return db.Create(outbound).Error
}

func (s *OutboundService) UpdateOutbound(outbound *model.XrayOutbound) error {
	db := database.GetDB()
	
	fmt.Printf("DEBUG SERVICE: Updating outbound ID=%d, SlaveId=%d, Tag=%s\n", outbound.Id, outbound.SlaveId, outbound.Tag)
	
	// First get the existing record to check if it exists and get the old slaveId
	var oldOutbound model.XrayOutbound
	err := db.Where("id = ?", outbound.Id).First(&oldOutbound).Error
	if err != nil {
		fmt.Printf("DEBUG SERVICE: Failed to find existing outbound ID=%d: %v\n", outbound.Id, err)
		return err // Return the "record not found" error
	}
	
	fmt.Printf("DEBUG SERVICE: Found existing outbound: ID=%d, SlaveId=%d, Tag=%s\n", oldOutbound.Id, oldOutbound.SlaveId, oldOutbound.Tag)
	
	// Save the old slaveId for pushing config later
	oldSlaveId := oldOutbound.SlaveId
	
	// Check if slaveId changed
	if oldOutbound.SlaveId != outbound.SlaveId {
		fmt.Printf("DEBUG SERVICE: SlaveId changed from %d to %d, deleting old and creating new\n", oldOutbound.SlaveId, outbound.SlaveId)
		
		// Delete the old record
		err = db.Delete(&oldOutbound).Error
		if err != nil {
			fmt.Printf("DEBUG SERVICE: Failed to delete old outbound: %v\n", err)
			return err
		}
		
		// Create new record (ID will be auto-assigned)
		err = db.Create(outbound).Error
		if err != nil {
			fmt.Printf("DEBUG SERVICE: Failed to create new outbound: %v\n", err)
			return err
		}
		
		fmt.Printf("DEBUG SERVICE: Created new outbound with ID=%d\n", outbound.Id)
	} else {
		// Same slaveId, just update the record
		err = db.Model(&model.XrayOutbound{}).Where("id = ?", outbound.Id).Save(outbound).Error
		if err != nil {
			fmt.Printf("DEBUG SERVICE: Save failed: %v\n", err)
			return err
		}
		fmt.Printf("DEBUG SERVICE: Updated existing outbound\n")
	}
	
	// Push config to both old and new slave if they're different and not master
	slaveService := SlaveService{}
	if oldSlaveId != 0 {
		slaveService.PushConfig(oldSlaveId)
	}
	if outbound.SlaveId != 0 && outbound.SlaveId != oldSlaveId {
		slaveService.PushConfig(outbound.SlaveId)
	}
	return nil
}

func (s *OutboundService) DeleteOutbound(id int) error {
	db := database.GetDB()
	return db.Delete(&model.XrayOutbound{}, id).Error
}
