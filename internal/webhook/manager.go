package webhook

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/suspectuso/ton-tracker/internal/storage"
	"github.com/suspectuso/ton-tracker/internal/tonapi"
)

// Manager manages TonAPI webhook subscriptions
type Manager struct {
	storage    *storage.Storage
	tonAPI     *tonapi.Client
	endpoint   string
	log        *slog.Logger

	mu          sync.Mutex
	webhookID   int64
	subscribed  map[string]bool
}

// NewManager creates a new webhook manager
func NewManager(store *storage.Storage, tonAPI *tonapi.Client, endpoint string, log *slog.Logger) *Manager {
	return &Manager{
		storage:    store,
		tonAPI:     tonAPI,
		endpoint:   endpoint,
		log:        log,
		subscribed: make(map[string]bool),
	}
}

// Init initializes the webhook, creating it if necessary
func (m *Manager) Init(ctx context.Context) error {
	if m.endpoint == "" {
		m.log.Warn("webhook endpoint not set, skipping webhook init")
		return nil
	}

	// List existing webhooks
	webhooks, err := m.tonAPI.ListWebhooks(ctx)
	if err != nil {
		return err
	}

	// Find or create webhook
	for _, wh := range webhooks {
		if wh.Endpoint == m.endpoint {
			m.webhookID = wh.ID
			m.log.Info("using existing webhook", "id", wh.ID)
			return nil
		}
	}

	// Create new webhook
	webhook, err := m.tonAPI.CreateWebhook(ctx, m.endpoint)
	if err != nil {
		return err
	}

	m.webhookID = webhook.ID
	m.log.Info("created new webhook", "id", webhook.ID)

	return nil
}

// SyncLoop periodically syncs subscriptions with wallets in DB
func (m *Manager) SyncLoop(ctx context.Context, interval time.Duration) {
	if m.endpoint == "" {
		return
	}

	// Initial sync after delay
	time.Sleep(5 * time.Second)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	m.log.Info("webhook sync loop started", "interval", interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.sync(ctx); err != nil {
				m.log.Error("sync subscriptions", "error", err)
			}
		}
	}
}

func (m *Manager) sync(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.webhookID == 0 {
		return nil
	}

	// Get all wallets from DB
	wallets, err := m.storage.GetAllWallets()
	if err != nil {
		return err
	}

	// Build set of addresses we need
	needed := make(map[string]bool)
	for _, w := range wallets {
		needed[w.AddressRaw] = true
	}

	// Find addresses to add and remove
	var toAdd []string
	for addr := range needed {
		if !m.subscribed[addr] {
			toAdd = append(toAdd, addr)
		}
	}

	var toRemove []string
	for addr := range m.subscribed {
		if !needed[addr] {
			toRemove = append(toRemove, addr)
		}
	}

	// Subscribe new addresses
	if len(toAdd) > 0 {
		if err := m.tonAPI.SubscribeAccounts(ctx, m.webhookID, toAdd); err != nil {
			m.log.Error("subscribe accounts", "error", err, "count", len(toAdd))
		} else {
			for _, addr := range toAdd {
				m.subscribed[addr] = true
			}
			m.log.Info("subscribed accounts", "count", len(toAdd))
		}
	}

	// Unsubscribe removed addresses
	if len(toRemove) > 0 {
		if err := m.tonAPI.UnsubscribeAccounts(ctx, m.webhookID, toRemove); err != nil {
			m.log.Error("unsubscribe accounts", "error", err, "count", len(toRemove))
		} else {
			for _, addr := range toRemove {
				delete(m.subscribed, addr)
			}
			m.log.Info("unsubscribed accounts", "count", len(toRemove))
		}
	}

	return nil
}

// GetWebhookID returns the current webhook ID
func (m *Manager) GetWebhookID() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.webhookID
}
