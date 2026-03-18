package config

import (
	"fmt"
	"os"
	"strings"
)

// ServerConfig holds typed, validated configuration for the local server.
type ServerConfig struct {
	// Server
	Addr string // API_ADDR, default ":3000"

	// LLM API Keys
	OpenAIKey    string // OPENAI_API_KEY
	OpenAIModel  string // OPENAI_MODEL
	AnthropicKey string // ANTHROPIC_API_KEY
	AnthropicModel string // ANTHROPIC_MODEL
	GeminiKey    string // GEMINI_API_KEY
	GeminiModel  string // GEMINI_MODEL

	// Feature flags (derived)
	HasOpenAI    bool
	HasAnthropic bool
	HasGemini    bool
}

// LoadServerConfig reads environment variables into a typed config struct.
func LoadServerConfig() ServerConfig {
	cfg := ServerConfig{
		Addr:           envOr("API_ADDR", ":3000"),
		OpenAIKey:      os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:    os.Getenv("OPENAI_MODEL"),
		AnthropicKey:   os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicModel: os.Getenv("ANTHROPIC_MODEL"),
		GeminiKey:      os.Getenv("GEMINI_API_KEY"),
		GeminiModel:    os.Getenv("GEMINI_MODEL"),
	}
	cfg.HasOpenAI = cfg.OpenAIKey != ""
	cfg.HasAnthropic = cfg.AnthropicKey != ""
	cfg.HasGemini = cfg.GeminiKey != ""
	return cfg
}

// Validate checks for configuration problems and returns warnings.
func (c ServerConfig) Validate() []string {
	var warnings []string

	if c.Addr == "" {
		warnings = append(warnings, "API_ADDR is empty, defaulting to :3000")
	}
	if !c.HasOpenAI && !c.HasAnthropic && !c.HasGemini {
		warnings = append(warnings, "No LLM API keys configured — all providers will use stubs")
	}
	if !c.HasAnthropic {
		warnings = append(warnings, "ANTHROPIC_API_KEY not set — planning and review analysis will use stub")
	}
	if !c.HasGemini {
		warnings = append(warnings, "GEMINI_API_KEY not set — summarization and frontier generation will use stub")
	}
	return warnings
}

// Summary returns a multi-line config summary for logging.
func (c ServerConfig) Summary() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  Addr:      %s\n", c.Addr))
	sb.WriteString(fmt.Sprintf("  OpenAI:    %s\n", providerStatus(c.HasOpenAI, c.OpenAIModel)))
	sb.WriteString(fmt.Sprintf("  Anthropic: %s\n", providerStatus(c.HasAnthropic, c.AnthropicModel)))
	sb.WriteString(fmt.Sprintf("  Gemini:    %s\n", providerStatus(c.HasGemini, c.GeminiModel)))
	return sb.String()
}

func providerStatus(enabled bool, model string) string {
	if !enabled {
		return "stub (no API key)"
	}
	if model != "" {
		return "enabled (" + model + ")"
	}
	return "enabled (default model)"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
