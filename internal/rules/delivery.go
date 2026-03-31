package rules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// dispatch fires all enabled delivery channels concurrently for a matched rule.
func (e *Engine) dispatch(ctx context.Context, rule models.Rule, event models.Event) {
	title := renderTemplate(rule.NotifTitle, event)
	body := renderTemplate(rule.NotifBody, event)

	if rule.DeliveryEmail {
		go e.deliverEmail(ctx, rule, event, title, body)
	}
	if rule.DeliveryPush {
		go e.deliverPush(ctx, rule, event, title, body)
	}
	if rule.DeliveryWebhook && rule.WebhookURL != nil && *rule.WebhookURL != "" {
		go e.deliverWebhook(ctx, rule, event)
	}
}

func (e *Engine) deliverEmail(ctx context.Context, rule models.Rule, event models.Event, subject, body string) {
	smtp, err := e.smtpSettings(ctx)
	if err != nil {
		log.Printf("rules: email delivery skipped for rule %s: smtp not configured: %v", rule.ID, err)
		e.logExecution(ctx, rule.ID, event.ID, "email", false, err.Error())
		return
	}

	var recipients []string
	if smtp.To != "" {
		recipients = []string{smtp.To}
	} else if smtp.From != "" {
		recipients = []string{smtp.From}
	}
	if len(recipients) == 0 {
		log.Printf("rules: email delivery skipped for rule %s: no recipients configured", rule.ID)
		return
	}

	htmlBody := "<pre style=\"font-family:monospace;white-space:pre-wrap\">" + escapeHTML(body) + "</pre>"
	if err := jobs.SendMail(smtp.Host, smtp.Port, smtp.User, smtp.Pass, smtp.From, recipients, subject, htmlBody); err != nil {
		log.Printf("rules: email delivery failed for rule %s: %v", rule.ID, err)
		e.logExecution(ctx, rule.ID, event.ID, "email", false, err.Error())
		return
	}
	log.Printf("rules: email delivered for rule %s event %s", rule.ID, event.ID)
	e.logExecution(ctx, rule.ID, event.ID, "email", true, "")
}

func (e *Engine) deliverPush(ctx context.Context, rule models.Rule, event models.Event, title, body string) {
	subs, err := e.store.WebPushSubscriptions.ListAll(ctx)
	if err != nil {
		log.Printf("rules: push delivery check failed for rule %s: %v", rule.ID, err)
		e.logExecution(ctx, rule.ID, event.ID, "push", false, err.Error())
		return
	}
	if len(subs) == 0 {
		log.Printf("rules: push delivery skipped for rule %s: no subscriptions", rule.ID)
		return
	}

	if err := e.pushSender.SendToAll(ctx, title, body); err != nil {
		log.Printf("rules: push delivery failed for rule %s: %v", rule.ID, err)
		e.logExecution(ctx, rule.ID, event.ID, "push", false, err.Error())
		return
	}
	log.Printf("rules: push delivered for rule %s event %s", rule.ID, event.ID)
	e.logExecution(ctx, rule.ID, event.ID, "push", true, "")
}

func (e *Engine) deliverWebhook(ctx context.Context, rule models.Rule, event models.Event) {
	webhookPayload := map[string]interface{}{
		"rule_id":      rule.ID,
		"rule_name":    rule.Name,
		"event_id":     event.ID,
		"source_name":  event.SourceName,
		"severity":     event.Level,
		"display_text": event.Title,
		"fired_at":     time.Now().UTC().Format(time.RFC3339),
	}

	if event.Payload != "" {
		var fields map[string]interface{}
		if err := json.Unmarshal([]byte(event.Payload), &fields); err == nil {
			webhookPayload["fields"] = fields
		} else {
			webhookPayload["fields"] = map[string]interface{}{}
		}
	} else {
		webhookPayload["fields"] = map[string]interface{}{}
	}

	data, err := json.Marshal(webhookPayload)
	if err != nil {
		log.Printf("rules: webhook marshal failed for rule %s: %v", rule.ID, err)
		e.logExecution(ctx, rule.ID, event.ID, "webhook", false, err.Error())
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, *rule.WebhookURL, bytes.NewReader(data))
	if err != nil {
		log.Printf("rules: webhook request build failed for rule %s: %v", rule.ID, err)
		e.logExecution(ctx, rule.ID, event.ID, "webhook", false, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("rules: webhook delivery failed for rule %s: %v", rule.ID, err)
		e.logExecution(ctx, rule.ID, event.ID, "webhook", false, err.Error())
		return
	}
	resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		log.Printf("rules: webhook non-2xx for rule %s: %s", rule.ID, errMsg)
		e.logExecution(ctx, rule.ID, event.ID, "webhook", false, errMsg)
		return
	}
	log.Printf("rules: webhook delivered for rule %s event %s", rule.ID, event.ID)
	e.logExecution(ctx, rule.ID, event.ID, "webhook", true, "")
}

// smtpSettings reads SMTP config from the settings table with env fallback.
func (e *Engine) smtpSettings(ctx context.Context) (*models.SMTPSettings, error) {
	var s models.SMTPSettings
	err := e.store.Settings.GetJSON(ctx, "smtp", &s)
	if errors.Is(err, repo.ErrNotFound) {
		if e.cfg.SMTPHost == "" {
			return nil, fmt.Errorf("smtp not configured")
		}
		return &models.SMTPSettings{
			Host: e.cfg.SMTPHost,
			Port: e.cfg.SMTPPort,
			User: e.cfg.SMTPUser,
			Pass: e.cfg.SMTPPass,
			From: e.cfg.SMTPFrom,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	if s.Host == "" {
		return nil, fmt.Errorf("smtp not configured")
	}
	return &s, nil
}

// logExecution records a delivery attempt to the rule_executions table.
func (e *Engine) logExecution(ctx context.Context, ruleID, eventID, delivery string, success bool, errMsg string) {
	exec := models.RuleExecution{
		ID:       uuid.NewString(),
		RuleID:   ruleID,
		EventID:  eventID,
		FiredAt:  time.Now().UTC(),
		Delivery: delivery,
		Success:  success,
	}
	if errMsg != "" {
		exec.Error = &errMsg
	}
	if err := e.store.Rules.LogExecution(ctx, exec); err != nil {
		log.Printf("rules: failed to log execution for rule %s: %v", ruleID, err)
	}
}

// escapeHTML does minimal HTML escaping for plain-text email bodies.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
