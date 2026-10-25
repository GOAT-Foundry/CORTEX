package i18n

import (
	"strings"
	"sync"
)

// Supported locales.
const (
	LocaleEN = "en"
	LocaleJA = "ja"
)

// Manager tracks the active locale and generates language instructions for AI agents.
type Manager struct {
	mu     sync.RWMutex
	locale string
}

// NewManager creates a locale manager initialized with the given default locale.
func NewManager(initial string) *Manager {
	return &Manager{locale: Normalize(initial)}
}

// Normalize returns a valid supported locale code ("en" or "ja").
func Normalize(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	if strings.HasPrefix(c, "ja") {
		return LocaleJA
	}
	return LocaleEN
}

// Get returns the current active locale.
func (m *Manager) Get() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.locale
}

// Set updates the active locale and returns the normalized value.
func (m *Manager) Set(code string) string {
	norm := Normalize(code)
	m.mu.Lock()
	m.locale = norm
	m.mu.Unlock()
	return norm
}

// AgentLanguageDirective returns the system prompt segment instructing the AI
// agent to think and respond in the user's selected language.
func (m *Manager) AgentLanguageDirective(override string) string {
	loc := m.Get()
	if strings.TrimSpace(override) != "" {
		loc = Normalize(override)
	}
	if loc == LocaleJA {
		return "Language Directive: The user's active locale is Japanese (ja). Always respond, explain, and generate reports or artifacts in natural Japanese unless the user explicitly asks for another language."
	}
	return "Language Directive: The user's active locale is English (en). Always respond, explain, and generate reports or artifacts in clear English unless the user explicitly asks for another language."
}
