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

// Normalize returns a valid supported locale code ("en" or "ja"),
// falling back to LocaleEN when unknown.
func Normalize(code string) string {
	if norm, ok := ParseStrict(code); ok {
		return norm
	}
	return LocaleEN
}

// ParseStrict validates whether code represents a supported locale ("en" or "ja",
// including regional tags such as "en-US" or "ja-JP").
func ParseStrict(code string) (string, bool) {
	c := strings.ToLower(strings.TrimSpace(code))
	switch {
	case c == "ja" || strings.HasPrefix(c, "ja-") || strings.HasPrefix(c, "ja_"):
		return LocaleJA, true
	case c == "en" || strings.HasPrefix(c, "en-") || strings.HasPrefix(c, "en_"):
		return LocaleEN, true
	default:
		return "", false
	}
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

// ResolveLocale determines the effective locale for a request by checking:
// 1. Explicit request parameter (if valid)
// 2. X-Cortex-Locale header (if valid)
// 3. Accept-Language header primary tag (if valid)
// 4. The manager's active persisted locale
func (m *Manager) ResolveLocale(explicit, headerLocale, acceptLanguage string) string {
	if norm, ok := ParseStrict(explicit); ok {
		return norm
	}
	if norm, ok := ParseStrict(headerLocale); ok {
		return norm
	}
	if acceptLanguage != "" {
		first := strings.Split(acceptLanguage, ",")[0]
		first = strings.Split(first, ";")[0]
		if norm, ok := ParseStrict(first); ok {
			return norm
		}
	}
	return m.Get()
}

// AgentLanguageDirective returns the system prompt segment instructing the AI
// agent to think and respond in the user's selected language.
func (m *Manager) AgentLanguageDirective(override string) string {
	loc := m.ResolveLocale(override, "", "")
	if loc == LocaleJA {
		return "Language Directive: The user's active locale is Japanese (ja). Always respond, explain, and generate reports or artifacts in natural Japanese unless the user explicitly asks for another language."
	}
	return "Language Directive: The user's active locale is English (en). Always respond, explain, and generate reports or artifacts in clear English unless the user explicitly asks for another language."
}

// SystemReminder returns the per-turn reminder string injected via Norma's
// SystemReminderFunc so multi-turn sessions dynamically track UI locale switches.
func (m *Manager) SystemReminder(override string) string {
	loc := m.ResolveLocale(override, "", "")
	if loc == LocaleJA {
		return "# activeLocale\nCurrent UI & output language: ja (Japanese / 日本語). Respond in Japanese unless explicitly asked otherwise."
	}
	return "# activeLocale\nCurrent UI & output language: en (English). Respond in English unless explicitly asked otherwise."
}
