package jobs

import (
	"strings"
	"testing"
)

func TestSanitizeHeader_StripsCRLF(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"normal value", "normal value"},
		{"injected\r\nBCC: attacker@evil.com", "injectedBCC: attacker@evil.com"},
		{"injected\nBCC: attacker@evil.com", "injectedBCC: attacker@evil.com"},
		{"injected\rBCC: attacker@evil.com", "injectedBCC: attacker@evil.com"},
		{"\r\n", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := sanitizeHeader(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeHeader(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestBuildMIMEMessage_HeaderInjectionPrevented(t *testing.T) {
	maliciousSubject := "Hello\r\nBCC: attacker@evil.com"
	maliciousFrom := "legit@example.com\r\nX-Injected: header"
	maliciousTo := []string{"victim@example.com\r\nBCC: third@evil.com"}

	msg := buildMIMEMessage(maliciousFrom, maliciousTo, maliciousSubject, "<p>body</p>")

	// Split into header and body sections.
	parts := strings.SplitN(msg, "\r\n\r\n", 2)
	if len(parts) < 2 {
		t.Fatal("message has no header/body separator")
	}
	headers := parts[0]

	// No bare CR or LF should appear in the headers (CRLF pairs are fine as
	// they are the RFC 2822 line terminator that we wrote explicitly).
	for _, line := range strings.Split(headers, "\r\n") {
		if strings.ContainsAny(line, "\r\n") {
			t.Errorf("header line contains bare CR or LF: %q", line)
		}
	}

	// Injected field names must not appear as standalone header lines.
	// After CRLF stripping the content is still present but cannot start a
	// new header line, so we check that no line *begins* with the field name.
	for _, line := range strings.Split(headers, "\r\n") {
		if strings.HasPrefix(line, "BCC:") {
			t.Errorf("injected BCC appeared as a standalone header line: %q", line)
		}
		if strings.HasPrefix(line, "X-Injected:") {
			t.Errorf("injected X-Injected appeared as a standalone header line: %q", line)
		}
	}
}
