package push

import (
	"testing"

	"github.com/digitalcheffe/nora/internal/config"
)

func TestEnsureVAPIDKeys_GeneratesWhenMissing(t *testing.T) {
	cfg := &config.Config{}

	if err := EnsureVAPIDKeys(cfg); err != nil {
		t.Fatalf("EnsureVAPIDKeys: %v", err)
	}

	if cfg.VAPIDPublic == "" {
		t.Error("expected VAPIDPublic to be set")
	}
	if cfg.VAPIDPrivate == "" {
		t.Error("expected VAPIDPrivate to be set")
	}
}

func TestEnsureVAPIDKeys_UsesExistingKeys(t *testing.T) {
	cfg := &config.Config{
		VAPIDPublic:  "existing-public",
		VAPIDPrivate: "existing-private",
	}

	if err := EnsureVAPIDKeys(cfg); err != nil {
		t.Fatalf("EnsureVAPIDKeys: %v", err)
	}

	if cfg.VAPIDPublic != "existing-public" {
		t.Errorf("expected VAPIDPublic to remain unchanged, got %q", cfg.VAPIDPublic)
	}
	if cfg.VAPIDPrivate != "existing-private" {
		t.Errorf("expected VAPIDPrivate to remain unchanged, got %q", cfg.VAPIDPrivate)
	}
}

func TestEnsureVAPIDKeys_GeneratesUniqueKeyPairs(t *testing.T) {
	cfg1 := &config.Config{}
	cfg2 := &config.Config{}

	if err := EnsureVAPIDKeys(cfg1); err != nil {
		t.Fatalf("first EnsureVAPIDKeys: %v", err)
	}
	if err := EnsureVAPIDKeys(cfg2); err != nil {
		t.Fatalf("second EnsureVAPIDKeys: %v", err)
	}

	if cfg1.VAPIDPublic == cfg2.VAPIDPublic {
		t.Error("expected two generated key pairs to differ")
	}
}
