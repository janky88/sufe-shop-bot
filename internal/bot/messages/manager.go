package messages

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"sync"
)

//go:embed *.json
var messagesFS embed.FS

type Manager struct {
	messages map[string]map[string]string
	mu       sync.RWMutex
}

var (
	manager     *Manager
	managerOnce sync.Once
)

// GetManager returns the singleton message manager
func GetManager() *Manager {
	managerOnce.Do(func() {
		manager = &Manager{
			messages: make(map[string]map[string]string),
		}
		manager.loadMessages()
	})
	return manager
}

func (m *Manager) loadMessages() {
	languages := []string{"en", "zh"}
	
	for _, lang := range languages {
		data, err := messagesFS.ReadFile(lang + ".json")
		if err != nil {
			fmt.Printf("Failed to load %s.json: %v\n", lang, err)
			continue
		}
		
		var msgs map[string]string
		if err := json.Unmarshal(data, &msgs); err != nil {
			fmt.Printf("Failed to parse %s.json: %v\n", lang, err)
			continue
		}
		
		m.messages[lang] = msgs
	}
}

// Get returns a message for the given key and language
func (m *Manager) Get(lang, key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Fallback to English if language not found
	if _, ok := m.messages[lang]; !ok {
		lang = "en"
	}
	
	if msg, ok := m.messages[lang][key]; ok {
		return msg
	}
	
	// Try English as fallback
	if lang != "en" {
		if msg, ok := m.messages["en"][key]; ok {
			return msg
		}
	}
	
	return key // Return key if message not found
}

// Format returns a formatted message with template data
func (m *Manager) Format(lang, key string, data interface{}) string {
	msgTemplate := m.Get(lang, key)
	
	// If no template syntax, return as-is
	if !strings.Contains(msgTemplate, "{{") {
		return msgTemplate
	}
	
	// Parse and execute template
	tmpl, err := template.New(key).Parse(msgTemplate)
	if err != nil {
		return msgTemplate
	}
	
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return msgTemplate
	}
	
	return buf.String()
}

// GetUserLanguage determines the user's language preference
func GetUserLanguage(userLang string, telegramLang string) string {
	// Priority: stored user language > telegram language > default
	if userLang != "" {
		return userLang
	}
	
	// Map Telegram language codes to our supported languages
	if strings.HasPrefix(telegramLang, "zh") {
		return "zh"
	}
	
	// Default to English
	return "en"
}

// GetAvailableLanguages returns list of available languages
func (m *Manager) GetAvailableLanguages() []Language {
	return []Language{
		{Code: "en", Name: "English", Flag: "🇬🇧"},
		{Code: "zh", Name: "中文", Flag: "🇨🇳"},
	}
}

type Language struct {
	Code string
	Name string
	Flag string
}