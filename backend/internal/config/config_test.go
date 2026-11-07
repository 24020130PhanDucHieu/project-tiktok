package config

import (
	"strings"
	"testing"
)

const strongSecret = "k3Q9vT2xLm8RzW5yBn7HdP4sFc6JgA1e" // 32 characters

func TestProductionRefusesTheDevelopmentSecrets(t *testing.T) {
	c := Config{Env: "production", JWTSecret: defaultJWTSecret, ZaloBridgeSecret: defaultBridgeSecret}
	err := c.Validate()
	if err == nil {
		t.Fatal("production must not start with the default secrets")
	}
	for _, name := range []string{"JWT_SECRET", "ZALO_BRIDGE_SECRET"} {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("the error should name %s, got %q", name, err)
		}
	}
}

func TestProductionRefusesAShortSecret(t *testing.T) {
	c := Config{Env: "production", JWTSecret: "short", ZaloBridgeSecret: strongSecret}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("a secret under 32 characters must be refused, got %v", err)
	}
}

func TestProductionRefusesThePlaceholdersOfTheExampleEnvFile(t *testing.T) {
	// .env.example ships these 33-character placeholders: long enough to pass a length check, still public
	c := Config{Env: "production", JWTSecret: "replace-with-a-long-random-secret", ZaloBridgeSecret: "replace-with-another-long-random-secret"}
	err := c.Validate()
	if err == nil {
		t.Fatal("an untouched .env.example must not start in production")
	}
	for _, name := range []string{"JWT_SECRET", "ZALO_BRIDGE_SECRET"} {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("the error should name %s, got %q", name, err)
		}
	}
}

func TestProductionAcceptsStrongSecrets(t *testing.T) {
	c := Config{Env: "production", JWTSecret: strongSecret, ZaloBridgeSecret: strongSecret + "x"}
	if err := c.Validate(); err != nil {
		t.Fatalf("strong secrets are fine: %v", err)
	}
}

func TestDevelopmentKeepsWorkingWithTheDefaults(t *testing.T) {
	c := Config{Env: "development", JWTSecret: defaultJWTSecret, ZaloBridgeSecret: defaultBridgeSecret}
	if err := c.Validate(); err != nil {
		t.Fatalf("development must start without any setup: %v", err)
	}
}
