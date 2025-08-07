package store

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

var (
	ErrGroupNotFound    = errors.New("group not found")
	ErrGroupExists      = errors.New("group already registered")
	ErrNotGroupAdmin    = errors.New("user is not group admin")
)

// RegisterGroup registers a new group
func RegisterGroup(db *gorm.DB, tgGroupID int64, groupName, groupType string, addedByUserID uint) (*Group, error) {
	// Check if group already exists
	var existing Group
	if err := db.Where("tg_group_id = ?", tgGroupID).First(&existing).Error; err == nil {
		return nil, ErrGroupExists
	}

	group := &Group{
		TgGroupID:     tgGroupID,
		GroupName:     groupName,
		GroupType:     groupType,
		AddedByUserID: addedByUserID,
		IsActive:      true,
		NotifyStock:   true,
		NotifyPromo:   true,
	}

	if err := db.Create(group).Error; err != nil {
		return nil, err
	}

	// Add the user as admin
	admin := &GroupAdmin{
		GroupID: group.ID,
		UserID:  addedByUserID,
		Role:    "admin",
	}
	db.Create(admin)

	return group, nil
}

// GetGroup retrieves a group by Telegram ID
func GetGroup(db *gorm.DB, tgGroupID int64) (*Group, error) {
	var group Group
	if err := db.Where("tg_group_id = ?", tgGroupID).First(&group).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

// GetActiveGroups retrieves all active groups
func GetActiveGroups(db *gorm.DB) ([]Group, error) {
	var groups []Group
	err := db.Where("is_active = ?", true).Find(&groups).Error
	return groups, err
}

// GetGroupsForBroadcast retrieves groups based on notification preferences
func GetGroupsForBroadcast(db *gorm.DB, notificationType string) ([]Group, error) {
	var groups []Group
	query := db.Where("is_active = ?", true)
	
	switch notificationType {
	case "stock_update":
		query = query.Where("notify_stock = ?", true)
	case "promotion":
		query = query.Where("notify_promo = ?", true)
	}
	
	err := query.Find(&groups).Error
	return groups, err
}

// IsUserGroupAdmin checks if a user is admin of a group
func IsUserGroupAdmin(db *gorm.DB, userID uint, groupID uint) bool {
	var count int64
	db.Model(&GroupAdmin{}).
		Where("user_id = ? AND group_id = ?", userID, groupID).
		Count(&count)
	return count > 0
}

// UpdateGroupSettings updates group notification preferences
func UpdateGroupSettings(db *gorm.DB, groupID uint, notifyStock, notifyPromo bool) error {
	return db.Model(&Group{}).
		Where("id = ?", groupID).
		Updates(map[string]interface{}{
			"notify_stock": notifyStock,
			"notify_promo": notifyPromo,
			"updated_at":   time.Now(),
		}).Error
}

// DeactivateGroup deactivates a group
func DeactivateGroup(db *gorm.DB, groupID uint) error {
	return db.Model(&Group{}).
		Where("id = ?", groupID).
		Update("is_active", false).Error
}

// GetAllUsers retrieves all active users
func GetAllUsers(db *gorm.DB) ([]User, error) {
	var users []User
	err := db.Find(&users).Error
	return users, err
}

// CreateBroadcastMessage creates a new broadcast message
func CreateBroadcastMessage(db *gorm.DB, msgType, content, targetType string, createdByID uint) (*BroadcastMessage, error) {
	msg := &BroadcastMessage{
		Type:        msgType,
		Content:     content,
		TargetType:  targetType,
		Status:      "pending",
		CreatedByID: createdByID,
	}
	
	// Calculate total recipients
	switch targetType {
	case "all":
		var userCount, groupCount int64
		db.Model(&User{}).Count(&userCount)
		db.Model(&Group{}).Where("is_active = ?", true).Count(&groupCount)
		msg.TotalRecipients = int(userCount + groupCount)
	case "users":
		var count int64
		db.Model(&User{}).Count(&count)
		msg.TotalRecipients = int(count)
	case "groups":
		var count int64
		db.Model(&Group{}).Where("is_active = ?", true).Count(&count)
		msg.TotalRecipients = int(count)
	}
	
	if err := db.Create(msg).Error; err != nil {
		return nil, err
	}
	
	return msg, nil
}

// UpdateBroadcastStatus updates broadcast message status
func UpdateBroadcastStatus(db *gorm.DB, broadcastID uint, status string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	
	if status == "sending" {
		now := time.Now()
		updates["started_at"] = &now
	} else if status == "completed" || status == "failed" {
		now := time.Now()
		updates["completed_at"] = &now
	}
	
	return db.Model(&BroadcastMessage{}).
		Where("id = ?", broadcastID).
		Updates(updates).Error
}

// IncrementBroadcastCount increments sent or failed count
func IncrementBroadcastCount(db *gorm.DB, broadcastID uint, sent bool) error {
	field := "sent_count"
	if !sent {
		field = "failed_count"
	}
	
	return db.Model(&BroadcastMessage{}).
		Where("id = ?", broadcastID).
		UpdateColumn(field, gorm.Expr(field + " + ?", 1)).Error
}

// LogBroadcastAttempt logs a broadcast send attempt
func LogBroadcastAttempt(db *gorm.DB, broadcastID uint, recipientType string, recipientID int64, status string, errorMsg string) error {
	log := &BroadcastLog{
		BroadcastID:   broadcastID,
		RecipientType: recipientType,
		RecipientID:   recipientID,
		Status:        status,
		Error:         errorMsg,
	}
	return db.Create(log).Error
}

// GetGroupStats returns group statistics
func GetGroupStats(db *gorm.DB) (total, active int64, err error) {
	err = db.Model(&Group{}).Count(&total).Error
	if err != nil {
		return
	}
	
	err = db.Model(&Group{}).Where("is_active = ?", true).Count(&active).Error
	return
}