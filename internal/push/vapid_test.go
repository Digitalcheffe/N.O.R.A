package push

import (
	"path/filepath"
	"testing"

	"github.com/digitalcheffe/nora/internal/config"
)

func cfgWithTempDir(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{DBPath: filepath.Join(t.TempDir(), "nora.db")}
}

func TestEnsureVAPIDKeys_GeneratesWhenMissing(t *testing.T) {
	cfg := cfgWithTempDir(t)

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
	cfg := cfgWithTempDir(t)
	cfg.VAPIDPublic = "existing-public"
	cfg.VAPIDPrivate = "existing-private"

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
	cfg1 := cfgWithTempDir(t)
	cfg2 := cfgWithTempDir(t)

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

func TestEnsureVAPIDKeys_PersistsAndReloads(t *testing.T) {
	cfg1 := cfgWithTempDir(t)

	if err := EnsureVAPIDKeys(cfg1); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Second config pointing at the same data dir simulates a restart.
	cfg2 := &config.Config{DBPath: cfg1.DBPath}
	if err := EnsureVAPIDKeys(cfg2); err != nil {
		t.Fatalf("second run: %v", err)
	}

	if cfg1.VAPIDPublic != cfg2.VAPIDPublic {
		t.Error("expected keys to be the same after reload from file")
	}
	if cfg1.VAPIDPrivate != cfg2.VAPIDPrivate {
		t.Error("expected private key to be the same after reload from file")
	}
}
