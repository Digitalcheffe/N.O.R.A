package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// RuleRepo provides CRUD for alert_rules and audit logging for rule_executions.
type RuleRepo interface {
	List(ctx context.Context) ([]models.Rule, error)
	ListEnabled(ctx context.Context) ([]models.Rule, error)
	Get(ctx context.Context, id string) (models.Rule, error)
	Create(ctx context.Context, r models.Rule) (models.Rule, error)
	Update(ctx context.Context, r models.Rule) (models.Rule, error)
	Delete(ctx context.Context, id string) error
	LogExecution(ctx context.Context, exec models.RuleExecution) error
	DeleteExecutionsBefore(ctx context.Context, before time.Time) (int64, error)
}

type sqliteRuleRepo struct {
	db *sqlx.DB
}

// NewRuleRepo returns a RuleRepo backed by the given SQLite database.
func NewRuleRepo(db *sqlx.DB) RuleRepo {
	return &sqliteRuleRepo{db: db}
}

const ruleSelectCols = `
	id, name, enabled, source_id, source_type, severity,
	conditions, condition_logic,
	delivery_email, delivery_push, delivery_webhook, webhook_url,
	notif_title, notif_body, created_at, updated_at`

func (r *sqliteRuleRepo) List(ctx context.Context) ([]models.Rule, error) {
	var rules []models.Rule
	err := r.db.SelectContext(ctx, &rules,
		`SELECT`+ruleSelectCols+` FROM alert_rules ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	if rules == nil {
		rules = []models.Rule{}
	}
	return rules, nil
}

func (r *sqliteRuleRepo) ListEnabled(ctx context.Context) ([]models.Rule, error) {
	var rules []models.Rule
	err := r.db.SelectContext(ctx, &rules,
		`SELECT`+ruleSelectCols+` FROM alert_rules WHERE enabled = 1 ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list enabled rules: %w", err)
	}
	if rules == nil {
		rules = []models.Rule{}
	}
	return rules, nil
}

func (r *sqliteRuleRepo) Get(ctx context.Context, id string) (models.Rule, error) {
	var rule models.Rule
	err := r.db.GetContext(ctx, &rule,
		`SELECT`+ruleSelectCols+` FROM alert_rules WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return rule, ErrNotFound
	}
	if err != nil {
		return rule, fmt.Errorf("get rule: %w", err)
	}
	return rule, nil
}

func (r *sqliteRuleRepo) Create(ctx context.Context, rule models.Rule) (models.Rule, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO alert_rules (
			id, name, enabled, source_id, source_type, severity,
			conditions, condition_logic,
			delivery_email, delivery_push, delivery_webhook, webhook_url,
			notif_title, notif_body, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.Name, rule.Enabled,
		rule.SourceID, rule.SourceType, rule.Severity,
		rule.Conditions, rule.ConditionLogic,
		rule.DeliveryEmail, rule.DeliveryPush, rule.DeliveryWebhook, rule.WebhookURL,
		rule.NotifTitle, rule.NotifBody,
		rule.CreatedAt.UTC().Format(time.RFC3339),
		rule.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return rule, fmt.Errorf("create rule: %w", err)
	}
	return r.Get(ctx, rule.ID)
}

func (r *sqliteRuleRepo) Update(ctx context.Context, rule models.Rule) (models.Rule, error) {
	_, err := r.db.ExecContext(ctx, `
		UPDATE alert_rules SET
			name=?, enabled=?, source_id=?, source_type=?, severity=?,
			conditions=?, condition_logic=?,
			delivery_email=?, delivery_push=?, delivery_webhook=?, webhook_url=?,
			notif_title=?, notif_body=?, updated_at=?
		WHERE id=?`,
		rule.Name, rule.Enabled,
		rule.SourceID, rule.SourceType, rule.Severity,
		rule.Conditions, rule.ConditionLogic,
		rule.DeliveryEmail, rule.DeliveryPush, rule.DeliveryWebhook, rule.WebhookURL,
		rule.NotifTitle, rule.NotifBody,
		rule.UpdatedAt.UTC().Format(time.RFC3339),
		rule.ID,
	)
	if err != nil {
		return rule, fmt.Errorf("update rule: %w", err)
	}
	return r.Get(ctx, rule.ID)
}

func (r *sqliteRuleRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	return nil
}

func (r *sqliteRuleRepo) LogExecution(ctx context.Context, exec models.RuleExecution) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO rule_executions (id, rule_id, event_id, fired_at, delivery, success, error)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		exec.ID, exec.RuleID, exec.EventID,
		exec.FiredAt.UTC().Format(time.RFC3339),
		exec.Delivery, exec.Success, exec.Error,
	)
	if err != nil {
		return fmt.Errorf("log rule execution: %w", err)
	}
	return nil
}

func (r *sqliteRuleRepo) DeleteExecutionsBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM rule_executions WHERE datetime(fired_at) < datetime(?)`,
		before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("delete rule executions: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
