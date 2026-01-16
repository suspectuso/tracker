package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// Telegram
	BotToken    string
	BotUsername string

	// TonAPI
	TonAPIKey     string
	TonAPIBaseURL string

	// Webhook
	WebhookEndpoint string
	WebhookPort     int

	// Database
	DBPath string

	// Limits
	MaxWalletsPerUser        int
	PremiumMaxWalletsPerUser int
	VIPUserIDs               map[int64]bool
	VIPMaxWalletsPerUser     int

	// Premium
	PremiumPriceTON   float64
	ServiceWalletAddr string

	// Filters
	MinTransferTON float64
}

func Load() *Config {
	cfg := &Config{
		// Telegram
		BotToken:    getEnv("BOT_TOKEN", ""),
		BotUsername: getEnv("BOT_USERNAME", "ton_tracker_bot"),

		// TonAPI
		TonAPIKey:     getEnv("TONAPI_API_KEY", ""),
		TonAPIBaseURL: strings.TrimSuffix(getEnv("TONAPI_BASE_URL", "https://tonapi.io/v2"), "/"),

		// Webhook
		WebhookEndpoint: getEnv("WEBHOOK_ENDPOINT", ""),
		WebhookPort:     getEnvInt("WEBHOOK_PORT", 8080),

		// Database
		DBPath: getEnv("DB_PATH", "./tracker.db"),

		// Limits
		MaxWalletsPerUser:        getEnvInt("MAX_WALLETS_PER_USER", 3),
		PremiumMaxWalletsPerUser: getEnvInt("PREMIUM_MAX_WALLETS_PER_USER", 100),
		VIPMaxWalletsPerUser:     getEnvInt("VIP_MAX_WALLETS_PER_USER", 10),

		// Premium
		PremiumPriceTON:   getEnvFloat("PREMIUM_PRICE_TON", 5.0),
		ServiceWalletAddr: getEnv("SERVICE_WALLET_ADDR", ""),

		// Filters
		MinTransferTON: getEnvFloat("MIN_TRANSFER_TON", 0),
	}

	// Parse VIP user IDs
	cfg.VIPUserIDs = make(map[int64]bool)
	vipIDs := getEnv("VIP_USER_IDS", "")
	for _, idStr := range strings.Split(vipIDs, ",") {
		idStr = strings.TrimSpace(idStr)
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			cfg.VIPUserIDs[id] = true
		}
	}

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}
