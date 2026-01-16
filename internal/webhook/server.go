package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/suspectuso/ton-tracker/internal/storage"
	"github.com/suspectuso/ton-tracker/internal/tonapi"
)

// EventHandler is a function that handles incoming events
type EventHandler func(ctx context.Context, wallet *storage.Wallet, event *tonapi.Event)

// Server handles incoming webhooks from TonAPI
type Server struct {
	storage  *storage.Storage
	tonAPI   *tonapi.Client
	handler  EventHandler
	log      *slog.Logger

	server *http.Server
}

// NewServer creates a new webhook server
func NewServer(store *storage.Storage, tonAPI *tonapi.Client, handler EventHandler, log *slog.Logger) *Server {
	return &Server{
		storage: store,
		tonAPI:  tonAPI,
		handler: handler,
		log:     log,
	}
}

// Start starts the webhook server
func (s *Server) Start(ctx context.Context, port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", s.handleWebhook)
	mux.HandleFunc("/webhook/", s.handleWebhook)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", s.handleHealth)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	s.log.Info("starting webhook server", "port", port)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(shutdownCtx)
	}()

	return s.server.ListenAndServe()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload tonapi.WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.log.Warn("invalid webhook payload", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Ignore mempool and new_contract events
	if payload.EventType == "mempool_msg" || payload.EventType == "new_contract" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Account transaction event
	if payload.AccountID == "" {
		s.log.Warn("missing account_id in webhook")
		w.WriteHeader(http.StatusOK)
		return
	}

	s.log.Debug("webhook received",
		"account", payload.AccountID[:10]+"...",
		"tx_hash", truncate(payload.TxHash, 10),
		"has_event", payload.Event != nil,
	)

	// Process asynchronously
	go s.processTransaction(context.Background(), payload)

	w.WriteHeader(http.StatusOK)
}

func (s *Server) processTransaction(ctx context.Context, payload tonapi.WebhookPayload) {
	// Find wallets by address
	wallets, err := s.storage.GetWalletsByRaw(payload.AccountID)
	if err != nil {
		s.log.Error("get wallets by raw", "error", err)
		return
	}

	if len(wallets) == 0 {
		s.log.Debug("no wallets found for account", "account", payload.AccountID[:10]+"...")
		return
	}

	// Get event (from payload or fetch)
	var event *tonapi.Event
	if payload.Event != nil {
		event = payload.Event
	} else if payload.TxHash != "" {
		var err error
		event, err = s.tonAPI.GetEventByHash(ctx, payload.TxHash)
		if err != nil {
			s.log.Warn("fetch event by hash", "error", err, "tx_hash", payload.TxHash)
			return
		}
	} else {
		s.log.Warn("no event data and no tx_hash")
		return
	}

	if event.EventID == "" {
		s.log.Warn("no event_id in event")
		return
	}

	s.log.Info("processing event",
		"event_id", event.EventID,
		"wallets", len(wallets),
	)

	// Process for each wallet in parallel
	var wg sync.WaitGroup
	for _, w := range wallets {
		wallet := w // capture

		// Check if already processed
		isNew, err := s.storage.MarkEventProcessed(wallet.ID, event.EventID)
		if err != nil {
			s.log.Error("mark event processed", "error", err)
			continue
		}
		if !isNew {
			s.log.Debug("event already processed",
				"event_id", event.EventID,
				"wallet_id", wallet.ID,
			)
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handler(ctx, &wallet, event)
		}()
	}
	wg.Wait()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
