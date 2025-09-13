package store

import (
	"time"
)

// User represents a Telegram user
type User struct {
	ID           uint      `gorm:"primaryKey"`
	TgUserID     int64     `gorm:"uniqueIndex;not null"`
	Username     string    `gorm:"size:100"`
	TgUsername   string    `gorm:"size:100"`
	TgFirstName  string    `gorm:"size:100"`
	TgLastName   string    `gorm:"size:100"`
	Language     string    `gorm:"size:10;default:'en'"`
	BalanceCents int       `gorm:"default:0;not null"` // User balance in cents
	CreatedAt    time.Time
}

// Product represents a sellable item
type Product struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:200;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	PriceCents  int       `gorm:"not null" json:"price_cents"` // Price in cents to avoid float precision issues
	IsActive    bool      `gorm:"default:true;index" json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	ProductID       *uint     `gorm:"index"` // Nullable for deposit orders
	Product         *Product  `gorm:"foreignKey:ProductID"`
	AmountCents     int       `gorm:"not null"`
	BalanceUsed     int       `gorm:"default:0;not null"` // Balance used for this order
	PaymentAmount   int       `gorm:"not null"` // Actual payment amount (after balance deduction)
	Status          string    `gorm:"size:20;not null;default:'pending';index"` // pending, paid, delivered, paid_no_stock, failed_delivery, expired
	EpayTradeNo     string    `gorm:"size:100;index"`
	EpayOutTradeNo  string    `gorm:"size:100;uniqueIndex"`
	DeliveryRetries int       `gorm:"default:0;not null"` // Number of delivery retry attempts
	LastRetryAt     *time.Time
	CreatedAt       time.Time
	PaidAt          *time.Time
	DeliveredAt     *time.Time
	Code            *Code     `gorm:"-"` // Virtual field for displaying code in admin
}

// RechargeCard represents a recharge card for balance top-up
type RechargeCard struct {
	ID           uint      `gorm:"primaryKey"`
	Code         string    `gorm:"uniqueIndex;not null"`
	AmountCents  int       `gorm:"not null"` // Amount in cents
	MaxUses      int       `gorm:"default:1;not null"` // Maximum total uses
	UsedCount    int       `gorm:"default:0;not null"` // Current used count
	MaxUsesPerUser int     `gorm:"default:1;not null"` // Maximum uses per user
	IsUsed       bool      `gorm:"default:false;index"` // Deprecated, kept for compatibility
	UsedByUserID *uint     // Deprecated, kept for compatibility
	UsedBy       *User     `gorm:"foreignKey:UsedByUserID"` // Deprecated
	UsedAt       *time.Time // Deprecated
	CreatedAt    time.Time
	ExpiresAt    *time.Time `gorm:"index"`
}

// RechargeCardUsage represents a recharge card usage record
type RechargeCardUsage struct {
	ID             uint         `gorm:"primaryKey"`
	RechargeCardID uint         `gorm:"not null;index:idx_card_user,unique"`
	RechargeCard   RechargeCard `gorm:"foreignKey:RechargeCardID"`
	UserID         uint         `gorm:"not null;index:idx_card_user,unique"`
	User           User         `gorm:"foreignKey:UserID"`
	UseCount       int          `gorm:"default:1;not null"` // How many times this user has used this card
	LastUsedAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
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
	Code      string    `gorm:"size:50;not null;uniqueIndex:idx_code_lang"` // Template code with composite index
	Language  string    `gorm:"size:10;not null;default:'en';uniqueIndex:idx_code_lang"`
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
func (RechargeCardUsage) TableName() string { return "recharge_card_usages" }
func (BalanceTransaction) TableName() string { return "balance_transactions" }
func (MessageTemplate) TableName() string { return "message_templates" }
func (SystemSetting) TableName() string { return "system_settings" }

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

// SystemSetting represents system-wide settings
type SystemSetting struct {
	ID          uint      `gorm:"primaryKey"`
	Key         string    `gorm:"uniqueIndex;not null;size:100"`
	Value       string    `gorm:"type:text"`
	Description string    `gorm:"size:500"`
	Type        string    `gorm:"size:50;default:'string'"` // string, int, bool, json
	UpdatedAt   time.Time
	CreatedAt   time.Time
}

func (Group) TableName() string { return "groups" }
func (GroupAdmin) TableName() string { return "group_admins" }
func (BroadcastMessage) TableName() string { return "broadcast_messages" }
func (BroadcastLog) TableName() string { return "broadcast_logs" }

// FAQ represents a frequently asked question
type FAQ struct {
	ID        uint      `gorm:"primaryKey"`
	Question  string    `gorm:"size:500;not null"`
	Answer    string    `gorm:"type:text;not null"`
	Language  string    `gorm:"size:10;not null;default:'zh'"`
	SortOrder int       `gorm:"default:0"`
	IsActive  bool      `gorm:"default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AdminUser represents an admin user
type AdminUser struct {
	ID                   uint       `gorm:"primaryKey"`
	Username             string     `gorm:"uniqueIndex;size:50;not null"`
	Password             string     `gorm:"size:255;not null"`
	Email                string     `gorm:"size:100"`
	IsActive             bool       `gorm:"default:true"`
	IsSuperAdmin         bool       `gorm:"default:false"`
	TelegramID           *int64     `gorm:"index"`
	ReceiveNotifications bool       `gorm:"default:true"`
	LastLoginAt          *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Ticket represents a support ticket
type Ticket struct {
	ID          uint      `gorm:"primaryKey"`
	TicketID    string    `gorm:"uniqueIndex;size:50"` // e.g., "TK-20240312-001"
	UserID      int64     `gorm:"index;not null"`      // Telegram user ID
	Username    string    `gorm:"size:100"`
	Status      string    `gorm:"size:20;default:'open'"` // open, in_progress, resolved, closed
	Priority    string    `gorm:"size:20;default:'normal'"` // low, normal, high, urgent
	Subject     string    `gorm:"size:200;not null"`
	Category    string    `gorm:"size:50"` // order_issue, payment_issue, product_issue, other
	AssignedTo  *uint     `gorm:"index"`   // Admin user ID (nullable)
	LastReplyAt *time.Time
	ResolvedAt  *time.Time
	ClosedAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Relations
	Messages []TicketMessage `gorm:"foreignKey:TicketID;references:ID"`
	AssignedBy *AdminUser `gorm:"foreignKey:AssignedTo;references:ID"`
}

// TicketMessage represents a message in a ticket conversation
type TicketMessage struct {
	ID         uint      `gorm:"primaryKey"`
	TicketID   uint      `gorm:"index;not null"`
	SenderType string    `gorm:"size:20;not null"` // user, admin, system
	SenderID   int64     `gorm:"not null"`         // Telegram user ID or Admin ID
	SenderName string    `gorm:"size:100"`
	Content    string    `gorm:"type:text;not null"`
	MessageID  int       // Telegram message ID for reference
	IsRead     bool      `gorm:"default:false"`
	ReadAt     *time.Time
	CreatedAt  time.Time
}

// TicketTemplate represents a template for quick replies
type TicketTemplate struct {
	ID        uint      `gorm:"primaryKey"`
	Name      string    `gorm:"size:100;not null"`
	Category  string    `gorm:"size:50"`
	Content   string    `gorm:"type:text;not null"`
	IsActive  bool      `gorm:"default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (FAQ) TableName() string { return "faqs" }