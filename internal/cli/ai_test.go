package cli

import (
	"testing"

	"kalua/internal/ai"
	"kalua/internal/config"
)

func iniText(t *testing.T, text string) *config.File {
	t.Helper()
	return config.Parse([]byte(text))
}

func TestResolveAIEnvStyleKeys(t *testing.T) {
	cfg := iniText(t, `
[AI]
KALUA_AI_BASE_URL = https://openrouter.ai/api/v1
KALUA_AI_API_KEY  = sk-or-v1-abc
KALUA_AI_MODEL    = nvidia/nemotron
`)
	got := resolveAI(cfg, aiOptions{})
	if got.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("BaseURL = %q", got.BaseURL)
	}
	if got.APIKey != "sk-or-v1-abc" {
		t.Errorf("APIKey = %q", got.APIKey)
	}
	if got.Model != "nvidia/nemotron" {
		t.Errorf("Model = %q", got.Model)
	}
}

func TestResolveAIFlagStyleKeys(t *testing.T) {
	cfg := iniText(t, `
[AI]
base-url = https://openrouter.ai/api/v1
model    = nvidia/nemotron
api-key  = sk-or-v1-flag
`)
	got := resolveAI(cfg, aiOptions{})
	if got.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("BaseURL = %q", got.BaseURL)
	}
	if got.Model != "nvidia/nemotron" {
		t.Errorf("Model = %q", got.Model)
	}
	if got.APIKey != "sk-or-v1-flag" {
		t.Errorf("APIKey = %q", got.APIKey)
	}
}

func TestResolveAIIniBeatsEnv(t *testing.T) {
	t.Setenv("KALUA_AI_BASE_URL", "http://env:1234/v1")
	t.Setenv("KALUA_AI_MODEL", "env-model")
	cfg := iniText(t, `
[AI]
KALUA_AI_BASE_URL = http://ini:1234/v1
model = ini-model
`)
	got := resolveAI(cfg, aiOptions{})
	if got.BaseURL != "http://ini:1234/v1" {
		t.Errorf("BaseURL = %q, want INI value (INI > env)", got.BaseURL)
	}
	if got.Model != "ini-model" {
		t.Errorf("Model = %q, want INI value", got.Model)
	}
}

func TestResolveAIFlagsBeatIni(t *testing.T) {
	cfg := iniText(t, `
[AI]
base-url = http://ini:1234/v1
model    = ini-model
api-key  = sk-ini
`)
	got := resolveAI(cfg, aiOptions{BaseURL: "http://flag:1234/v1", Model: "flag-model", APIKey: "sk-flag"})
	if got.BaseURL != "http://flag:1234/v1" {
		t.Errorf("BaseURL = %q, want flag value", got.BaseURL)
	}
	if got.Model != "flag-model" {
		t.Errorf("Model = %q, want flag value", got.Model)
	}
	if got.APIKey != "sk-flag" {
		t.Errorf("APIKey = %q, want flag value", got.APIKey)
	}
}

func TestResolveAIOpenAIRFallback(t *testing.T) {
	// OpenRouter endpoint with no key in [AI]: fall back to OPENAI_API_KEY.
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	cfg := iniText(t, `
[AI]
base-url = https://openrouter.ai/api/v1
`)
	got := resolveAI(cfg, aiOptions{})
	if got.APIKey != "sk-openai" {
		t.Errorf("APIKey = %q, want OPENAI_API_KEY fallback", got.APIKey)
	}
}

func TestResolveAINoOpenAIRFallbackForLocal(t *testing.T) {
	// A local (non-OpenRouter) endpoint must stay key-less.
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	cfg := iniText(t, `
[AI]
base-url = http://localhost:1234/v1
`)
	got := resolveAI(cfg, aiOptions{})
	if got.APIKey != "" {
		t.Errorf("APIKey = %q, want empty for local endpoint", got.APIKey)
	}
}

func TestResolveAIApiKeyOnlyFromExplicitApiKeyEnv(t *testing.T) {
	t.Setenv("KALUA_AI_API_KEY", "")
	cfg := iniText(t, `
[AI]
base-url = http://ini:1234/v1
model    = ini-model
api-key  = sk-ini
`)
	got := resolveAI(cfg, aiOptions{})
	if got.APIKey != "sk-ini" {
		t.Errorf("APIKey = %q, want INI value (unset flag must not clobber)", got.APIKey)
	}
	// Explicit --api-key-env points at another var and wins.
	t.Setenv("KALUA_AI_API_KEY", "sk-env")
	t.Setenv("OTHER_KEY", "sk-other")
	got = resolveAI(cfg, aiOptions{APIKey: "sk-other"})
	if got.APIKey != "sk-other" {
		t.Errorf("APIKey = %q, want explicit --api-key-env value", got.APIKey)
	}
}

func TestResolveAIDefaultsWithoutIni(t *testing.T) {
	got := resolveAI(config.New(), aiOptions{})
	want := ai.EnvConfig()
	if got != want {
		t.Errorf("resolveAI(nil config) = %+v, want %+v", got, want)
	}
	if got.BaseURL != "http://localhost:1234/v1" || got.Model != "local-model" {
		t.Errorf("defaults: %+v", got)
	}
}
