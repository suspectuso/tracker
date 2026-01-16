package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/suspectuso/ton-tracker/internal/config"
	"github.com/suspectuso/ton-tracker/internal/storage"
	"github.com/suspectuso/ton-tracker/internal/tonapi"
)

var addrRegex = regexp.MustCompile(`(0:[0-9A-Za-z:_-]{20,}|[UE]Q[0-9A-Za-z:_-]{20,})`)

// Bot wraps the telegram bot with handlers
type Bot struct {
	bot      *bot.Bot
	cfg      *config.Config
	storage  *storage.Storage
	tonAPI   *tonapi.Client
	states   *StateManager
	log      *slog.Logger
}

// New creates a new telegram bot
func New(cfg *config.Config, store *storage.Storage, tonAPI *tonapi.Client, log *slog.Logger) (*Bot, error) {
	b := &Bot{
		cfg:     cfg,
		storage: store,
		tonAPI:  tonAPI,
		states:  NewStateManager(),
		log:     log,
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(b.defaultHandler),
		bot.WithCallbackQueryDataHandler("", bot.MatchTypePrefix, b.callbackHandler),
	}

	tgBot, err := bot.New(cfg.BotToken, opts...)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}

	b.bot = tgBot

	// Register command handlers
	tgBot.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, b.startHandler)
	tgBot.RegisterHandler(bot.HandlerTypeMessageText, "/start ", bot.MatchTypePrefix, b.startHandler)
	tgBot.RegisterHandler(bot.HandlerTypeMessageText, "/me", bot.MatchTypeExact, b.meHandler)

	return b, nil
}

// Start starts the bot polling
func (b *Bot) Start(ctx context.Context) {
	b.bot.Start(ctx)
}

// GetBot returns the underlying bot instance
func (b *Bot) GetBot() *bot.Bot {
	return b.bot
}

// --- Handlers ---

func (b *Bot) startHandler(ctx context.Context, tgBot *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	userID := update.Message.From.ID
	userName := update.Message.From.FirstName
	if userName == "" {
		userName = update.Message.From.Username
	}
	if userName == "" {
		userName = "друг"
	}

	limit := b.getMaxWallets(userID)
	vipNote := ""
	if b.cfg.VIPUserIDs[userID] {
		vipNote = " (VIP)"
	}

	text := fmt.Sprintf(
		"<a href='tg://user?id=%d'>%s</a>, добро пожаловать в <b>TON Tracker</b>! 🚀\n\n"+
			"Я отслеживаю TON-кошельки и мгновенно уведомляю о:\n"+
			"• Переводах TON\n"+
			"• Свопах на DEX (STON.fi, DeDust)\n\n"+
			"Текущий лимит: <b>%d</b> кошельков%s\n\n"+
			"Выбери действие 👇",
		userID, userName, limit, vipNote,
	)

	b.sendMessage(ctx, update.Message.Chat.ID, text, MainKeyboard())
}

func (b *Bot) meHandler(ctx context.Context, tgBot *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	userID := update.Message.From.ID
	limit := b.getMaxWallets(userID)

	var flags []string
	if b.cfg.VIPUserIDs[userID] {
		flags = append(flags, "VIP")
	}
	if b.storage.IsPremium(userID) {
		flags = append(flags, "Premium")
	}
	if len(flags) == 0 {
		flags = append(flags, "обычный")
	}

	count, _ := b.storage.GetWalletCount(userID)

	text := fmt.Sprintf(
		"👤 <b>Твой профиль</b>\n\n"+
			"Статус: <b>%s</b>\n"+
			"Кошельков: <b>%d/%d</b>",
		strings.Join(flags, ", "), count, limit,
	)

	b.sendMessage(ctx, update.Message.Chat.ID, text, MainKeyboard())
}

func (b *Bot) defaultHandler(ctx context.Context, tgBot *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}

	userID := update.Message.From.ID
	text := strings.TrimSpace(update.Message.Text)

	state := b.states.Get(userID)
	if state == nil {
		return
	}

	switch state.State {
	case StateWaitName:
		b.handleWaitName(ctx, update.Message, text, state)
	case StateWaitAddress:
		b.handleWaitAddress(ctx, update.Message, text, state)
	case StateWaitMinAmount:
		b.handleWaitMinAmount(ctx, update.Message, text, state)
	}
}

