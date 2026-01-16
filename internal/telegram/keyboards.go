package telegram

import (
	"fmt"

	"github.com/go-telegram/bot/models"
	"github.com/suspectuso/ton-tracker/internal/storage"
)

// MainKeyboard returns the main menu keyboard
func MainKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "➕ Добавить кошелёк", CallbackData: "add"},
				{Text: "📋 Список кошельков", CallbackData: "list"},
			},
			{
				{Text: "⭐ Premium", CallbackData: "premium"},
			},
		},
	}
}

// WalletsKeyboard returns a keyboard with wallet list
func WalletsKeyboard(wallets []storage.Wallet) *models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton

	for _, w := range wallets {
		url := fmt.Sprintf("https://tonviewer.com/%s", w.AddressDisplay)
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: w.Name, URL: url},
			{Text: "⚙️", CallbackData: fmt.Sprintf("cfg:%d", w.ID)},
			{Text: "🗑", CallbackData: fmt.Sprintf("del:%d", w.ID)},
		})
	}

	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "⬅️ Назад", CallbackData: "back"},
	})

	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// WalletSettingsKeyboard returns settings keyboard for a wallet
func WalletSettingsKeyboard(walletID int64) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "⬇️ Минимальная сумма", CallbackData: fmt.Sprintf("cfg_min:%d", walletID)},
			},
			{
				{Text: "♻️ Сбросить фильтры", CallbackData: fmt.Sprintf("cfg_reset:%d", walletID)},
			},
			{
				{Text: "⬅️ Назад", CallbackData: "list"},
			},
		},
	}
}

// BackKeyboard returns a simple back button
func BackKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "⬅️ Назад", CallbackData: "back"},
			},
		},
	}
}

// PremiumKeyboard returns premium payment options keyboard
func PremiumKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "💼 Оплатить", CallbackData: "pay_wallet"},
			},
			{
				{Text: "⬅️ Назад", CallbackData: "back"},
			},
		},
	}
}

// CheckPaymentKeyboard returns keyboard for checking payment
func CheckPaymentKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🔄 Проверить оплату", CallbackData: "check_payment"},
			},
			{
				{Text: "⬅️ Назад", CallbackData: "premium"},
			},
		},
	}
}

// StartMenuKeyboard returns keyboard to go back to start menu
func StartMenuKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "⬅️ Главное меню", CallbackData: "back"},
			},
		},
	}
}
