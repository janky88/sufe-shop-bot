package store

import (
	"time"
	"gorm.io/gorm"
)

// User represents a Telegram user
type User struct {
	ID           uint      `gorm:"primaryKey"`
	TgUserID     int64     `gorm:"uniqueIndex;not null"`
	Username     string    `gorm:"size:100"`
	Language     string    `gorm:"size:10;default:'en'"`
	BalanceCents int       `gorm:"default:0;not null"` // User balance in cents
	CreatedAt    time.Time
}

// Product represents a sellable item
type Product struct {
	ID         uint      `gorm:"primaryKey"`
	Name       string    `gorm:"size:200;not null"`
	PriceCents int       `gorm:"not null"` // Price in cents to avoid float precision issues
	IsActive   bool      `gorm:"default:true;index"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Code represents a card/account code
type Code struct {
	ID         uint      `gorm:"primaryKey"`
	ProductID  uint      `gorm:"not null;index"`
	Product    Product   `gorm:"foreignKey:ProductID"`
	Code       string    `gorm:"type:text;not null"`
	IsSold     bool      `gorm:"default:false;index"`
	SoldAt     *time.Time
	OrderID    *uint
	Order      *Order    `gorm:"foreignKey:OrderID"`
	CreatedAt  time.Time
}

// Order represents a purchase order
type Order struct {
	ID              uint      `gorm:"primaryKey"`
	UserID          uint      `gorm:"not null;index"`
	User            User      `gorm:"foreignKey:UserID"`
	ProductID       uint      `gorm:"not null;index"`
	Product         Product   `gorm:"foreignKey:ProductID"`
	AmountCents     int       `gorm:"not null"`
	BalanceUsed     int       `gorm:"default:0;not null"` // Balance used for this order
	PaymentAmount   int       `gorm:"not null"` // Actual payment amount (after balance deduction)
	Status          string    `gorm:"size:20;not null;default:'pending';index"` // pending, paid, delivered, paid_no_stock, failed_delivery
	EpayTradeNo     string    `gorm:"size:100;index"`
	EpayOutTradeNo  string    `gorm:"size:100;uniqueIndex"`
	DeliveryRetries int       `gorm:"default:0;not null"` // Number of delivery retry attempts
	LastRetryAt     *time.Time
	CreatedAt       time.Time
	PaidAt          *time.Time
}

// RechargeCard represents a recharge card for balance top-up
type RechargeCard struct {
	ID           uint      `gorm:"primaryKey"`
	Code         string    `gorm:"uniqueIndex;not null"`
	AmountCents  int       `gorm:"not null"` // Amount in cents
	IsUsed       bool      `gorm:"default:false;index"`
	UsedByUserID *uint
	UsedBy       *User     `gorm:"foreignKey:UsedByUserID"`
	UsedAt       *time.Time
	CreatedAt    time.Time
	ExpiresAt    *time.Time `gorm:"index"`
}

// BalanceTransaction represents a balance transaction
type BalanceTransaction struct {
	ID             uint      `gorm:"primaryKey"`
	UserID         uint      `gorm:"not null;index"`
	User           User      `gorm:"foreignKey:UserID"`
	Type           string    `gorm:"size:20;not null"` // recharge, purchase, refund
	AmountCents    int       `gorm:"not null"` // Positive for income, negative for expense
	BalanceAfter   int       `gorm:"not null"` // Balance after transaction
	RechargeCardID *uint
	RechargeCard   *RechargeCard `gorm:"foreignKey:RechargeCardID"`
	OrderID        *uint
	Order          *Order    `gorm:"foreignKey:OrderID"`
	Description    string    `gorm:"size:200"`
	CreatedAt      time.Time
}

// MessageTemplate represents customizable message templates
type MessageTemplate struct {
	ID        uint      `gorm:"primaryKey"`
	Code      string    `gorm:"uniqueIndex;not null;size:50"` // Template code (e.g., "order_paid", "no_stock")
	Language  string    `gorm:"size:10;not null;default:'en'"`
	Name      string    `gorm:"size:100;not null"` // Human-readable name
	Content   string    `gorm:"type:text;not null"` // Template content with {{variables}}
	Variables string    `gorm:"size:500"` // JSON array of available variables
	IsActive  bool      `gorm:"default:true"`
	UpdatedAt time.Time
	CreatedAt time.Time
}

// TableName customizations
func (User) TableName() string { return "users" }
func (Product) TableName() string { return "products" }
func (Code) TableName() string { return "codes" }
func (Order) TableName() string { return "orders" }
func (RechargeCard) TableName() string { return "recharge_cards" }
func (BalanceTransaction) TableName() string { return "balance_transactions" }
func (MessageTemplate) TableName() string { return "message_templates" }

// Group represents a Telegram group or channel
type Group struct {
	ID           uint      `gorm:"primaryKey"`
	TgGroupID    int64     `gorm:"uniqueIndex;not null"` // Telegram Chat ID
	GroupName    string    `gorm:"size:200"`
	GroupType    string    `gorm:"size:50"` // group, supergroup, channel
	IsActive     bool      `gorm:"default:true;not null"`
	Language     string    `gorm:"size:10;default:'zh'"`
	NotifyStock  bool      `gorm:"default:true;not null"`  // Notify on stock updates
	NotifyPromo  bool      `gorm:"default:true;not null"`  // Notify on promotions
	AddedByUserID uint     `gorm:"index"`
	AddedBy      *User     `gorm:"foreignKey:AddedByUserID"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// GroupAdmin represents administrators for groups
type GroupAdmin struct {
	ID        uint      `gorm:"primaryKey"`
	GroupID   uint      `gorm:"index:idx_group_user,unique"`
	UserID    uint      `gorm:"index:idx_group_user,unique"`
	Group     Group     `gorm:"foreignKey:GroupID"`
	User      User      `gorm:"foreignKey:UserID"`
	Role      string    `gorm:"size:50;default:'admin'"` // admin, moderator
	CreatedAt time.Time
}

// BroadcastMessage represents a broadcast message to be sent
type BroadcastMessage struct {
	ID              uint      `gorm:"primaryKey"`
	Type            string    `gorm:"size:50;not null"` // stock_update, promotion, announcement
	Content         string    `gorm:"type:text;not null"`
	TargetType      string    `gorm:"size:20;not null"` // all, users, groups
	Status          string    `gorm:"size:20;default:'pending'"` // pending, sending, completed, failed
	TotalRecipients int       `gorm:"default:0"`
	SentCount       int       `gorm:"default:0"`
	FailedCount     int       `gorm:"default:0"`
	CreatedByID     uint      `gorm:"index"`
	CreatedBy       *User     `gorm:"foreignKey:CreatedByID"`
	StartedAt       *time.Time
	CompletedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// BroadcastLog represents individual message send attempts
type BroadcastLog struct {
	ID               uint      `gorm:"primaryKey"`
	BroadcastID      uint      `gorm:"index"`
	Broadcast        BroadcastMessage `gorm:"foreignKey:BroadcastID"`
	RecipientType    string    `gorm:"size:20"` // user, group
	RecipientID      int64     `gorm:"index"`   // Telegram ID
	Status           string    `gorm:"size:20"` // sent, failed
	Error            string    `gorm:"type:text"`
	CreatedAt        time.Time
}

func (Group) TableName() string { return "groups" }
func (GroupAdmin) TableName() string { return "group_admins" }
func (BroadcastMessage) TableName() string { return "broadcast_messages" }
func (BroadcastLog) TableName() string { return "broadcast_logs" }