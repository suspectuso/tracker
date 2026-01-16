package storage

import "time"

// Wallet represents a tracked TON wallet
type Wallet struct {
	ID             int64
	UserID         int64
	Name           string
	AddressRaw     string // 0:... format
	AddressDisplay string // UQ.../EQ... format
	MinAmountTON   *float64
	CreatedAt      time.Time
}

// PremiumUser represents a user with premium subscription
type PremiumUser struct {
	UserID       int64
	ActivatedAt  time.Time
	PayerAddress string
	EventID      string
}

// ProcessedEvent tracks which events have been processed to avoid duplicates
type ProcessedEvent struct {
	WalletID int64
	EventID  string
}

// PremiumPayment records premium payments
type PremiumPayment struct {
	EventID       string
	UserID        int64
	Amount        float64
	SenderAddress string
}

// PendingPremiumPayment for tracking unique payment amounts
type PendingPremiumPayment struct {
	UserID       int64
	UniqueAmount float64
	CreatedAt    time.Time
}
