package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsAPIKeyFromDotEnv(t *testing.T) {
	dir := t.TempDir()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousWD)
	})
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("API_KEY=api-key-from-dotenv\nAI_RELAY_MODEL=model-from-dotenv\n"), 0o600); err != nil {
		t.Fatalf("write dotenv: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("API_KEY", "")
	t.Setenv("AI_RELAY_MODEL", "")

	cfg := Load()
	if cfg.AIAPIKey != "api-key-from-dotenv" {
		t.Fatalf("AIAPIKey = %q", cfg.AIAPIKey)
	}
	if cfg.AIRelayModel != "model-from-dotenv" {
		t.Fatalf("AIRelayModel = %q", cfg.AIRelayModel)
	}
}

func TestLoadPrefersEnvironmentAPIKeyOverDotEnv(t *testing.T) {
	dir := t.TempDir()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousWD)
	})
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("API_KEY=api-key-from-dotenv\n"), 0o600); err != nil {
		t.Fatalf("write dotenv: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("API_KEY", "api-key-from-env")

	cfg := Load()
	if cfg.AIAPIKey != "api-key-from-env" {
		t.Fatalf("AIAPIKey = %q", cfg.AIAPIKey)
	}
}
