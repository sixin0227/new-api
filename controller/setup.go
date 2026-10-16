package controller

import (
	"fmt"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type Setup struct {
	Status       bool   `json:"status"`
	RootInit     bool   `json:"root_init"`
	DatabaseType string `json:"database_type"`
	// CustomDB is true when the database was explicitly configured via the
	// SQL_DSN env var or the persisted config file. When false the system is
	// running on the built-in default SQLite and the setup wizard may offer
	// a database configuration form.
	CustomDB bool `json:"custom_database"`
}

type SetupRequest struct {
	Username           string `json:"username"`
	Password           string `json:"password"`
	ConfirmPassword    string `json:"confirmPassword"`
	SelfUseModeEnabled bool   `json:"SelfUseModeEnabled"`
	DemoSiteEnabled    bool   `json:"DemoSiteEnabled"`
}

type DatabaseConfigRequest struct {
	DatabaseType string `json:"databaseType"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Database     string `json:"database"`
}

// SaveDatabaseConfig persists the database configuration to a config file.
// The new configuration will take effect on next server restart.
func SaveDatabaseConfig(c *gin.Context) {
	if constant.Setup {
		common.ApiErrorT(c, "The system is already initialized")
		return
	}

	var req DatabaseConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}

	if req.DatabaseType == "" || req.Database == "" {
		common.ApiErrorT(c, "Database type and name are required")
		return
	}

	dsn := buildDSN(req)
	if dsn == "" {
		common.ApiErrorT(c, "Failed to build database connection string")
		return
	}

	if err := common.SaveDSNConfig(dsn); err != nil {
		common.ApiErrorT(c, "Failed to save database configuration: {{error}}", map[string]any{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"data":    gin.H{"databaseType": req.DatabaseType},
	})
}

func buildDSN(req DatabaseConfigRequest) string {
	switch req.DatabaseType {
	case "mysql":
		if req.Port == 0 {
			req.Port = 3306
		}
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			req.Username, req.Password, req.Host, req.Port, req.Database)
	case "postgres":
		if req.Port == 0 {
			req.Port = 5432
		}
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable&connect_timeout=10",
			req.Username, req.Password, req.Host, req.Port, req.Database)
	default:
		return ""
	}
}

func GetSetup(c *gin.Context) {
	setup := Setup{
		Status: constant.Setup,
	}
	if constant.Setup {
		c.JSON(200, gin.H{
			"success": true,
			"data":    setup,
		})
		return
	}
	setup.RootInit = model.RootUserExists()
	setup.DatabaseType = string(common.MainDatabaseType())
	setup.CustomDB = os.Getenv("SQL_DSN") != "" || common.ReadDSNConfig() != ""
	c.JSON(200, gin.H{
		"success": true,
		"data":    setup,
	})
}

func PostSetup(c *gin.Context) {
	// Check if setup is already completed
	if constant.Setup {
		common.ApiErrorT(c, "The system is already initialized")
		return
	}

	// Check if root user already exists
	rootExists := model.RootUserExists()

	var req SetupRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}

	// If root doesn't exist, validate and create admin account
	if !rootExists {
		// Validate username length: max 12 characters to align with model.User validation
		if len(req.Username) > 12 {
			common.ApiErrorT(c, "Username must be {{max}} characters or fewer", map[string]any{"max": 12})
			return
		}
		// Validate password
		if req.Password != req.ConfirmPassword {
			common.ApiErrorT(c, "Passwords do not match")
			return
		}

		if err := common.ValidateNewAccountPassword(req.Password); err != nil {
			c.JSON(200, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		// Create root user
		hashedPassword, err := common.HashAccountPassword(req.Password)
		if err != nil {
			common.ApiErrorT(c, "System error: {{error}}", map[string]any{"error": err.Error()})
			return
		}
		rootUser := model.User{
			Username:    req.Username,
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		err = model.DB.Create(&rootUser).Error
		if err != nil {
			common.ApiErrorT(c, "Failed to create the administrator account: {{error}}", map[string]any{"error": err.Error()})
			return
		}
	}

	// Set operation modes
	operation_setting.SelfUseModeEnabled = req.SelfUseModeEnabled
	operation_setting.DemoSiteEnabled = req.DemoSiteEnabled

	// Save operation modes to database for persistence
	err = model.UpdateOption("SelfUseModeEnabled", boolToString(req.SelfUseModeEnabled))
	if err != nil {
		common.ApiErrorT(c, "Failed to save the self-use mode setting: {{error}}", map[string]any{"error": err.Error()})
		return
	}

	err = model.UpdateOption("DemoSiteEnabled", boolToString(req.DemoSiteEnabled))
	if err != nil {
		common.ApiErrorT(c, "Failed to save the demo site mode setting: {{error}}", map[string]any{"error": err.Error()})
		return
	}

	// Update setup status
	constant.Setup = true

	setup := model.Setup{
		Version:       common.Version,
		InitializedAt: time.Now().Unix(),
	}
	err = model.DB.Create(&setup).Error
	if err != nil {
		common.ApiErrorT(c, "Failed to initialize the system: {{error}}", map[string]any{"error": err.Error()})
		return
	}

	common.ApiSuccessT(c, "System initialized successfully", nil)
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
