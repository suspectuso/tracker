package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/suspectuso/ton-tracker/internal/config"
	"github.com/suspectuso/ton-tracker/internal/notifier"
	"github.com/suspectuso/ton-tracker/internal/storage"
	"github.com/suspectuso/ton-tracker/internal/telegram"
	"github.com/suspectuso/ton-tracker/internal/tonapi"
	"github.com/suspectuso/ton-tracker/internal/webhook"
)

func main() {
	// Setup logger
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(log)

	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Debug("no .env file found")
	}

	// Load config
	cfg := config.Load()

	if cfg.BotToken == "" {
		log.Error("BOT_TOKEN is required")
		os.Exit(1)
	}

	// Initialize storage
	store, err := storage.New(cfg.DBPath)
	if err != nil {
		log.Error("init storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	log.Info("storage initialized", "path", cfg.DBPath)

	// Initialize TonAPI client
	tonAPI := tonapi.NewClient(cfg.TonAPIBaseURL, cfg.TonAPIKey)
	log.Info("tonapi client initialized", "base_url", cfg.TonAPIBaseURL)

	// Initialize telegram bot
	bot, err := telegram.New(cfg, store, tonAPI, log)
	if err != nil {
		log.Error("init telegram bot", "error", err)
		os.Exit(1)
	}
	log.Info("telegram bot initialized")

	// Initialize notifier
	notify := notifier.New(cfg, store, bot, log)

	// Initialize webhook manager
	webhookManager := webhook.NewManager(store, tonAPI, cfg.WebhookEndpoint, log)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize webhook
	if cfg.WebhookEndpoint != "" {
		if err := webhookManager.Init(ctx); err != nil {
			log.Error("init webhook", "error", err)
		} else {
			log.Info("webhook initialized", "endpoint", cfg.WebhookEndpoint)
		}
	}

	// Start webhook server
	webhookServer := webhook.NewServer(store, tonAPI, notify.HandleEvent, log)
	go func() {
		if err := webhookServer.Start(ctx, cfg.WebhookPort); err != nil && err != http.ErrServerClosed {
			log.Error("webhook server", "error", err)
		}
	}()

	// Start webhook sync loop
	go webhookManager.SyncLoop(ctx, 30*time.Second)

	// Start premium checker
	premiumChecker := notifier.NewPremiumChecker(cfg, store, tonAPI, bot, log)
	go premiumChecker.Start(ctx, 10*time.Second)

	// Seed all wallets (mark existing events as processed)
	go seedAllWallets(ctx, store, tonAPI, log)

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Info("shutting down...")
		cancel()
	}()

	// Start bot polling
	log.Info("starting bot polling...")
	bot.Start(ctx)
}

// seedAllWallets marks all existing events as processed to avoid sending old notifications
func seedAllWallets(ctx context.Context, store *storage.Storage, tonAPI *tonapi.Client, log *slog.Logger) {
	wallets, err := store.GetAllWallets()
	if err != nil {
		log.Error("get all wallets for seeding", "error", err)
		return
	}

	if len(wallets) == 0 {
		log.Info("no wallets to seed")
		return
	}

	log.Info("seeding wallets", "count", len(wallets))

	totalSeeded := 0
	for _, w := range wallets {
		events, err := tonAPI.GetEvents(ctx, w.AddressRaw, 5)
		if err != nil {
			log.Warn("fetch events for seeding", "wallet_id", w.ID, "error", err)
			continue
		}

		for _, ev := range events {
			if ev.EventID != "" {
				isNew, _ := store.MarkEventProcessed(w.ID, ev.EventID)
				if isNew {
					totalSeeded++
				}
			}
		}
	}

	log.Info("seeding complete", "events_marked", totalSeeded)
}
