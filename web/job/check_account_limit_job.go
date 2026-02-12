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
func (j *CheckAccountLimitJob) Run() {
	// Check for accounts that have exceeded traffic limits
	err := j.accountService.DisableClientsExceedingAccountLimit()
	if err != nil {
		logger.Warning("CheckAccountLimitJob - Error disabling clients exceeding account limits:", err)
	}

	// Check for expired accounts
	err = j.accountService.DisableExpiredAccountClients()
	if err != nil {
		logger.Warning("CheckAccountLimitJob - Error disabling expired account clients:", err)
	}
}
