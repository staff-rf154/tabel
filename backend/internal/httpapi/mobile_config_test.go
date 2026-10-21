package httpapi

import (
	"testing"

	"tabel/internal/repo"
)

func TestBuildMobileBootstrapRoles(t *testing.T) {
	tests := []struct {
		role         string
		shell        string
		navFirst     string
		checkFeature string
	}{
		{role: "employee", shell: "employee", navFirst: "home", checkFeature: "monthly_stats"},
		{role: "operator", shell: "operator", navFirst: "scan", checkFeature: "queue"},
		{role: "shift_lead", shell: "management", navFirst: "summary", checkFeature: "reports_summary"},
		{role: "director", shell: "management", navFirst: "summary", checkFeature: "reports_summary"},
		{role: "hr", shell: "management", navFirst: "summary", checkFeature: "reports_summary"},
		{role: "admin", shell: "management", navFirst: "summary", checkFeature: "reports_summary"},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			config := buildMobileBootstrap(repo.Profile{Role: tt.role})
			if config.Shell != tt.shell {
				t.Fatalf("shell = %q, want %q", config.Shell, tt.shell)
			}
			if len(config.Navigation) == 0 || config.Navigation[0].Key != tt.navFirst {
				t.Fatalf("navigation = %#v, want first %q", config.Navigation, tt.navFirst)
			}
			if !config.Features[tt.checkFeature] {
				t.Fatalf("feature %q is disabled", tt.checkFeature)
			}
			if !config.Policy.CacheUntilLogout {
				t.Fatal("interface config must be cached until logout")
			}
		})
	}
}

func TestBuildMobileBootstrapAdminHasUsersDirectory(t *testing.T) {
	config := buildMobileBootstrap(repo.Profile{Role: "admin"})
	if !config.Features["users_directory"] {
		t.Fatal("admin must have users directory")
	}
	found := false
	for _, item := range config.Navigation {
		if item.Key == "users" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("admin navigation = %#v, want users item", config.Navigation)
	}
}

func TestBuildMobileBootstrapUnknownRole(t *testing.T) {
	config := buildMobileBootstrap(repo.Profile{Role: "auditor"})
	if config.Shell != "unsupported" {
		t.Fatalf("shell = %q, want unsupported", config.Shell)
	}
	if config.RoleTitle != "auditor" {
		t.Fatalf("role title = %q, want auditor", config.RoleTitle)
	}
}