func (b *Bot) handleWaitName(ctx context.Context, msg *models.Message, name string, state *UserState) {
	if len(name) < 2 {
		b.sendMessage(ctx, msg.Chat.ID, "Название слишком короткое, попробуй ещё раз.", nil)
		return
	}

	state.Data["name"] = name
	b.states.Set(msg.From.ID, StateWaitAddress, state.Data)

	b.sendMessage(ctx, msg.Chat.ID,
		"🔹 Теперь отправь адрес TON кошелька\n(можно ссылкой с tonviewer/tonscan):",
		BackKeyboard(),
	)
}

func (b *Bot) handleWaitAddress(ctx context.Context, msg *models.Message, text string, state *UserState) {
	userID := msg.From.ID

	// Extract address from text
	addr := extractAddress(text)
	if addr == "" {
		b.sendMessage(ctx, msg.Chat.ID,
			"❌ Адрес не похож на TON. Попробуй ещё раз.",
			nil,
		)
		return
	}

	// Resolve address via TonAPI
	info, err := b.tonAPI.GetAccountInfo(ctx, addr)
	if err != nil {
		b.log.Error("resolve address", "error", err)
		b.sendMessage(ctx, msg.Chat.ID,
			"❌ Не удалось проверить адрес. Попробуй ещё раз.",
			nil,
		)
		return
	}

	name := state.Data["name"].(string)
	maxWallets := b.getMaxWallets(userID)

	wallet, err := b.storage.AddWallet(userID, name, info.Address, addr, maxWallets)
	b.states.Clear(userID)

	if err == storage.ErrLimitReached {
		b.sendMessage(ctx, msg.Chat.ID,
			fmt.Sprintf("❌ Достигнут лимит в %d кошельков.\nОформи Premium для увеличения лимита.", maxWallets),
			MainKeyboard(),
		)
		return
	}
	if err != nil {
		b.log.Error("add wallet", "error", err)
		b.sendMessage(ctx, msg.Chat.ID, "❌ Ошибка при добавлении кошелька.", MainKeyboard())
		return
	}

	b.log.Info("wallet added",
		"user_id", userID,
		"wallet_id", wallet.ID,
		"address", wallet.AddressRaw,
	)

	b.sendMessage(ctx, msg.Chat.ID, "✅ Кошелёк добавлен!", MainKeyboard())
}

func (b *Bot) handleWaitMinAmount(ctx context.Context, msg *models.Message, text string, state *UserState) {
	userID := msg.From.ID

	amount, err := strconv.ParseFloat(strings.Replace(text, ",", ".", 1), 64)
	if err != nil || amount < 0 {
		b.sendMessage(ctx, msg.Chat.ID,
			"❌ Введи положительное число. Например: <code>0.5</code> или <code>10</code>",
			nil,
		)
		return
	}

	walletID := state.Data["wallet_id"].(int64)
	b.states.Clear(userID)

	err = b.storage.SetWalletMinAmount(userID, walletID, amount)
	if err != nil {
		b.sendMessage(ctx, msg.Chat.ID, "❌ Ошибка при обновлении фильтра.", nil)
		return
	}

	b.sendMessage(ctx, msg.Chat.ID,
		fmt.Sprintf("✅ Минимальная сумма установлена: <b>%.2f TON</b>", amount),
		StartMenuKeyboard(),
	)
}

func (b *Bot) callbackHandler(ctx context.Context, tgBot *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	cb := update.CallbackQuery
	userID := cb.From.ID
	data := cb.Data

	// Answer callback to remove loading state
	tgBot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cb.ID,
	})

	switch {
	case data == "back":
		b.showMainMenu(ctx, cb)
	case data == "add":
		b.handleAdd(ctx, cb)
	case data == "list":
		b.showWalletList(ctx, cb)
	case strings.HasPrefix(data, "del:"):
		b.handleDelete(ctx, cb, data)
	case strings.HasPrefix(data, "cfg:"):
		b.handleSettings(ctx, cb, data)
	case strings.HasPrefix(data, "cfg_min:"):
		b.handleSetMinAmount(ctx, cb, data)
	case strings.HasPrefix(data, "cfg_reset:"):
		b.handleResetFilters(ctx, cb, data)
	case data == "premium":
		b.showPremium(ctx, cb)
	case data == "pay_wallet":
		b.handlePayWallet(ctx, cb)
	case data == "check_payment":
		b.handleCheckPayment(ctx, cb)
	default:
		b.log.Warn("unknown callback", "data", data, "user_id", userID)
	}
}

