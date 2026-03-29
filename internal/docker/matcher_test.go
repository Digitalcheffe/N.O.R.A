package docker

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/digitalcheffe/nora/internal/apptemplate"
)

// buildRegistry constructs a minimal Registry containing only the given profile IDs.
func buildRegistry(profileIDs ...string) *apptemplate.Registry {
	memFS := fstest.MapFS{}
	for _, id := range profileIDs {
		memFS[id+".yaml"] = &fstest.MapFile{
			Data: []byte("meta:\n  name: " + id + "\n"),
		}
	}
	reg, err := apptemplate.NewRegistry(fs.FS(memFS))
	if err != nil {
		panic("buildRegistry: " + err.Error())
	}
	return reg
}

func TestMatchContainerToProfile(t *testing.T) {
	reg := buildRegistry("sonarr", "radarr", "plex", "home-assistant")

	tests := []struct {
		name          string
		containerName string
		image         string
		wantProfile   string // "" means nil result expected
		wantConf      int
	}{
		{
			name:          "exact container name match",
			containerName: "sonarr",
			image:         "some-unrelated-image:latest",
			wantProfile:   "sonarr",
			wantConf:      95,
		},
		{
			name:          "exact match is case-insensitive",
			containerName: "Sonarr",
			image:         "",
			wantProfile:   "sonarr",
			wantConf:      95,
		},
		{
			name:          "image base name exact match",
			containerName: "my-media-server",
			image:         "lscr.io/linuxserver/sonarr:latest",
			wantProfile:   "sonarr",
			wantConf:      85,
		},
		{
			name:          "image base name exact match no registry prefix",
			containerName: "my-container",
			image:         "plex:1.32",
			wantProfile:   "plex",
			wantConf:      85,
		},
		{
			name:          "container name contains profile ID as substring",
			containerName: "sonarr-4k",
			image:         "unrelated-image:latest",
			wantProfile:   "sonarr",
			wantConf:      80,
		},
		{
			name:          "image base name contains profile ID as substring",
			containerName: "my-pvr",
			image:         "ghcr.io/team/sonarr-custom:edge",
			wantProfile:   "sonarr",
			wantConf:      70,
		},
		{
			name:          "no match returns nil",
			containerName: "postgres",
			image:         "postgres:15",
			wantProfile:   "",
			wantConf:      0,
		},
		{
			name:          "below threshold confidence is not returned",
			containerName: "totally-unrelated",
			image:         "totally-unrelated:latest",
			wantProfile:   "",
			wantConf:      0,
		},
		{
			name:          "nil registry returns nil",
			containerName: "sonarr",
			image:         "sonarr:latest",
			wantProfile:   "",
			wantConf:      0,
		},
		{
			name:          "home-assistant hyphenated image base name",
			containerName: "ha",
			image:         "ghcr.io/home-assistant/home-assistant:stable",
			wantProfile:   "home-assistant",
			wantConf:      85,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var r *apptemplate.Registry
			if tc.name != "nil registry returns nil" {
				r = reg
			}

			got := MatchContainerToProfile(tc.containerName, tc.image, r)

			if tc.wantProfile == "" {
				if got != nil {
					t.Errorf("expected nil result, got ProfileID=%q Confidence=%d", got.ProfileID, got.Confidence)
				}
				return
			}

			if got == nil {
				t.Fatalf("expected match for profile %q, got nil", tc.wantProfile)
			}
			if got.ProfileID != tc.wantProfile {
				t.Errorf("ProfileID: got %q, want %q", got.ProfileID, tc.wantProfile)
			}
			if got.Confidence != tc.wantConf {
				t.Errorf("Confidence: got %d, want %d", got.Confidence, tc.wantConf)
			}
		})
	}
}

func TestImageBaseName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"nginx:latest", "nginx"},
		{"lscr.io/linuxserver/sonarr:latest", "sonarr"},
		{"ghcr.io/home-assistant/home-assistant:stable", "home-assistant"},
		{"registry.example.com:5000/myapp:v1.2", "myapp"},
		{"ubuntu", "ubuntu"},
		{"", ""},
	}
	for _, tc := range tests {
		got := imageBaseName(tc.input)
		if got != tc.want {
			t.Errorf("imageBaseName(%q): got %q, want %q", tc.input, got, tc.want)
		}
	}
}
