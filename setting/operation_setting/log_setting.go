package operation_setting

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const LogMaxRangeDaysOptionKey = "log_setting.max_range_days"

// LogSetting limits how far regular users can reach into usage logs.
type LogSetting struct {
	// UserExportEnabled lets regular users export their own usage logs.
	// Administrators can always export.
	UserExportEnabled bool `json:"user_export_enabled"`
	// MaxRangeDays is the longest time range a user can search in their logs
	// and dashboard, and the longest range any export can cover.
	MaxRangeDays int `json:"max_range_days"`
	// RecordBodyEnabled is the master switch for recording relay request and
	// response bodies into request_bodies. Recording additionally requires the
	// per-token RecordBodies flag, so a fresh deployment records nothing.
	RecordBodyEnabled bool `json:"record_body_enabled"`
	// RecordBodyMaxBytes bounds one recorded side; a larger body is skipped
	// whole and flagged instead of truncated.
	RecordBodyMaxBytes int `json:"record_body_max_bytes"`
}

var logSetting = LogSetting{
	UserExportEnabled:  true,
	MaxRangeDays:       365,
	RecordBodyEnabled:  false,
	RecordBodyMaxBytes: 1 << 20,
}

func init() {
	config.GlobalConfig.Register("log_setting", &logSetting)
}

func GetLogSetting() *LogSetting {
	return &logSetting
}

func (s *LogSetting) MaxRangeSeconds() int64 {
	return int64(max(s.MaxRangeDays, 1)) * 24 * 60 * 60
}

func ValidateLogMaxRangeDays(value string) error {
	days, err := strconv.Atoi(value)
	if err != nil || days < 1 {
		return common.NewMessage("The longest log time range must be at least 1 day")
	}
	return nil
}
