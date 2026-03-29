package infra

import "testing"

func TestParseHostFromRule(t *testing.T) {
	tests := []struct {
		name  string
		rule  string
		want  string // "" means expect nil
		isNil bool
	}{
		{
			name: "backtick syntax",
			rule: "Host(`sonarr.example.com`)",
			want: "sonarr.example.com",
		},
		{
			name: "double-quote syntax",
			rule: `Host("sonarr.example.com")`,
			want: "sonarr.example.com",
		},
		{
			name:  "HostRegexp skipped",
			rule:  "HostRegexp(`[a-z]+\\.example\\.com`)",
			isNil: true,
		},
		{
			name:  "PathPrefix only — no host",
			rule:  "PathPrefix(`/api`)",
			isNil: true,
		},
		{
			name:  "empty rule",
			rule:  "",
			isNil: true,
		},
		{
			name: "Host with trailing PathPrefix — first match wins",
			rule: "Host(`foo.bar.com`) && PathPrefix(`/api`)",
			want: "foo.bar.com",
		},
		{
			name: "subdomain with multiple labels",
			rule: "Host(`app.internal.corp.example.com`)",
			want: "app.internal.corp.example.com",
		},
		{
			name: "double-quote with ampersand rule",
			rule: `Host("grafana.home") && PathPrefix("/")`,
			want: "grafana.home",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseHostFromRule(tc.rule)
			if tc.isNil {
				if got != nil {
					t.Errorf("expected nil, got %q", *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %q, got nil", tc.want)
			}
			if *got != tc.want {
				t.Errorf("got %q, want %q", *got, tc.want)
			}
		})
	}
}
