// Package rules implements the notification rules engine.
// The engine evaluates every incoming event against enabled rules and dispatches
// outbound notifications (email, web push, webhook) when conditions match.
package rules

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/push"
	"github.com/digitalcheffe/nora/internal/repo"
)

const cacheTTL = 30 * time.Second

// Engine evaluates incoming events against enabled rules and dispatches notifications.
type Engine struct {
	store       *repo.Store
	pushSender  *push.Sender
	cfg         *config.Config
	mu          sync.Mutex
	cachedRules []models.Rule
	cacheExpiry time.Time
}

// NewEngine creates a RulesEngine.
func NewEngine(store *repo.Store, pushSender *push.Sender, cfg *config.Config) *Engine {
	return &Engine{
		store:      store,
		pushSender: pushSender,
		cfg:        cfg,
	}
}

// Evaluate checks an event against all enabled rules and fires matching deliveries.
// Intended to run as a goroutine — never blocks the caller.
func (e *Engine) Evaluate(ctx context.Context, event models.Event) {
	rules, err := e.enabledRules(ctx)
	if err != nil {
		log.Printf("rules: load enabled rules: %v", err)
		return
	}

	for _, rule := range rules {
		if !e.passesGate(rule, event) {
			continue
		}

		conditions, err := parseConditions(rule.Conditions)
		if err != nil {
			log.Printf("rules: parse conditions for rule %s: %v", rule.ID, err)
			continue
		}

		if !evaluateConditions(conditions, rule.ConditionLogic, event) {
			continue
		}

		// Rule matched — dispatch deliveries concurrently.
		e.dispatch(ctx, rule, event)
	}
}

// InvalidateCache forces the next Evaluate call to reload rules from the DB.
func (e *Engine) InvalidateCache() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cacheExpiry = time.Time{}
}

// enabledRules returns cached enabled rules, refreshing if the cache is stale.
func (e *Engine) enabledRules(ctx context.Context) ([]models.Rule, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if time.Now().Before(e.cacheExpiry) {
		return e.cachedRules, nil
	}

	rules, err := e.store.Rules.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	e.cachedRules = rules
	e.cacheExpiry = time.Now().Add(cacheTTL)
	return rules, nil
}

// passesGate checks source_id, source_type, and severity gates.
func (e *Engine) passesGate(rule models.Rule, event models.Event) bool {
	if rule.SourceID != nil && *rule.SourceID != "" {
		if event.SourceID != *rule.SourceID {
			return false
		}
	}
	if rule.SourceType != nil && *rule.SourceType != "" {
		if !matchesSourceType(*rule.SourceType, event.SourceType) {
			return false
		}
	}
	if rule.Severity != nil && *rule.Severity != "" {
		if event.Level != *rule.Severity {
			return false
		}
	}
	return true
}

// matchesSourceType maps rule source_type abbreviations to event source_type values.
func matchesSourceType(ruleType, eventType string) bool {
	switch ruleType {
	case "app":
		return eventType == "app"
	case "docker":
		return eventType == "docker_engine"
	case "monitor":
		return eventType == "monitor_check"
	default:
		return ruleType == eventType
	}
}

// parseConditions deserializes the JSON conditions array from a rule.
func parseConditions(raw string) ([]models.RuleCondition, error) {
	if raw == "" || raw == "[]" {
		return nil, nil
	}
	var conditions []models.RuleCondition
	if err := json.Unmarshal([]byte(raw), &conditions); err != nil {
		return nil, err
	}
	return conditions, nil
}
