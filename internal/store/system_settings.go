package store

import (
	"gorm.io/gorm"
)

// Settings keys
const (
	SettingOrderExpireHours   = "order_expire_hours"
	SettingOrderCleanupDays   = "order_cleanup_days"
	SettingEnableAutoExpire   = "enable_auto_expire"
	SettingEnableAutoCleanup  = "enable_auto_cleanup"
)

// GetSetting retrieves a setting by key
func GetSetting(db *gorm.DB, key string) (string, error) {
	var setting SystemSetting
	err := db.Where("key = ?", key).First(&setting).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Return default values
			switch key {
			case SettingOrderExpireHours:
				return "24", nil
			case SettingOrderCleanupDays:
				return "7", nil
			case SettingEnableAutoExpire:
				return "true", nil
			case SettingEnableAutoCleanup:
				return "true", nil
			default:
				return "", nil
			}
		}
		return "", err
	}
	return setting.Value, nil
}

// SetSetting updates or creates a setting
func SetSetting(db *gorm.DB, key, value, description, settingType string) error {
	var setting SystemSetting
	err := db.Where("key = ?", key).First(&setting).Error
	
	if err == gorm.ErrRecordNotFound {
		// Create new setting
		setting = SystemSetting{
			Key:         key,
			Value:       value,
			Description: description,
			Type:        settingType,
		}
		return db.Create(&setting).Error
	}
	
	// Update existing setting
	return db.Model(&setting).Updates(map[string]interface{}{
		"value":       value,
		"description": description,
		"type":        settingType,
	}).Error
}

// GetAllSettings retrieves all settings
func GetAllSettings(db *gorm.DB) ([]SystemSetting, error) {
	var settings []SystemSetting
	err := db.Order("key").Find(&settings).Error
	return settings, err
}

// InitializeSettings creates default settings if they don't exist
func InitializeSettings(db *gorm.DB) error {
	defaultSettings := []struct {
		Key         string
		Value       string
		Description string
		Type        string
	}{
		{
			Key:         SettingOrderExpireHours,
			Value:       "24",
			Description: "订单过期时间（小时）",
			Type:        "int",
		},
		{
			Key:         SettingOrderCleanupDays,
			Value:       "7",
			Description: "清理过期订单的天数",
			Type:        "int",
		},
		{
			Key:         SettingEnableAutoExpire,
			Value:       "true",
			Description: "启用订单自动过期",
			Type:        "bool",
		},
		{
			Key:         SettingEnableAutoCleanup,
			Value:       "true",
			Description: "启用过期订单自动清理",
			Type:        "bool",
		},
	}
	
	for _, s := range defaultSettings {
		var existing SystemSetting
		err := db.Where("key = ?", s.Key).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := db.Create(&SystemSetting{
				Key:         s.Key,
				Value:       s.Value,
				Description: s.Description,
				Type:        s.Type,
			}).Error; err != nil {
				return err
			}
		}
	}
	
	return nil
}

// SettingsMap is a helper to get all settings as a map
func GetSettingsMap(db *gorm.DB) (map[string]string, error) {
	settings, err := GetAllSettings(db)
	if err != nil {
		return nil, err
	}
	
	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	
	// Add defaults for missing settings
	if _, ok := result[SettingOrderExpireHours]; !ok {
		result[SettingOrderExpireHours] = "24"
	}
	if _, ok := result[SettingOrderCleanupDays]; !ok {
		result[SettingOrderCleanupDays] = "7"
	}
	if _, ok := result[SettingEnableAutoExpire]; !ok {
		result[SettingEnableAutoExpire] = "true"
	}
	if _, ok := result[SettingEnableAutoCleanup]; !ok {
		result[SettingEnableAutoCleanup] = "true"
	}
	
	return result, nil
}