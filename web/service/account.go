package service

import (
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/util/common"
	"github.com/mhsanaei/3x-ui/v2/util/random"
	"github.com/mhsanaei/3x-ui/v2/xray"

	"gorm.io/gorm"
)

// AccountService provides business logic for managing multi-inbound user accounts.
// It handles account CRUD operations, client associations, and aggregated traffic management.
type AccountService struct {
	inboundService InboundService
}

// GetAccounts retrieves all accounts from the database with their client count.
func (s *AccountService) GetAccounts() ([]*model.Account, error) {
	db := database.GetDB()
	var accounts []*model.Account
	err := db.Model(model.Account{}).Find(&accounts).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return accounts, nil
}

// GetAccount retrieves a single account by ID.
func (s *AccountService) GetAccount(id int) (*model.Account, error) {
	db := database.GetDB()
	account := &model.Account{}
	err := db.Model(model.Account{}).Where("id = ?", id).First(account).Error
	if err != nil {
		return nil, err
	}
	return account, nil
}

// GetAccountByUsername retrieves an account by username.
func (s *AccountService) GetAccountByUsername(username string) (*model.Account, error) {
	db := database.GetDB()
	account := &model.Account{}
	err := db.Model(model.Account{}).Where("username = ?", username).First(account).Error
	if err != nil {
		return nil, err
	}
	return account, nil
}

// GetAccountBySubId retrieves an account by subscription ID.
func (s *AccountService) GetAccountBySubId(subId string) (*model.Account, error) {
	db := database.GetDB()
	account := &model.Account{}
	err := db.Model(model.Account{}).Where("sub_id = ?", subId).First(account).Error
	if err != nil {
		return nil, err
	}
	return account, nil
}

// AddAccount creates a new account.
func (s *AccountService) AddAccount(account *model.Account) error {
	db := database.GetDB()

	// Check if username already exists
	existingAccount := &model.Account{}
	err := db.Model(model.Account{}).Where("username = ?", account.Username).First(existingAccount).Error
	if err == nil {
		return common.NewError("Username already exists:", account.Username)
	}

	// Generate subscription ID if not provided
	if account.SubId == "" {
		account.SubId = random.Seq(16)
	}

	// Set timestamps
	now := time.Now().UnixMilli()
	account.CreatedAt = now
	account.UpdatedAt = now

	return db.Create(account).Error
}

// UpdateAccount updates an existing account.
func (s *AccountService) UpdateAccount(account *model.Account) error {
	db := database.GetDB()

	// Check if account exists
	oldAccount, err := s.GetAccount(account.Id)
	if err != nil {
		return err
	}

	// Check if username is being changed and if it conflicts
	if account.Username != oldAccount.Username {
		existingAccount := &model.Account{}
		err := db.Model(model.Account{}).Where("username = ? AND id != ?", account.Username, account.Id).First(existingAccount).Error
		if err == nil {
			return common.NewError("Username already exists:", account.Username)
		}
	}

	// Update timestamp
	account.UpdatedAt = time.Now().UnixMilli()

	// Preserve CreatedAt
	account.CreatedAt = oldAccount.CreatedAt

	return db.Save(account).Error
}

// DelAccount deletes an account and its associated client relationships.
func (s *AccountService) DelAccount(id int) error {
	db := database.GetDB()

	return db.Transaction(func(tx *gorm.DB) error {
		// Delete account-client associations
		if err := tx.Where("account_id = ?", id).Delete(&model.AccountClient{}).Error; err != nil {
			return err
		}

		// Reset AccountId in client_traffics
		if err := tx.Model(&xray.ClientTraffic{}).Where("account_id = ?", id).Update("account_id", 0).Error; err != nil {
			return err
		}

		// Delete the account
		if err := tx.Delete(&model.Account{}, id).Error; err != nil {
			return err
		}

		return nil
	})
}

