package bot

import (
	"fmt"
	
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	logger "shop-bot/internal/log"
	"shop-bot/internal/store"
	"shop-bot/internal/bot/messages"
)

// Language selection handlers

func (b *Bot) handleLanguageSelection(message *tgbotapi.Message) {
	// Get current user language
	user, _ := store.GetOrCreateUser(b.db, message.From.ID, message.From.UserName)
	lang := messages.GetUserLanguage(user.Language, message.From.LanguageCode)
	
	// Create language selection keyboard
	languages := b.msg.GetAvailableLanguages()
	var rows [][]tgbotapi.InlineKeyboardButton
	
	for _, l := range languages {
		button := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%s %s", l.Flag, l.Name),
			fmt.Sprintf("set_lang:%s", l.Code),
		)
		rows = append(rows, []tgbotapi.InlineKeyboardButton{button})
	}
	
	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	
	msg := tgbotapi.NewMessage(message.Chat.ID, b.msg.Get(lang, "choose_language"))
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) handleSetLanguage(callback *tgbotapi.CallbackQuery, newLang string) {
	// Update user language
	user, err := store.GetOrCreateUser(b.db, callback.From.ID, callback.From.UserName)
	if err != nil {
		logger.Error("Failed to get user", "error", err)
		return
	}
	
	// Update language in database
	if err := b.db.Model(&store.User{}).Where("id = ?", user.ID).Update("language", newLang).Error; err != nil {
		logger.Error("Failed to update language", "error", err)
		return
	}
	
	// Update reply keyboard with new language
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(b.msg.Get(newLang, "btn_buy")),
			tgbotapi.NewKeyboardButton(b.msg.Get(newLang, "btn_deposit")),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(b.msg.Get(newLang, "btn_profile")),
			tgbotapi.NewKeyboardButton(b.msg.Get(newLang, "btn_faq")),
		),
	)
	
	// Send confirmation message
	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, b.msg.Get(newLang, "language_changed"))
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
	
	// Delete the language selection message
	deleteMsg := tgbotapi.NewDeleteMessage(callback.Message.Chat.ID, callback.Message.MessageID)
	b.api.Send(deleteMsg)
	
	logger.Info("User language updated", "user_id", user.ID, "new_lang", newLang)
}