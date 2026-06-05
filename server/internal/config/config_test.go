package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATA_DIR", "")
	t.Setenv("HTTP_ADDR", "")
	c := Load()
	if c.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr default = %q", c.HTTPAddr)
	}
	if c.DataDir != "./data" {
		t.Errorf("DataDir default = %q", c.DataDir)
	}
	if c.DBPath != "data/filetag.db" {
		t.Errorf("DBPath = %q", c.DBPath)
	}
	if c.AccessTTL != 15*time.Minute {
		t.Errorf("AccessTTL = %v", c.AccessTTL)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9000")
	t.Setenv("ACCESS_TTL", "30m")
	t.Setenv("GITHUB_OAUTH_CLIENT_ID", "id")
	t.Setenv("GITHUB_OAUTH_CLIENT_SECRET", "sec")
	c := Load()
	if c.HTTPAddr != ":9000" {
		t.Errorf("HTTPAddr = %q", c.HTTPAddr)
	}
	if c.AccessTTL != 30*time.Minute {
		t.Errorf("AccessTTL = %v", c.AccessTTL)
	}
	if !c.GitHub.Configured() {
		t.Error("GitHub should be Configured")
	}
	if c.Google.Configured() {
		t.Error("Google should not be Configured")
	}
}