func (b *Bot) showMainMenu(ctx context.Context, cb *models.CallbackQuery) {
	userID := cb.From.ID
	userName := cb.From.FirstName
	if userName == "" {
		userName = cb.From.Username
	}
	if userName == "" {
		userName = "друг"
	}

	limit := b.getMaxWallets(userID)
	vipNote := ""
	if b.cfg.VIPUserIDs[userID] {
		vipNote = " (VIP)"
	}

	text := fmt.Sprintf(
		"<a href='tg://user?id=%d'>%s</a>, добро пожаловать в <b>TON Tracker</b>! 🚀\n\n"+
			"Я отслеживаю TON-кошельки и мгновенно уведомляю о:\n"+
			"• Переводах TON\n"+
			"• Свопах на DEX (STON.fi, DeDust)\n\n"+
			"Текущий лимит: <b>%d</b> кошельков%s\n\n"+
			"Выбери действие 👇",
		userID, userName, limit, vipNote,
	)

	b.editMessage(ctx, cb.Message, text, MainKeyboard())
}

func (b *Bot) handleAdd(ctx context.Context, cb *models.CallbackQuery) {
	b.states.Set(cb.From.ID, StateWaitName, nil)
	b.editMessage(ctx, cb.Message, "🔹 Введи название для нового кошелька:", BackKeyboard())
}

func (b *Bot) showWalletList(ctx context.Context, cb *models.CallbackQuery) {
	wallets, err := b.storage.ListWallets(cb.From.ID)
	if err != nil {
		b.log.Error("list wallets", "error", err)
		return
	}

	if len(wallets) == 0 {
		b.editMessage(ctx, cb.Message, "❌ У тебя нет добавленных кошельков.", MainKeyboard())
		return
	}

	limit := b.getMaxWallets(cb.From.ID)

	var lines []string
	lines = append(lines, "📋 <b>Твои кошельки:</b>\n")
	for _, w := range wallets {
		lines = append(lines, fmt.Sprintf("• <b>%s</b> — %s", w.Name, w.AddressDisplay))
	}
	lines = append(lines, fmt.Sprintf("\nЛимит: <b>%d</b> кошельков", limit))

	b.editMessage(ctx, cb.Message, strings.Join(lines, "\n"), WalletsKeyboard(wallets))
}

func (b *Bot) handleDelete(ctx context.Context, cb *models.CallbackQuery, data string) {
	walletID, _ := strconv.ParseInt(strings.TrimPrefix(data, "del:"), 10, 64)

	err := b.storage.RemoveWallet(cb.From.ID, walletID)
	if err != nil {
		b.log.Error("remove wallet", "error", err)
	}

	// Refresh wallet list
	b.showWalletList(ctx, cb)
}

func (b *Bot) handleSettings(ctx context.Context, cb *models.CallbackQuery, data string) {
	walletID, _ := strconv.ParseInt(strings.TrimPrefix(data, "cfg:"), 10, 64)

	wallet, err := b.storage.GetWallet(walletID)
	if err != nil || wallet.UserID != cb.From.ID {
		b.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            "Кошелёк не найден",
			ShowAlert:       true,
		})
		return
	}

	minLine := "Минимальная сумма: <b>не установлена</b>"
	if wallet.MinAmountTON != nil {
		minLine = fmt.Sprintf("Минимальная сумма: <b>%.2f TON</b>", *wallet.MinAmountTON)
	}

	text := fmt.Sprintf("⚙️ <b>Настройки: %s</b>\n\n%s", wallet.Name, minLine)
	b.editMessage(ctx, cb.Message, text, WalletSettingsKeyboard(walletID))
}

func (b *Bot) handleSetMinAmount(ctx context.Context, cb *models.CallbackQuery, data string) {
	walletID, _ := strconv.ParseInt(strings.TrimPrefix(data, "cfg_min:"), 10, 64)

	b.states.Set(cb.From.ID, StateWaitMinAmount, map[string]interface{}{
		"wallet_id": walletID,
	})

	b.editMessage(ctx, cb.Message,
		"🔢 Введи минимальную сумму в TON.\nНапример: <code>0.5</code> или <code>10</code>",
		nil,
	)
}

