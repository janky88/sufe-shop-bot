package bot

import (
	"fmt"
	"strings"
	
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
	"shop-bot/internal/bot/messages"
)

func (b *Bot) handleBalanceHistory(callback *tgbotapi.CallbackQuery) {
	// Get user
	user, err := store.GetOrCreateUser(b.db, callback.From.ID, callback.From.UserName)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		return
	}
	
	lang := messages.GetUserLanguage(user.Language, callback.From.LanguageCode)
	
	// Get balance transactions
	transactions, err := store.GetBalanceTransactions(b.db, user.ID, 10, 0)
	if err != nil {
		logger.Error("Failed to get balance transactions", "error", err)
		b.sendError(callback.Message.Chat.ID, b.msg.Get(lang, "failed_to_load_history"))
		return
	}
	
	// Build history message
	var historyMsg strings.Builder
	historyMsg.WriteString(b.msg.Get(lang, "balance_history_title"))
	historyMsg.WriteString("\n\n")
	
	if len(transactions) == 0 {
		historyMsg.WriteString(b.msg.Get(lang, "no_balance_history"))
	} else {
		for _, tx := range transactions {
			// Format transaction type
			txType := tx.Type
			if txType == "recharge" {
				txType = b.msg.Get(lang, "tx_type_recharge")
			} else if txType == "purchase" {
				txType = b.msg.Get(lang, "tx_type_purchase")
			}
			
			// Format amount with + or -
			amountStr := fmt.Sprintf("%.2f", float64(tx.AmountCents)/100)
			if tx.AmountCents > 0 {
				amountStr = "+" + amountStr
			}
			
			// Add transaction line
			historyMsg.WriteString(fmt.Sprintf(
				"%s | %s | $%s | Balance: $%.2f | %s\n",
				tx.CreatedAt.Format("01/02 15:04"),
				txType,
				amountStr,
				float64(tx.BalanceAfter)/100,
				tx.Description,
			))
		}
	}
	
	// Get current balance
	balance, _ := store.GetUserBalance(b.db, user.ID)
	historyMsg.WriteString(fmt.Sprintf("\n%s: $%.2f", 
		b.msg.Get(lang, "current_balance"),
		float64(balance)/100,
	))
	
	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, historyMsg.String())
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
}