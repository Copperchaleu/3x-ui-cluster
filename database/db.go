// Package database provides database initialization, migration, and management utilities
// for the 3x-ui panel using GORM with SQLite.
package database

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	xuiLogger "github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/util/crypto"
	"github.com/mhsanaei/3x-ui/v2/xray"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

const (
	defaultUsername = "admin"
	defaultPassword = "admin"
)

func initModels() error {
	models := []any{
		&model.User{},
		&model.Account{},        // New: Multi-inbound account
		&model.AccountClient{},  // New: Account-client association
		&model.Slave{},
		&model.Inbound{},
		&model.OutboundTraffics{},
		&model.Setting{},
		&model.InboundClientIps{},
		&xray.ClientTraffic{},
		&model.HistoryOfSeeders{},
		&model.SlaveSetting{},
		&model.SlaveCert{},
	}
	for _, model := range models {
		if err := db.AutoMigrate(model); err != nil {
			xuiLogger.Errorf("Error auto migrating model: %v", err)
			return err
		}
	}
	
	// Add account_id column to client_traffics if it doesn't exist
	if !db.Migrator().HasColumn(&xray.ClientTraffic{}, "account_id") {
		if err := db.Migrator().AddColumn(&xray.ClientTraffic{}, "account_id"); err != nil {
			xuiLogger.Errorf("Error adding account_id column to client_traffics: %v", err)
			return err
		}
		xuiLogger.Info("Added account_id column to client_traffics table")
	}
	
	// Create index on account_id if it doesn't exist
	if !db.Migrator().HasIndex(&xray.ClientTraffic{}, "idx_client_traffics_account_id") {
		if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_client_traffics_account_id ON client_traffics(account_id)").Error; err != nil {
			xuiLogger.Errorf("Error creating index on account_id: %v", err)
		} else {
			xuiLogger.Info("Created index on account_id for client_traffics table")
		}
	}
	
	return nil
}

// initUser creates a default admin user if the users table is empty.
func initUser() error {
	empty, err := isTableEmpty("users")
	if err != nil {
		xuiLogger.Errorf("Error checking if users table is empty: %v", err)
		return err
	}
	if empty {
		xuiLogger.Info("Creating default admin user...")
		hashedPassword, err := crypto.HashPasswordAsBcrypt(defaultPassword)

		if err != nil {
			xuiLogger.Errorf("Error hashing default password: %v", err)
			return err
		}

		user := &model.User{
			Username: defaultUsername,
			Password: hashedPassword,
		}
		err = db.Create(user).Error
		if err != nil {
			xuiLogger.Errorf("Error creating default admin user: %v", err)
			return err
		}
		xuiLogger.Info("Default admin user created successfully")
	}
	return nil
}

// runSeeders migrates user passwords to bcrypt and records seeder execution to prevent re-running.
func runSeeders(isUsersEmpty bool) error {
	empty, err := isTableEmpty("history_of_seeders")
	if err != nil {
		xuiLogger.Errorf("Error checking if seeders history table is empty: %v", err)
		return err
	}

	if empty && isUsersEmpty {
		hashSeeder := &model.HistoryOfSeeders{
			SeederName: "UserPasswordHash",
		}
		return db.Create(hashSeeder).Error
	} else {
		var seedersHistory []string
		db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &seedersHistory)

		if !slices.Contains(seedersHistory, "UserPasswordHash") && !isUsersEmpty {
			xuiLogger.Info("Running password hash migration seeder...")
			var users []model.User
			db.Find(&users)

			for _, user := range users {
				hashedPassword, err := crypto.HashPasswordAsBcrypt(user.Password)
				if err != nil {
					xuiLogger.Errorf("Error hashing password for user '%s': %v", user.Username, err)
					return err
				}
				db.Model(&user).Update("password", hashedPassword)
			}

			hashSeeder := &model.HistoryOfSeeders{
				SeederName: "UserPasswordHash",
			}
			err := db.Create(hashSeeder).Error
			if err == nil {
				xuiLogger.Info("Password hash migration completed successfully")
			}
			return err
		}
	}

	return nil
}

