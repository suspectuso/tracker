package notifier

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/suspectuso/ton-tracker/internal/config"
	"github.com/suspectuso/ton-tracker/internal/storage"
	"github.com/suspectuso/ton-tracker/internal/telegram"
	"github.com/suspectuso/ton-tracker/internal/tonapi"
)

// Notifier processes events and sends notifications to users
type Notifier struct {
	cfg     *config.Config
	storage *storage.Storage
	bot     *telegram.Bot
	log     *slog.Logger
}

// New creates a new Notifier
func New(cfg *config.Config, store *storage.Storage, bot *telegram.Bot, log *slog.Logger) *Notifier {
	return &Notifier{
		cfg:     cfg,
		storage: store,
		bot:     bot,
		log:     log,
	}
}

// HandleEvent processes an event and sends notifications
func (n *Notifier) HandleEvent(ctx context.Context, wallet *storage.Wallet, event *tonapi.Event) {
	n.log.Info("handling event",
		"event_id", event.EventID,
		"wallet_id", wallet.ID,
		"user_id", wallet.UserID,
		"actions", len(event.Actions),
	)

	// Extract swaps and transfers
	swaps := n.extractSwaps(event)
	transfers := n.extractTransfers(event, wallet.AddressRaw)

	// Process swaps
	for _, swap := range swaps {
		// Apply min amount filter
		if wallet.MinAmountTON != nil && swap.TonAmount < *wallet.MinAmountTON {
			n.log.Debug("skipping swap below min amount",
				"ton_amount", swap.TonAmount,
				"min_amount", *wallet.MinAmountTON,
			)
			continue
		}

		text := n.formatSwapMessage(wallet, swap)
		if err := n.bot.SendNotification(ctx, wallet.UserID, text, nil); err != nil {
			n.log.Error("send swap notification", "error", err)
		}
	}

	// Process transfers (only if no swaps to avoid duplicates from swap fees)
	if len(swaps) == 0 {
		for _, tr := range transfers {
			// Apply min amount filter
			if wallet.MinAmountTON != nil && tr.Amount < *wallet.MinAmountTON {
				continue
			}

			// Apply global min transfer filter
			if tr.Amount < n.cfg.MinTransferTON {
				continue
			}

			text := n.formatTransferMessage(wallet, tr)
			if err := n.bot.SendNotification(ctx, wallet.UserID, text, nil); err != nil {
				n.log.Error("send transfer notification", "error", err)
			}
		}
	}
}

// Swap represents a parsed swap
type Swap struct {
	Dex           string
	Side          string // "buy" or "sell"
	FromSymbol    string
	FromAmount    float64
	ToSymbol      string
	ToAmount      float64
	TonAmount     float64
	JettonSymbol  string
	JettonAmount  float64
	JettonMaster  string
}

// Transfer represents a parsed transfer
type Transfer struct {
	Direction string // "in" or "out"
	Amount    float64
	Sender    string
	Recipient string
	Comment   string
}

func (n *Notifier) extractSwaps(event *tonapi.Event) []Swap {
	var swaps []Swap

	for _, action := range event.Actions {
		if action.Type != "JettonSwap" || action.JettonSwap == nil {
			continue
		}

		js := action.JettonSwap
		swap := Swap{
			Dex: js.Dex,
		}

		// Determine buy/sell based on TON in/out
		if js.TonIn > 0 {
			// Buying jetton with TON
			swap.Side = "buy"
			swap.FromSymbol = "TON"
			swap.FromAmount = tonapi.NanoToTON(js.TonIn)
			swap.TonAmount = swap.FromAmount

			if js.JettonMasterOut != nil {
				swap.ToSymbol = js.JettonMasterOut.Symbol
				swap.ToAmount = tonapi.JettonUnitsToAmount(js.AmountOut, js.JettonMasterOut.Decimals)
				swap.JettonSymbol = js.JettonMasterOut.Symbol
				swap.JettonAmount = swap.ToAmount
				swap.JettonMaster = js.JettonMasterOut.Address
			}
		} else if js.TonOut > 0 {
			// Selling jetton for TON
			swap.Side = "sell"
			swap.ToSymbol = "TON"
			swap.ToAmount = tonapi.NanoToTON(js.TonOut)
			swap.TonAmount = swap.ToAmount

			if js.JettonMasterIn != nil {
				swap.FromSymbol = js.JettonMasterIn.Symbol
				swap.FromAmount = tonapi.JettonUnitsToAmount(js.AmountIn, js.JettonMasterIn.Decimals)
				swap.JettonSymbol = js.JettonMasterIn.Symbol
				swap.JettonAmount = swap.FromAmount
				swap.JettonMaster = js.JettonMasterIn.Address
			}
		}

		swaps = append(swaps, swap)
	}

	return swaps
}

