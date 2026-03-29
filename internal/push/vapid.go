package push

import (
	"fmt"
	"log"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/digitalcheffe/nora/internal/config"
)

// EnsureVAPIDKeys ensures the config has a valid VAPID key pair.
// If NORA_VAPID_PUBLIC and NORA_VAPID_PRIVATE are already set they are used as-is.
// Otherwise a new pair is generated and logged so the operator can persist them.
func EnsureVAPIDKeys(cfg *config.Config) error {
	if cfg.VAPIDPublic != "" && cfg.VAPIDPrivate != "" {
		log.Printf("push: using configured VAPID public key: %s", cfg.VAPIDPublic)
		return nil
	}

	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return fmt.Errorf("generate VAPID keys: %w", err)
	}

	cfg.VAPIDPublic = publicKey
	cfg.VAPIDPrivate = privateKey

	log.Printf("push: generated new VAPID key pair (add these to your .env to persist them)")
	log.Printf("push: NORA_VAPID_PUBLIC=%s", publicKey)
	log.Printf("push: NORA_VAPID_PRIVATE=%s", privateKey)

	return nil
}