// isTableEmpty returns true if the named table contains zero rows.
func isTableEmpty(tableName string) (bool, error) {
	var count int64
	err := db.Table(tableName).Count(&count).Error
	return count == 0, err
}

// InitDB sets up the database connection, migrates models, and runs seeders.
func InitDB(dbPath string) error {
	xuiLogger.Debugf("Initializing database at path: %s", dbPath)
	dir := path.Dir(dbPath)
	err := os.MkdirAll(dir, fs.ModePerm)
	if err != nil {
		xuiLogger.Errorf("Failed to create database directory: %v", err)
		return err
	}

	var gormLogger logger.Interface

	if config.IsDebug() {
		gormLogger = logger.Default
	} else {
		gormLogger = logger.Discard
	}

	c := &gorm.Config{
		Logger: gormLogger,
	}
	db, err = gorm.Open(sqlite.Open(dbPath), c)
	if err != nil {
		xuiLogger.Errorf("Failed to open database connection: %v", err)
		return err
	}
	xuiLogger.Info("Database connection established")

    // Migration: Rename nodes table to slaves if exists
    if db.Migrator().HasTable("nodes") && !db.Migrator().HasTable("slaves") {
        xuiLogger.Info("Migrating nodes table to slaves...")
        if err := db.Migrator().RenameTable("nodes", "slaves"); err != nil {
            xuiLogger.Errorf("Failed to rename nodes table: %v", err)
        } else {
            xuiLogger.Info("Successfully renamed nodes table to slaves")
        }
    }
    
    // Migration: Rename node_id column in inbounds to slave_id
    if db.Migrator().HasTable("inbounds") && db.Migrator().HasColumn(&model.Inbound{}, "node_id") {
        xuiLogger.Info("Migrating inbounds.node_id to slave_id...")
        if err := db.Migrator().RenameColumn(&model.Inbound{}, "node_id", "slave_id"); err != nil {
             xuiLogger.Errorf("Failed to rename node_id column: %v", err)
        } else {
            xuiLogger.Info("Successfully renamed node_id column to slave_id")
        }
    }

    // Migration: Check for inbounds with SlaveId=0 (Master node) and warn user
    xuiLogger.Debug("Checking for inbounds assigned to Master (SlaveId=0)...")
    var masterInbounds int64
    db.Model(&model.Inbound{}).Where("slave_id = 0").Count(&masterInbounds)
    
    if masterInbounds > 0 {
        xuiLogger.Warningf("Found %d inbounds assigned to Master node (SlaveId=0)", masterInbounds)
        xuiLogger.Warning("Master node no longer runs Xray proxy - Please reassign these inbounds to Slave servers")
    }
    
    // Migration: Initialize slave_settings with xrayTemplateConfig for all slaves
    xuiLogger.Debug("Migrating xrayTemplateConfig to per-slave settings...")
    if err := migrateXrayTemplateConfig(); err != nil {
        xuiLogger.Warningf("Failed to migrate xrayTemplateConfig: %v", err)
    }

	if err := initModels(); err != nil {
		xuiLogger.Errorf("Failed to initialize database models: %v", err)
		return err
	}
	xuiLogger.Info("Database models initialized successfully")

	isUsersEmpty, err := isTableEmpty("users")
	if err != nil {
		return err
	}

	if err := initUser(); err != nil {
		xuiLogger.Errorf("Failed to initialize default user: %v", err)
		return err
	}
	err = runSeeders(isUsersEmpty)
	if err != nil {
		xuiLogger.Errorf("Failed to run database seeders: %v", err)
	}
	xuiLogger.Info("Database initialization completed successfully")
	return err
}

// CloseDB closes the database connection if it exists.
func CloseDB() error {
	if db != nil {
		xuiLogger.Info("Closing database connection...")
		sqlDB, err := db.DB()
		if err != nil {
			xuiLogger.Errorf("Failed to get database instance for closing: %v", err)
			return err
		}
		err = sqlDB.Close()
		if err != nil {
			xuiLogger.Errorf("Failed to close database connection: %v", err)
		} else {
			xuiLogger.Info("Database connection closed successfully")
		}
		return err
	}
	return nil
}

