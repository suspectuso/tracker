package notifier

import (
	"context"
	"log/slog"
	"regexp"
	"time"

	"github.com/suspectuso/ton-tracker/internal/config"
	"github.com/suspectuso/ton-tracker/internal/storage"
	"github.com/suspectuso/ton-tracker/internal/telegram"
	"github.com/suspectuso/ton-tracker/internal/tonapi"
)

var tgIDRegex = regexp.MustCompile(`(\d{5,15})`)

// PremiumChecker monitors service wallet for premium payments
type PremiumChecker struct {
	cfg     *config.Config
	storage *storage.Storage
	tonAPI  *tonapi.Client
	bot     *telegram.Bot
	log     *slog.Logger

	serviceWalletRaw string
}

// NewPremiumChecker creates a new premium checker
func NewPremiumChecker(cfg *config.Config, store *storage.Storage, tonAPI *tonapi.Client, bot *telegram.Bot, log *slog.Logger) *PremiumChecker {
	serviceRaw := ""
	if cfg.ServiceWalletAddr != "" {
		serviceRaw = tonapi.NormalizeAddress(cfg.ServiceWalletAddr)
	}

	return &PremiumChecker{
		cfg:              cfg,
		storage:          store,
		tonAPI:           tonAPI,
		bot:              bot,
		log:              log,
		serviceWalletRaw: serviceRaw,
	}
}

// Start starts the premium checker loop
func (pc *PremiumChecker) Start(ctx context.Context, interval time.Duration) {
	if pc.serviceWalletRaw == "" {
		pc.log.Info("premium checker disabled: SERVICE_WALLET_ADDR not set")
		return
	}

	pc.log.Info("premium checker started",
		"service_wallet", pc.cfg.ServiceWalletAddr,
		"interval", interval,
	)

	time.Sleep(5 * time.Second) // Initial delay

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := pc.checkPayments(ctx); err != nil {
				pc.log.Error("check payments", "error", err)
			}
		}
	}
}

func (pc *PremiumChecker) checkPayments(ctx context.Context) error {
	events, err := pc.tonAPI.GetEvents(ctx, pc.serviceWalletRaw, 20)
	if err != nil {
		return err
	}

	for _, event := range events {
		pc.processEvent(ctx, &event)
	}

	return nil
}

func (pc *PremiumChecker) processEvent(ctx context.Context, event *tonapi.Event) {
	for _, action := range event.Actions {
		if action.Type != "TonTransfer" || action.TonTransfer == nil {
			continue
		}

		tt := action.TonTransfer

		// Only incoming transfers to service wallet
		recipientRaw := tonapi.NormalizeAddress(tt.Recipient.Address)
		if recipientRaw != pc.serviceWalletRaw {
			continue
		}

		amount := tonapi.NanoToTON(tt.Amount)

		// Check if amount is enough for premium (with small tolerance)
		if amount+0.000001 < pc.cfg.PremiumPriceTON {
			continue
		}

		// Try to get user ID from comment
		var userID int64
		matches := tgIDRegex.FindStringSubmatch(tt.Comment)
		if len(matches) > 0 {
			var err error
			userID, err = parseUserID(matches[1])
			if err != nil {
				continue
			}
		} else {
			// Try to find user by unique amount
			var err error
			userID, err = pc.storage.GetUserByPremiumAmount(amount)
			if err != nil {
				pc.log.Debug("premium payment without user ID",
					"amount", amount,
					"sender", tt.Sender.Address,
				)
				continue
			}
			pc.log.Info("found user by unique amount",
				"user_id", userID,
				"amount", amount,
			)
		}

		// Check if already processed
		isNew, err := pc.storage.MarkPremiumPayment(event.EventID, userID, amount, tt.Sender.Address)
		if err != nil {
			pc.log.Error("mark premium payment", "error", err)
			continue
		}
		if !isNew {
			continue
		}

		// Activate premium
		if err := pc.storage.ActivatePremium(userID, tt.Sender.Address, event.EventID); err != nil {
			pc.log.Error("activate premium", "error", err)
			continue
		}

		// Clear pending payment
		pc.storage.ClearPendingPremium(userID)

		pc.log.Info("premium activated",
			"user_id", userID,
			"amount", amount,
			"sender", tt.Sender.Address,
			"event_id", event.EventID,
		)

		// Notify user
		text := "⭐ <b>Premium активирован!</b>\n\n" +
			"Теперь твой лимит — до <b>" + formatNumber(float64(pc.cfg.PremiumMaxWalletsPerUser)) + "</b> кошельков.\n" +
			"Спасибо за поддержку 💙"

		if err := pc.bot.SendNotification(ctx, userID, text, nil); err != nil {
			pc.log.Error("send premium notification", "error", err)
		}
	}
}

func parseUserID(s string) (int64, error) {
	var id int64
	for _, c := range s {
		id = id*10 + int64(c-'0')
	}
	return id, nil
}
