package push

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/digitalcheffe/nora/internal/config"
)

type vapidFile struct {
	Public  string `json:"public"`
	Private string `json:"private"`
}

// EnsureVAPIDKeys ensures the config has a valid VAPID key pair using this priority:
//  1. NORA_VAPID_PUBLIC / NORA_VAPID_PRIVATE env vars (already loaded into cfg)
//  2. vapid.json in the data directory (persisted from a previous run)
//  3. Generate a new pair and write it to vapid.json for future restarts
func EnsureVAPIDKeys(cfg *config.Config) error {
	// 1. Env vars win — nothing else to do.
	if cfg.VAPIDPublic != "" && cfg.VAPIDPrivate != "" {
		log.Printf("push: using VAPID keys from environment")
		return nil
	}

	dataDir := filepath.Dir(cfg.DBPath)
	keyFile := filepath.Join(dataDir, "vapid_keys", "vapid.json")

	// 2. Load from file if it exists.
	if data, err := os.ReadFile(keyFile); err == nil {
		var kf vapidFile
		if err := json.Unmarshal(data, &kf); err == nil && kf.Public != "" && kf.Private != "" {
			cfg.VAPIDPublic = kf.Public
			cfg.VAPIDPrivate = kf.Private
			log.Printf("push: loaded VAPID keys from %s", keyFile)
			return nil
		}
		log.Printf("push: vapid.json exists but is malformed — regenerating")
	}

	// 3. Generate and persist.
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return fmt.Errorf("generate VAPID keys: %w", err)
	}

	cfg.VAPIDPublic = publicKey
	cfg.VAPIDPrivate = privateKey

	if err := os.MkdirAll(filepath.Dir(keyFile), 0700); err != nil {
		log.Printf("push: warning — could not create vapid_keys directory: %v", err)
	}
	kf := vapidFile{Public: publicKey, Private: privateKey}
	data, _ := json.MarshalIndent(kf, "", "  ")
	if err := os.WriteFile(keyFile, data, 0600); err != nil {
		log.Printf("push: warning — could not write vapid.json to %s: %v", keyFile, err)
		log.Printf("push: push notifications will work this session but subscriptions will break on restart")
	} else {
		log.Printf("push: generated new VAPID key pair and saved to %s", keyFile)
	}

	return nil
}