func (b *Bot) handleResetFilters(ctx context.Context, cb *models.CallbackQuery, data string) {
	walletID, _ := strconv.ParseInt(strings.TrimPrefix(data, "cfg_reset:"), 10, 64)

	err := b.storage.ResetWalletFilters(cb.From.ID, walletID)
	if err != nil {
		b.log.Error("reset filters", "error", err)
	}

	// Refresh settings view
	b.handleSettings(ctx, cb, fmt.Sprintf("cfg:%d", walletID))
}

func (b *Bot) showPremium(ctx context.Context, cb *models.CallbackQuery) {
	text := fmt.Sprintf(
		"⭐ <b>Premium TON Tracker</b>\n\n"+
			"• Увеличенный лимит до <b>%d</b> кошельков\n"+
			"• Приоритет в обработке\n\n"+
			"💎 Стоимость: <b>%.0f TON</b>",
		b.cfg.PremiumMaxWalletsPerUser, b.cfg.PremiumPriceTON,
	)

	b.editMessage(ctx, cb.Message, text, PremiumKeyboard())
}

func (b *Bot) handlePayWallet(ctx context.Context, cb *models.CallbackQuery) {
	userID := cb.From.ID

	// Generate unique amount
	uniqueAmount := storage.GenerateUniqueAmount(userID, b.cfg.PremiumPriceTON)
	b.storage.RegisterPendingPremium(userID, uniqueAmount)

	text := fmt.Sprintf(
		"💼 <b>Оплата Premium</b>\n\n"+
			"Переведи <b>%.4f TON</b> на кошелёк:\n\n"+
			"<code>%s</code>\n\n"+
			"⚠️ <b>Важно:</b> переведи точно указанную сумму!\n"+
			"Это позволит определить твой платёж без комментария.\n\n"+
			"После оплаты нажми «Проверить оплату» 👇",
		uniqueAmount, b.cfg.ServiceWalletAddr,
	)

	b.editMessage(ctx, cb.Message, text, CheckPaymentKeyboard())
}

func (b *Bot) handleCheckPayment(ctx context.Context, cb *models.CallbackQuery) {
	userID := cb.From.ID

	if b.storage.IsPremium(userID) {
		text := fmt.Sprintf(
			"✅ <b>Premium активен!</b>\n\n"+
				"Твой лимит: <b>%d</b> кошельков",
			b.cfg.PremiumMaxWalletsPerUser,
		)
		b.editMessage(ctx, cb.Message, text, StartMenuKeyboard())
		return
	}

	text := "🔍 <b>Проверяем платёж...</b>\n\n" +
		"Если ты только что отправил средства, подожди 10-30 секунд и нажми кнопку снова."

	b.editMessage(ctx, cb.Message, text, CheckPaymentKeyboard())
}

// --- Helpers ---

func (b *Bot) getMaxWallets(userID int64) int {
	if b.cfg.VIPUserIDs[userID] {
		return b.cfg.VIPMaxWalletsPerUser
	}
	if b.storage.IsPremium(userID) {
		return b.cfg.PremiumMaxWalletsPerUser
	}
	return b.cfg.MaxWalletsPerUser
}

func (b *Bot) sendMessage(ctx context.Context, chatID int64, text string, keyboard *models.InlineKeyboardMarkup) {
	params := &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}
	if keyboard != nil {
		params.ReplyMarkup = keyboard
	}

	_, err := b.bot.SendMessage(ctx, params)
	if err != nil {
		b.log.Error("send message", "error", err)
	}
}

func (b *Bot) editMessage(ctx context.Context, msg models.MaybeInaccessibleMessage, text string, keyboard *models.InlineKeyboardMarkup) {
	if msg.Message == nil {
		return
	}

	params := &bot.EditMessageTextParams{
		ChatID:    msg.Message.Chat.ID,
		MessageID: msg.Message.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}
	if keyboard != nil {
		params.ReplyMarkup = keyboard
	}

	_, err := b.bot.EditMessageText(ctx, params)
	if err != nil {
		b.log.Error("edit message", "error", err)
	}
}

// SendNotification sends a notification message to a user
func (b *Bot) SendNotification(ctx context.Context, userID int64, text string, keyboard *models.InlineKeyboardMarkup) error {
	disablePreview := true
	params := &bot.SendMessageParams{
		ChatID:    userID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &disablePreview,
		},
	}
	if keyboard != nil {
		params.ReplyMarkup = keyboard
	}

	_, err := b.bot.SendMessage(ctx, params)
	return err
}

func extractAddress(text string) string {
	matches := addrRegex.FindStringSubmatch(text)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}
