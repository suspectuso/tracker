package storage

import (
	"database/sql"
	"errors"
	"math"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrLimitReached  = errors.New("wallet limit reached")
	ErrAlreadyExists = errors.New("already exists")
)

// Storage handles all database operations
type Storage struct {
	db *sql.DB
}

// New creates a new Storage instance and initializes the database
func New(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	s := &Storage{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

// Close closes the database connection
func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) init() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS wallets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			address_raw TEXT NOT NULL,
			address_display TEXT NOT NULL,
			min_amount_ton REAL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wallets_user_id ON wallets(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_wallets_address_raw ON wallets(address_raw)`,

		`CREATE TABLE IF NOT EXISTS processed_events (
			wallet_id INTEGER NOT NULL,
			event_id TEXT NOT NULL,
			PRIMARY KEY (wallet_id, event_id)
		)`,

		`CREATE TABLE IF NOT EXISTS premium_users (
			user_id INTEGER PRIMARY KEY,
			activated_at INTEGER NOT NULL,
			payer_address TEXT,
			event_id TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS premium_payments (
			event_id TEXT PRIMARY KEY,
			user_id INTEGER,
			amount REAL,
			sender_address TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS pending_premium_payments (
			user_id INTEGER PRIMARY KEY,
			unique_amount REAL NOT NULL,
			created_at INTEGER NOT NULL
		)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}

	return nil
}

// --- Wallets ---

// AddWallet adds a new wallet for a user
func (s *Storage) AddWallet(userID int64, name, addressRaw, addressDisplay string, maxWallets int) (*Wallet, error) {
	// Check current wallet count
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM wallets WHERE user_id = ?", userID).Scan(&count)
	if err != nil {
		return nil, err
	}

	if count >= maxWallets {
		return nil, ErrLimitReached
	}

	now := time.Now().Unix()
	result, err := s.db.Exec(
		`INSERT INTO wallets (user_id, name, address_raw, address_display, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		userID, name, addressRaw, addressDisplay, now,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Wallet{
		ID:             id,
		UserID:         userID,
		Name:           name,
		AddressRaw:     addressRaw,
		AddressDisplay: addressDisplay,
		CreatedAt:      time.Unix(now, 0),
	}, nil
}

// ListWallets returns all wallets for a user
func (s *Storage) ListWallets(userID int64) ([]Wallet, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, name, address_raw, address_display, min_amount_ton, created_at
		 FROM wallets WHERE user_id = ? ORDER BY id DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wallets []Wallet
	for rows.Next() {
		var w Wallet
		var createdAt int64
		var minAmount sql.NullFloat64

		err := rows.Scan(&w.ID, &w.UserID, &w.Name, &w.AddressRaw, &w.AddressDisplay, &minAmount, &createdAt)
		if err != nil {
			return nil, err
		}

		w.CreatedAt = time.Unix(createdAt, 0)
		if minAmount.Valid {
			w.MinAmountTON = &minAmount.Float64
		}
		wallets = append(wallets, w)
	}

	return wallets, nil
}

// GetWallet returns a wallet by ID
func (s *Storage) GetWallet(walletID int64) (*Wallet, error) {
	var w Wallet
	var createdAt int64
	var minAmount sql.NullFloat64

	err := s.db.QueryRow(
		`SELECT id, user_id, name, address_raw, address_display, min_amount_ton, created_at
		 FROM wallets WHERE id = ?`,
		walletID,
	).Scan(&w.ID, &w.UserID, &w.Name, &w.AddressRaw, &w.AddressDisplay, &minAmount, &createdAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	w.CreatedAt = time.Unix(createdAt, 0)
	if minAmount.Valid {
		w.MinAmountTON = &minAmount.Float64
	}

	return &w, nil
}

// GetWalletsByRaw returns all wallets with a specific raw address
func (s *Storage) GetWalletsByRaw(addressRaw string) ([]Wallet, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, name, address_raw, address_display, min_amount_ton, created_at
		 FROM wallets WHERE address_raw = ?`,
		addressRaw,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wallets []Wallet
	for rows.Next() {
		var w Wallet
		var createdAt int64
		var minAmount sql.NullFloat64

		err := rows.Scan(&w.ID, &w.UserID, &w.Name, &w.AddressRaw, &w.AddressDisplay, &minAmount, &createdAt)
		if err != nil {
			return nil, err
		}

		w.CreatedAt = time.Unix(createdAt, 0)
		if minAmount.Valid {
			w.MinAmountTON = &minAmount.Float64
		}
		wallets = append(wallets, w)
	}

	return wallets, nil
}

// GetAllWallets returns all wallets in the database
func (s *Storage) GetAllWallets() ([]Wallet, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, name, address_raw, address_display, min_amount_ton, created_at
		 FROM wallets`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wallets []Wallet
	for rows.Next() {
		var w Wallet
		var createdAt int64
		var minAmount sql.NullFloat64

		err := rows.Scan(&w.ID, &w.UserID, &w.Name, &w.AddressRaw, &w.AddressDisplay, &minAmount, &createdAt)
		if err != nil {
			return nil, err
		}

		w.CreatedAt = time.Unix(createdAt, 0)
		if minAmount.Valid {
			w.MinAmountTON = &minAmount.Float64
		}
		wallets = append(wallets, w)
	}

	return wallets, nil
}

// RemoveWallet removes a wallet
func (s *Storage) RemoveWallet(userID, walletID int64) error {
	_, err := s.db.Exec(
		"DELETE FROM wallets WHERE user_id = ? AND id = ?",
		userID, walletID,
	)
	if err != nil {
		return err
	}

	// Also remove processed events
	_, err = s.db.Exec("DELETE FROM processed_events WHERE wallet_id = ?", walletID)
	return err
}

// SetWalletMinAmount sets the minimum amount filter for a wallet
func (s *Storage) SetWalletMinAmount(userID, walletID int64, amount float64) error {
	result, err := s.db.Exec(
		"UPDATE wallets SET min_amount_ton = ? WHERE id = ? AND user_id = ?",
		amount, walletID, userID,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ResetWalletFilters resets all filters for a wallet
func (s *Storage) ResetWalletFilters(userID, walletID int64) error {
	result, err := s.db.Exec(
		"UPDATE wallets SET min_amount_ton = NULL WHERE id = ? AND user_id = ?",
		walletID, userID,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Processed Events ---

// MarkEventProcessed marks an event as processed, returns true if it was new
func (s *Storage) MarkEventProcessed(walletID int64, eventID string) (bool, error) {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO processed_events (wallet_id, event_id) VALUES (?, ?)",
		walletID, eventID,
	)
	if err != nil {
		return false, err
	}

	// Check if it was actually inserted
	var count int
	err = s.db.QueryRow(
		"SELECT changes()",
	).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// --- Premium ---

// IsPremium checks if a user has premium
func (s *Storage) IsPremium(userID int64) bool {
	var count int
	err := s.db.QueryRow(
		"SELECT 1 FROM premium_users WHERE user_id = ?",
		userID,
	).Scan(&count)
	return err == nil
}

// ActivatePremium activates premium for a user
func (s *Storage) ActivatePremium(userID int64, payerAddress, eventID string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(
		`INSERT INTO premium_users (user_id, activated_at, payer_address, event_id)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
			activated_at = excluded.activated_at,
			payer_address = excluded.payer_address,
			event_id = excluded.event_id`,
		userID, now, payerAddress, eventID,
	)
	return err
}

// MarkPremiumPayment records a premium payment, returns true if new
func (s *Storage) MarkPremiumPayment(eventID string, userID int64, amount float64, sender string) (bool, error) {
	result, err := s.db.Exec(
		`INSERT OR IGNORE INTO premium_payments (event_id, user_id, amount, sender_address)
		 VALUES (?, ?, ?, ?)`,
		eventID, userID, amount, sender,
	)
	if err != nil {
		return false, err
	}

	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// RegisterPendingPremium registers a pending premium payment
func (s *Storage) RegisterPendingPremium(userID int64, uniqueAmount float64) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO pending_premium_payments (user_id, unique_amount, created_at)
		 VALUES (?, ?, ?)`,
		userID, uniqueAmount, now,
	)
	return err
}

// GetUserByPremiumAmount finds a user by their unique payment amount
func (s *Storage) GetUserByPremiumAmount(amount float64) (int64, error) {
	var userID int64
	err := s.db.QueryRow(
		`SELECT user_id FROM pending_premium_payments
		 WHERE ABS(unique_amount - ?) < 0.0001
		 ORDER BY created_at DESC LIMIT 1`,
		amount,
	).Scan(&userID)

	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	return userID, err
}

// ClearPendingPremium removes a pending premium payment
func (s *Storage) ClearPendingPremium(userID int64) error {
	_, err := s.db.Exec(
		"DELETE FROM pending_premium_payments WHERE user_id = ?",
		userID,
	)
	return err
}

// GetWalletCount returns the number of wallets for a user
func (s *Storage) GetWalletCount(userID int64) (int, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM wallets WHERE user_id = ?",
		userID,
	).Scan(&count)
	return count, err
}

// GenerateUniqueAmount generates a unique payment amount for a user
func GenerateUniqueAmount(userID int64, basePrice float64) float64 {
	suffix := float64(userID%1000) / 10000.0
	return math.Round((basePrice+suffix)*10000) / 10000
}
