package job

import (
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

// CheckAccountLimitJob periodically checks account traffic limits and expiry,
// disabling clients that exceed limits or have expired accounts.
type CheckAccountLimitJob struct {
	accountService service.AccountService
}

// NewCheckAccountLimitJob creates a new account limit checking job instance.
func NewCheckAccountLimitJob() *CheckAccountLimitJob {
	return &CheckAccountLimitJob{}
}

// Run checks all accounts for traffic limits and expiry, disabling clients as needed.
// After disabling clients, it pushes updated configs to affected slaves.
func (j *CheckAccountLimitJob) Run() {
	affectedSlaveIds := make(map[int]bool)
	
	// Check for accounts that have exceeded traffic limits
	trafficLimitSlaves, err := j.accountService.DisableClientsExceedingAccountLimit()
	if err != nil {
		logger.Warning("CheckAccountLimitJob - Error disabling clients exceeding account limits:", err)
	} else {
		for _, slaveId := range trafficLimitSlaves {
			affectedSlaveIds[slaveId] = true
		}
		if len(trafficLimitSlaves) > 0 {
			logger.Infof("CheckAccountLimitJob - Accounts disabled due to traffic limits, affected slaves: %v", trafficLimitSlaves)
		}
	}

	// Check for expired accounts
	expirySlaves, err := j.accountService.DisableExpiredAccountClients()
	if err != nil {
		logger.Warning("CheckAccountLimitJob - Error disabling expired account clients:", err)
	} else {
		for _, slaveId := range expirySlaves {
			affectedSlaveIds[slaveId] = true
		}
		if len(expirySlaves) > 0 {
			logger.Infof("CheckAccountLimitJob - Accounts disabled due to expiry, affected slaves: %v", expirySlaves)
		}
	}
	
	// Push updated config to all affected slaves
	if len(affectedSlaveIds) > 0 {
		slaveService := service.SlaveService{}
		for slaveId := range affectedSlaveIds {
			if err := slaveService.PushConfig(slaveId); err != nil {
				logger.Errorf("CheckAccountLimitJob - Failed to push config to slave %d: %v", slaveId, err)
			} else {
				logger.Infof("CheckAccountLimitJob - Successfully pushed config to slave %d after disabling clients", slaveId)
			}
		}
	}
}
