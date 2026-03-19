package config

import (
	"os"
	"testing"
)

// TestLoadAppConfig tests the LoadAppConfig function
func TestLoadAppConfig(t *testing.T) {
	tests := []struct {
		name           string
		env            map[string]string
		expectError    bool
		expectedAppID  int64
		expectedSecret string
		expectedKey    string
	}{
		{
			name:        "all missing",
			env:         map[string]string{},
			expectError: true,
		},
		{
			name: "invalid app id",
			env: map[string]string{
				"GITHUB_APP_ID": "invalid",
			},
			expectError: true,
		},
		{
			name: "all set",
			env: map[string]string{
				"GITHUB_APP_ID":             "2895893256896859",
				"GITHUB_APP_WEBHOOK_SECRET": "secret",
				"GITHUB_APP_PRIVATE_KEY":    "another_secret_key",
				"CLERK_SECRET_KEY":          "sk_test_xxx",
				"HMAC_SIGNING_KEY":          "test-hmac-key",
			},
			expectError:    false,
			expectedAppID:  2895893256896859,
			expectedSecret: "secret",
			expectedKey:    "another_secret_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			originalEnv := map[string]string{
				"GITHUB_APP_ID":             os.Getenv("GITHUB_APP_ID"),
				"GITHUB_APP_WEBHOOK_SECRET": os.Getenv("GITHUB_APP_WEBHOOK_SECRET"),
				"GITHUB_APP_PRIVATE_KEY":    os.Getenv("GITHUB_APP_PRIVATE_KEY"),
				"CLERK_SECRET_KEY":          os.Getenv("CLERK_SECRET_KEY"),
				"HMAC_SIGNING_KEY":          os.Getenv("HMAC_SIGNING_KEY"),
			}

			// Restore original environment variables after test
			defer func() {
				for k, v := range originalEnv {
					os.Setenv(k, v)
				}
			}()

			// Set up environment variables for this test
			for k, v := range tt.env {
				os.Setenv(k, v)
			}

			cfg, err := LoadAppConfig()
			if tt.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}

			if !tt.expectError {
				if err != nil {
					t.Fatalf("did not expect error but got: %v", err)
				}
				if cfg.GithubAppID != tt.expectedAppID {
					t.Errorf("expected app ID %d, got %d", tt.expectedAppID, cfg.GithubAppID)
				}
				if cfg.GithubAppWebhookSecret != tt.expectedSecret {
					t.Errorf("expected secret %q, got %q", tt.expectedSecret, cfg.GithubAppWebhookSecret)
				}
				if cfg.GithubAppPrivateKey != tt.expectedKey {
					t.Errorf("expected private key %q, got %q", tt.expectedKey, cfg.GithubAppPrivateKey)
				}
			}
		})
	}
}
