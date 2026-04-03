package jobs

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// SendMail sends a plain-text + HTML email via STARTTLS SMTP.
// It logs warnings rather than crashing when the server is unreachable.
// On SMTP error the error is returned; callers should log and skip.
func SendMail(host string, port int, user, pass, from string, to []string, subject, htmlBody string) error {
	if host == "" {
		return fmt.Errorf("smtp: host is not configured")
	}
	if len(to) == 0 {
		return fmt.Errorf("smtp: no recipients")
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	// Build the RFC 2822 message with MIME multipart.
	msg := buildMIMEMessage(from, to, subject, htmlBody)

	// Dial without TLS first — STARTTLS is negotiated after EHLO.
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp: dial %s: %w", addr, err)
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp: new client: %w", err)
	}
	defer c.Close()

	// Upgrade to TLS via STARTTLS if the server supports it.
	if ok, _ := c.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{ServerName: host}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("smtp: starttls: %w", err)
		}
	}

	// Authenticate if credentials are provided.
	if user != "" && pass != "" {
		auth := smtp.PlainAuth("", user, pass, host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp: auth: %w", err)
		}
	}

	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp: MAIL FROM: %w", err)
	}
	for _, r := range to {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("smtp: RCPT TO %s: %w", r, err)
		}
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp: DATA: %w", err)
	}
	if _, err := fmt.Fprint(w, msg); err != nil {
		return fmt.Errorf("smtp: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: close data: %w", err)
	}

	return c.Quit()
}

// sanitizeHeader strips CR and LF characters from a header value to prevent
// email header injection attacks.
func sanitizeHeader(v string) string {
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	return v
}

// buildMIMEMessage constructs an RFC 2822 MIME message with a text/html part.
// All user-supplied header values are sanitized to strip CRLF sequences before
// being written into the message, preventing header injection attacks.
func buildMIMEMessage(from string, to []string, subject, htmlBody string) string {
	sanitizedTo := make([]string, len(to))
	for i, addr := range to {
		sanitizedTo[i] = sanitizeHeader(addr)
	}

	var b strings.Builder
	b.WriteString("From: " + sanitizeHeader(from) + "\r\n")
	b.WriteString("To: " + strings.Join(sanitizedTo, ", ") + "\r\n")
	b.WriteString("Subject: " + sanitizeHeader(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("\r\n")
	// Strip bare CR characters from the body to prevent CRLF injection that
	// could insert spurious MIME headers into the message stream.
	b.WriteString(strings.ReplaceAll(htmlBody, "\r", ""))
	return b.String()
}
