package bot

import (
	"fmt"
	"strings"
	
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
	"shop-bot/internal/bot/messages"
)

// handleRechargeCard handles recharge card code input
func (b *Bot) handleRechargeCard(message *tgbotapi.Message) {
	user, _ := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)
	
	cardCode := strings.TrimSpace(message.Text)
	
	// Use the recharge card
	card, err := store.UseRechargeCard(b.db, user.ID, cardCode)
	if err != nil {
		var errorMsg string
		switch err {
		case store.ErrCardNotFound:
			errorMsg = b.msg.Get(lang, "card_not_found")
		case store.ErrCardAlreadyUsed:
			errorMsg = b.msg.Get(lang, "card_already_used")
		case store.ErrCardExpired:
			errorMsg = b.msg.Get(lang, "card_expired")
		default:
			errorMsg = b.msg.Get(lang, "card_error")
		}
		b.sendError(message.Chat.ID, errorMsg)
		return
	}
	
	// Get new balance
	newBalance, _ := store.GetUserBalance(b.db, user.ID)
	
	// Send success message
	successMsg := b.msg.Format(lang, "balance_recharged", map[string]interface{}{
		"Amount":     fmt.Sprintf("%.2f", float64(card.AmountCents)/100),
		"NewBalance": fmt.Sprintf("%.2f", float64(newBalance)/100),
		"CardCode":   cardCode,
	})
	
	msg := tgbotapi.NewMessage(message.Chat.ID, successMsg)
	b.api.Send(msg)
	
	logger.Info("Recharge card used", "user_id", user.ID, "card_code", cardCode, "amount", card.AmountCents)
}