func (n *Notifier) extractTransfers(event *tonapi.Event, watchedRaw string) []Transfer {
	var transfers []Transfer

	for _, action := range event.Actions {
		if action.Type != "TonTransfer" || action.TonTransfer == nil {
			continue
		}

		tt := action.TonTransfer
		tr := Transfer{
			Amount:    tonapi.NanoToTON(tt.Amount),
			Sender:    tt.Sender.Address,
			Recipient: tt.Recipient.Address,
			Comment:   tt.Comment,
		}

		if tt.Recipient.Address == watchedRaw {
			tr.Direction = "in"
		} else if tt.Sender.Address == watchedRaw {
			tr.Direction = "out"
		} else {
			continue
		}

		transfers = append(transfers, tr)
	}

	return transfers
}

func (n *Notifier) formatSwapMessage(wallet *storage.Wallet, swap Swap) string {
	var emoji, sideWord string
	switch swap.Side {
	case "buy":
		emoji = "✅"
		sideWord = "BUY"
	case "sell":
		emoji = "🔻"
		sideWord = "SELL"
	default:
		emoji = "🔁"
		sideWord = "SWAP"
	}

	// Format DEX name nicely
	dexDisplay := formatDex(swap.Dex)

	// Wallet link
	nameLink := fmt.Sprintf("<a href='https://tonviewer.com/%s'>%s</a>",
		wallet.AddressDisplay, wallet.Name)

	// Format amounts
	var pairLine string
	if swap.Side == "buy" {
		pairLine = fmt.Sprintf("%.2f TON 🔄 %s %s",
			swap.FromAmount, formatNumber(swap.ToAmount), swap.ToSymbol)
	} else {
		pairLine = fmt.Sprintf("%s %s 🔄 %.2f TON",
			formatNumber(swap.FromAmount), swap.FromSymbol, swap.ToAmount)
	}

	// Token address
	jettonAddr := ""
	if swap.JettonMaster != "" {
		friendly := tonapi.RawToFriendly(swap.JettonMaster)
		jettonAddr = fmt.Sprintf("\n\n<code>%s</code>", friendly)
	}

	return fmt.Sprintf(
		"%s <b>%s by %s</b>\n"+
			"<i>via %s</i>\n\n"+
			"%s%s",
		emoji, sideWord, nameLink,
		dexDisplay,
		pairLine, jettonAddr,
	)
}

func (n *Notifier) formatTransferMessage(wallet *storage.Wallet, tr Transfer) string {
	var emoji, sign string
	if tr.Direction == "in" {
		emoji = "🟩"
		sign = "+"
	} else {
		emoji = "🟥"
		sign = "-"
	}

	senderFriendly := tonapi.RawToFriendly(tr.Sender)
	recipientFriendly := tonapi.RawToFriendly(tr.Recipient)

	// Determine display names
	var senderText, recipientText string
	if tr.Sender == wallet.AddressRaw {
		senderText = wallet.Name
	} else {
		senderText = tonapi.ShortAddr(senderFriendly, 4)
	}

	if tr.Recipient == wallet.AddressRaw {
		recipientText = wallet.Name
	} else {
		recipientText = tonapi.ShortAddr(recipientFriendly, 4)
	}

	senderLink := fmt.Sprintf("<a href='https://tonviewer.com/%s'>%s</a>", senderFriendly, senderText)
	recipientLink := fmt.Sprintf("<a href='https://tonviewer.com/%s'>%s</a>", recipientFriendly, recipientText)

	lines := []string{
		"<b>🔔 Transfer detected</b>",
		"",
		fmt.Sprintf("%s%.2f TON %s", sign, tr.Amount, emoji),
		"",
		fmt.Sprintf("%s → %s", senderLink, recipientLink),
	}

	if tr.Comment != "" {
		lines = append(lines, "", fmt.Sprintf("💬 Comment: <code>%s</code>", tr.Comment))
	}

	return strings.Join(lines, "\n")
}

func formatDex(dex string) string {
	switch strings.ToLower(dex) {
	case "stonfi", "ston.fi":
		return "STON.fi"
	case "dedust":
		return "DeDust"
	case "megaton", "megatonfi":
		return "Megaton"
	default:
		if dex != "" {
			return strings.Title(dex)
		}
		return "DEX"
	}
}

func formatNumber(num float64) string {
	abs := num
	if abs < 0 {
		abs = -abs
	}

	switch {
	case abs >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", num/1_000_000_000)
	case abs >= 1_000_000:
		return fmt.Sprintf("%.2fM", num/1_000_000)
	case abs >= 1_000:
		return fmt.Sprintf("%.2fK", num/1_000)
	default:
		return fmt.Sprintf("%.2f", num)
	}
}