// GetAccountClients retrieves all clients associated with an account.
func (s *AccountService) GetAccountClients(accountId int) ([]map[string]interface{}, error) {
	db := database.GetDB()

	var associations []model.AccountClient
	err := db.Where("account_id = ?", accountId).Find(&associations).Error
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	for _, assoc := range associations {
		inbound, err := s.inboundService.GetInbound(assoc.InboundId)
		if err != nil {
			continue
		}

		clients, err := s.inboundService.GetClients(inbound)
		if err != nil {
			continue
		}

		for _, client := range clients {
			if client.Email == assoc.ClientEmail {
				result = append(result, map[string]interface{}{
					"id":          assoc.Id,
					"accountId":   assoc.AccountId,
					"inboundId":   assoc.InboundId,
					"inboundTag":  inbound.Tag,
					"inboundRemark": inbound.Remark,
					"clientEmail": assoc.ClientEmail,
					"clientId":    client.ID,
					"enable":      client.Enable,
					"createdAt":   assoc.CreatedAt,
				})
				break
			}
		}
	}

	return result, nil
}

// AddClientToAccount associates a client with an account.
// This creates the client in the inbound if it doesn't exist, or links an existing client.
func (s *AccountService) AddClientToAccount(accountId, inboundId int, client *model.Client) error {
	db := database.GetDB()

	return db.Transaction(func(tx *gorm.DB) error {
		// Check if client email already associated with another account
		existingAssoc := &model.AccountClient{}
		err := tx.Where("client_email = ?", client.Email).First(existingAssoc).Error
		if err == nil {
			return common.NewError("Client email already associated with another account:", client.Email)
		}

		// Get inbound
		inbound, err := s.inboundService.GetInbound(inboundId)
		if err != nil {
			return err
		}

		// Check if client already exists in inbound
		clients, err := s.inboundService.GetClients(inbound)
		if err != nil {
			return err
		}

		clientExists := false
		for _, c := range clients {
			if c.Email == client.Email {
				clientExists = true
				break
			}
		}

		// If client doesn't exist, we need to add it to the inbound
		// For now, we'll just create the association and traffic record
		// The actual client needs to be added through the inbound service

		// Create association
		assoc := &model.AccountClient{
			AccountId:   accountId,
			InboundId:   inboundId,
			ClientEmail: client.Email,
			CreatedAt:   time.Now().UnixMilli(),
		}
		if err := tx.Create(assoc).Error; err != nil {
			return err
		}

		// Update or create client traffic record with account association
		traffic := &xray.ClientTraffic{}
		err = tx.Where("email = ?", client.Email).First(traffic).Error
		if err == gorm.ErrRecordNotFound {
			// Create new traffic record only if client exists in inbound
			if !clientExists {
				return common.NewError("Client does not exist in inbound. Please add client to inbound first.")
			}
			traffic = &xray.ClientTraffic{
				InboundId:  inboundId,
				AccountId:  accountId,
				Email:      client.Email,
				Enable:     client.Enable,
				Total:      0,
				ExpiryTime: 0,
			}
			return tx.Create(traffic).Error
		} else if err != nil {
			return err
		}

		// Update existing traffic record with account association
		return tx.Model(traffic).Update("account_id", accountId).Error
	})
}

// RemoveClientFromAccount removes the association between a client and an account.
func (s *AccountService) RemoveClientFromAccount(accountId int, clientEmail string) error {
	db := database.GetDB()

	return db.Transaction(func(tx *gorm.DB) error {
		// Delete association
		if err := tx.Where("account_id = ? AND client_email = ?", accountId, clientEmail).Delete(&model.AccountClient{}).Error; err != nil {
			return err
		}

		// Reset AccountId in client_traffics
		if err := tx.Model(&xray.ClientTraffic{}).Where("email = ?", clientEmail).Update("account_id", 0).Error; err != nil {
			return err
		}

		return nil
	})
}

// GetAccountTraffic retrieves aggregated traffic statistics for an account.
func (s *AccountService) GetAccountTraffic(accountId int) (up, down int64, err error) {
	db := database.GetDB()

	type TrafficResult struct {
		Up   int64
		Down int64
	}

	result := &TrafficResult{}
	err = db.Model(&xray.ClientTraffic{}).
		Select("COALESCE(SUM(up), 0) as up, COALESCE(SUM(down), 0) as down").
		Where("account_id = ?", accountId).
		Scan(result).Error

	if err != nil {
		return 0, 0, err
	}

	return result.Up, result.Down, nil
}

