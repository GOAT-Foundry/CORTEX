package config

import (
	"os"
	"strings"
)

// Config holds runtime settings for the CORTEX server.
type Config struct {
	Port            string
	DBPath          string
	SkillsDir       string
	WebDir          string
	DefaultLocale   string
	DefaultProvider string
	DefaultModel    string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	locale := strings.ToLower(strings.TrimSpace(envOr("CORTEX_LOCALE", "en")))
	if locale != "ja" && locale != "en" {
		locale = "en"
	}
	return Config{
		Port:            envOr("PORT", "8080"),
		DBPath:          envOr("CORTEX_DB_PATH", "./data/cortex.db"),
		SkillsDir:       envOr("CORTEX_SKILLS_DIR", "./skills"),
		WebDir:          envOr("CORTEX_WEB_DIR", "./web/out"),
		DefaultLocale:   locale,
		DefaultProvider: envOr("CORTEX_PROVIDER", "openai"),
		DefaultModel:    envOr("CORTEX_MODEL", "gpt-4o"),
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
