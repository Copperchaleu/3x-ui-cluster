package service

import (
	"errors"
	"runtime"
	"sync"

	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/xray"

	"go.uber.org/atomic"
)

var (
	p                 *xray.Process
	lock              sync.Mutex
	isNeedXrayRestart atomic.Bool // Indicates that restart was requested for Xray
	isManuallyStopped atomic.Bool // Indicates that Xray was stopped manually from the panel
	result            string
)

// XrayService provides business logic for Xray process management.
// It handles starting, stopping, restarting Xray, and managing its configuration.
type XrayService struct {
	inboundService InboundService
	settingService SettingService
	xrayAPI        xray.XrayAPI
}

// IsXrayRunning checks if the Xray process is currently running.
func (s *XrayService) IsXrayRunning() bool {
	return p != nil && p.IsRunning()
}

// GetXrayErr returns the error from the Xray process, if any.
func (s *XrayService) GetXrayErr() error {
	if p == nil {
		return nil
	}

	err := p.GetErr()
	if err == nil {
		return nil
	}

	if runtime.GOOS == "windows" && err.Error() == "exit status 1" {
		// exit status 1 on Windows means that Xray process was killed
		// as we kill process to stop in on Windows, this is not an error
		return nil
	}

	return err
}

// GetXrayResult returns the result string from the Xray process.
func (s *XrayService) GetXrayResult() string {
	if result != "" {
		return result
	}
	if s.IsXrayRunning() {
		return ""
	}
	if p == nil {
		return ""
	}

	result = p.GetResult()

	if runtime.GOOS == "windows" && result == "exit status 1" {
		// exit status 1 on Windows means that Xray process was killed
		// as we kill process to stop in on Windows, this is not an error
		return ""
	}

	return result
}

// GetXrayVersion returns the version of the running Xray process.
func (s *XrayService) GetXrayVersion() string {
	if p == nil {
		return "Unknown"
	}
	return p.GetVersion()
}

// RemoveIndex removes an element at the specified index from a slice.
// Returns a new slice with the element removed.
func RemoveIndex(s []any, index int) []any {
	return append(s[:index], s[index+1:]...)
}

// GetXrayConfig is deprecated - Master node no longer runs Xray.
// All proxy functionality should be handled by Slave nodes.
func (s *XrayService) GetXrayConfig() (*xray.Config, error) {
	// Master node doesn't run Xray anymore
	return nil, errors.New("Master node does not run Xray. Please use Slave nodes for proxy functionality")
}

// GetXrayTraffic fetches the current traffic statistics from the running Xray process.
func (s *XrayService) GetXrayTraffic() ([]*xray.Traffic, []*xray.ClientTraffic, error) {
	if !s.IsXrayRunning() {
		err := errors.New("xray is not running")
		logger.Debug("Attempted to fetch Xray traffic, but Xray is not running:", err)
		return nil, nil, err
	}
	apiPort := p.GetAPIPort()
	s.xrayAPI.Init(apiPort)
	defer s.xrayAPI.Close()

	traffic, clientTraffic, err := s.xrayAPI.GetTraffic(true)
	if err != nil {
		logger.Debug("Failed to fetch Xray traffic:", err)
		return nil, nil, err
	}
	return traffic, clientTraffic, nil
}

// RestartXray is deprecated - Master node no longer runs Xray.
// Configuration changes are automatically pushed to Slave nodes.
func (s *XrayService) RestartXray(isForce bool) error {
	logger.Info("RestartXray called on Master - this is a no-op. Xray runs only on Slave nodes.")
	// Master doesn't run Xray, so this is effectively a no-op
	// Configuration updates are pushed to slaves automatically
	return nil
}

// StopXray is deprecated - Master node no longer runs Xray.
func (s *XrayService) StopXray() error {
	logger.Info("StopXray called on Master - this is a no-op. Xray runs only on Slave nodes.")
	return nil
}

// SetToNeedRestart is deprecated - Master node no longer runs Xray.
func (s *XrayService) SetToNeedRestart() {
	logger.Debug("SetToNeedRestart called on Master - this is a no-op")
	// Master doesn't run Xray, no restart flag needed
}

// IsNeedRestartAndSetFalse is deprecated - Master node no longer runs Xray.
func (s *XrayService) IsNeedRestartAndSetFalse() bool {
	logger.Debug("IsNeedRestartAndSetFalse called on Master - always returns false")
	return false // Master doesn't run Xray
}

// DidXrayCrash is deprecated - Master node no longer runs Xray.
func (s *XrayService) DidXrayCrash() bool {
	logger.Debug("DidXrayCrash called on Master - always returns false")
	return false // Master doesn't run Xray, can't crash
}
