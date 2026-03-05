package main

import (
	"os"
	"testing"
)

func TestEnvOrDefault(t *testing.T) {
	// Test with env set
	os.Setenv("TEST_ENV_OR_DEFAULT", "from_env")
	defer os.Unsetenv("TEST_ENV_OR_DEFAULT")

	if got := envOrDefault("TEST_ENV_OR_DEFAULT", "fallback"); got != "from_env" {
		t.Errorf("envOrDefault with env set = %q, want %q", got, "from_env")
	}

	// Test with env unset
	if got := envOrDefault("TEST_ENV_MISSING_KEY", "fallback"); got != "fallback" {
		t.Errorf("envOrDefault with env unset = %q, want %q", got, "fallback")
	}
}

func TestEnvBoolOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		envVal   string
		envSet   bool
		fallback bool
		want     bool
	}{
		{"true", "true", true, false, true},
		{"1", "1", true, false, true},
		{"yes", "yes", true, false, true},
		{"YES", "YES", true, false, true},
		{"false", "false", true, true, false},
		{"0", "0", true, true, false},
		{"empty", "", true, true, false},
		{"unset_false", "", false, false, false},
		{"unset_true", "", false, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_BOOL_" + tt.name
			if tt.envSet {
				os.Setenv(key, tt.envVal)
				defer os.Unsetenv(key)
			} else {
				os.Unsetenv(key)
			}

			if got := envBoolOrDefault(key, tt.fallback); got != tt.want {
				t.Errorf("envBoolOrDefault(%q, %q, %v) = %v, want %v", key, tt.envVal, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestEnvIntOrDefault(t *testing.T) {
	os.Setenv("TEST_INT_VALID", "42")
	defer os.Unsetenv("TEST_INT_VALID")

	if got := envIntOrDefault("TEST_INT_VALID", 10); got != 42 {
		t.Errorf("envIntOrDefault valid = %d, want 42", got)
	}

	os.Setenv("TEST_INT_INVALID", "abc")
	defer os.Unsetenv("TEST_INT_INVALID")

	if got := envIntOrDefault("TEST_INT_INVALID", 10); got != 10 {
		t.Errorf("envIntOrDefault invalid = %d, want 10", got)
	}

	if got := envIntOrDefault("TEST_INT_MISSING", 10); got != 10 {
		t.Errorf("envIntOrDefault missing = %d, want 10", got)
	}
}