// GetDB returns the global GORM database instance.
func GetDB() *gorm.DB {
	return db
}

// IsNotFound checks if the given error is a GORM record not found error.
func IsNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}

// IsSQLiteDB checks if the given file is a valid SQLite database by reading its signature.
func IsSQLiteDB(file io.ReaderAt) (bool, error) {
	signature := []byte("SQLite format 3\x00")
	buf := make([]byte, len(signature))
	_, err := file.ReadAt(buf, 0)
	if err != nil {
		return false, err
	}
	return bytes.Equal(buf, signature), nil
}

// Checkpoint performs a WAL checkpoint on the SQLite database to ensure data consistency.
func Checkpoint() error {
	// Update WAL
	err := db.Exec("PRAGMA wal_checkpoint;").Error
	if err != nil {
		return err
	}
	return nil
}

// ValidateSQLiteDB opens the provided sqlite DB path with a throw-away connection
// and runs a PRAGMA integrity_check to ensure the file is structurally sound.
// It does not mutate global state or run migrations.
func ValidateSQLiteDB(dbPath string) error {
	if _, err := os.Stat(dbPath); err != nil { // file must exist
		xuiLogger.Errorf("Database file not found: %s", dbPath)
		return err
	}
	xuiLogger.Debugf("Validating database integrity: %s", dbPath)
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		xuiLogger.Errorf("Failed to open database for validation: %v", err)
		return err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		xuiLogger.Errorf("Failed to get database instance: %v", err)
		return err
	}
	defer sqlDB.Close()
	var res string
	if err := gdb.Raw("PRAGMA integrity_check;").Scan(&res).Error; err != nil {
		xuiLogger.Errorf("Database integrity check failed: %v", err)
		return err
	}
	if res != "ok" {
		xuiLogger.Errorf("Database integrity check result: %s", res)
		return errors.New("sqlite integrity check failed: " + res)
	}
	xuiLogger.Info("Database integrity check passed")
	return nil
}

// migrateXrayTemplateConfig migrates the global xrayTemplateConfig to per-slave settings
func migrateXrayTemplateConfig() error {
	// Check if already migrated
	var count int64
	db.Model(&model.SlaveSetting{}).Where("setting_key = ?", "xrayTemplateConfig").Count(&count)
	if count > 0 {
		xuiLogger.Debug("xrayTemplateConfig already migrated to slave_settings")
		return nil
	}

	// Get global xrayTemplateConfig from settings table
	var globalConfig string
	err := db.Model(&model.Setting{}).Where("key = ?", "xrayTemplateConfig").Pluck("value", &globalConfig).Error
	if err != nil {
		return fmt.Errorf("failed to get global xrayTemplateConfig: %v", err)
	}

	if globalConfig == "" {
		xuiLogger.Debug("No global xrayTemplateConfig found, skipping migration")
		return nil
	}

	// Get all slaves
	var slaves []model.Slave
	if err := db.Find(&slaves).Error; err != nil {
		return fmt.Errorf("failed to get slaves: %v", err)
	}

	if len(slaves) == 0 {
		xuiLogger.Debug("No slaves found, skipping xrayTemplateConfig migration")
		return nil
	}

	xuiLogger.Infof("Migrating xrayTemplateConfig to %d slaves...", len(slaves))
	// Create slave_settings record for each slave
	for _, slave := range slaves {
		slaveSetting := model.SlaveSetting{
			SlaveId:      slave.Id,
			SettingKey:   "xrayTemplateConfig",
			SettingValue: globalConfig,
		}
		if err := db.Create(&slaveSetting).Error; err != nil {
			xuiLogger.Warningf("Failed to create slave_setting for slave %d: %v", slave.Id, err)
		} else {
			xuiLogger.Infof("Migrated xrayTemplateConfig to slave %d (%s)", slave.Id, slave.Name)
		}
	}
	xuiLogger.Info("xrayTemplateConfig migration completed")
	return nil
}