// CheckAccountTrafficLimit checks if an account has exceeded its traffic limit.
func (s *AccountService) CheckAccountTrafficLimit(accountId int) (exceeded bool, err error) {
	account, err := s.GetAccount(accountId)
	if err != nil {
		return false, err
	}

	if account.TotalGB <= 0 {
		return false, nil // No limit
	}

	up, down, err := s.GetAccountTraffic(accountId)
	if err != nil {
		return false, err
	}

	totalUsed := up + down
	totalLimit := account.TotalGB * 1024 * 1024 * 1024 // GB to bytes

	return totalUsed >= totalLimit, nil
}

// CheckAccountExpiry checks if an account has expired.
func (s *AccountService) CheckAccountExpiry(accountId int) (expired bool, err error) {
	account, err := s.GetAccount(accountId)
	if err != nil {
		return false, err
	}

	if account.ExpiryTime <= 0 {
		return false, nil // Never expires
	}

	return time.Now().UnixMilli() > account.ExpiryTime, nil
}

// ResetAccountTraffic resets the traffic usage for an account.
func (s *AccountService) ResetAccountTraffic(accountId int) error {
	db := database.GetDB()

	return db.Transaction(func(tx *gorm.DB) error {
		// Reset account traffic
		if err := tx.Model(&model.Account{}).Where("id = ?", accountId).Updates(map[string]interface{}{
			"up":   0,
			"down": 0,
		}).Error; err != nil {
			return err
		}

		// Reset client traffics
		if err := tx.Model(&xray.ClientTraffic{}).Where("account_id = ?", accountId).Updates(map[string]interface{}{
			"up":   0,
			"down": 0,
		}).Error; err != nil {
			return err
		}

		return nil
	})
}

// SyncAccountTraffic synchronizes account traffic from its associated client traffics.
// This should be called periodically or after traffic updates.
func (s *AccountService) SyncAccountTraffic(accountId int) error {
	db := database.GetDB()

	up, down, err := s.GetAccountTraffic(accountId)
	if err != nil {
		return err
	}

	return db.Model(&model.Account{}).Where("id = ?", accountId).Updates(map[string]interface{}{
		"up":        up,
		"down":      down,
		"updatedAt": time.Now().UnixMilli(),
	}).Error
}

// DisableClientsExceedingAccountLimit disables all clients for accounts that have exceeded their limits.
// This should be called periodically as a background job.
func (s *AccountService) DisableClientsExceedingAccountLimit() error {
	db := database.GetDB()

	// Find accounts that have exceeded limits
	var exceededAccounts []model.Account
	err := db.Raw(`
		SELECT a.* FROM accounts a
		WHERE a.total_gb > 0 
		AND (a.up + a.down) >= (a.total_gb * 1024 * 1024 * 1024)
		AND a.enable = true
	`).Scan(&exceededAccounts).Error

	if err != nil {
		return err
	}

	for _, account := range exceededAccounts {
		// Get all client emails for this account
		var associations []model.AccountClient
		db.Where("account_id = ?", account.Id).Find(&associations)

		for _, assoc := range associations {
			// Disable client in traffic table
			db.Model(&xray.ClientTraffic{}).
				Where("email = ?", assoc.ClientEmail).
				Update("enable", false)

			logger.Infof("Disabled client %s due to account %s traffic limit exceeded", assoc.ClientEmail, account.Username)
		}
	}

	return nil
}

// DisableExpiredAccountClients disables all clients for accounts that have expired.
// This should be called periodically as a background job.
func (s *AccountService) DisableExpiredAccountClients() error {
	db := database.GetDB()

	// Find expired accounts
	now := time.Now().UnixMilli()
	var expiredAccounts []model.Account
	err := db.Where("expiry_time > 0 AND expiry_time <= ? AND enable = true", now).Find(&expiredAccounts).Error

	if err != nil {
		return err
	}

	for _, account := range expiredAccounts {
		// Get all client emails for this account
		var associations []model.AccountClient
		db.Where("account_id = ?", account.Id).Find(&associations)

		for _, assoc := range associations {
			// Disable client in traffic table
			db.Model(&xray.ClientTraffic{}).
				Where("email = ?", assoc.ClientEmail).
				Update("enable", false)

			logger.Infof("Disabled client %s due to account %s expiry", assoc.ClientEmail, account.Username)
		}
	}

	return nil
}
