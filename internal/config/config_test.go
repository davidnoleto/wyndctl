package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestMapAuthEnvVars(t *testing.T) {
	tests := []struct {
		name             string
		appClientID      string
		stripeKey        string
		wantAppClientID  string
		wantStripeAPIKey string
	}{
		{
			name:             "both set",
			appClientID:      "abc123",
			stripeKey:        "sk_test_xyz",
			wantAppClientID:  "abc123",
			wantStripeAPIKey: "sk_test_xyz",
		},
		{
			name:             "only cognito set",
			appClientID:      "only-cognito",
			wantAppClientID:  "only-cognito",
			wantStripeAPIKey: "",
		},
		{
			name:             "only stripe set",
			stripeKey:        "sk_live_only",
			wantStripeAPIKey: "sk_live_only",
		},
		{
			name: "neither set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// viper is process-global; reset between subtests so values
			// don't bleed across iterations.
			viper.Reset()
			t.Setenv("APP_CLIENT_ID", tt.appClientID)
			t.Setenv("STRIPE_API_KEY", tt.stripeKey)

			mapAuthEnvVars()

			if got := viper.GetString("cognito.app_client_id"); got != tt.wantAppClientID {
				t.Errorf("cognito.app_client_id = %q, want %q", got, tt.wantAppClientID)
			}
			if got := viper.GetString("stripe.api_key"); got != tt.wantStripeAPIKey {
				t.Errorf("stripe.api_key = %q, want %q", got, tt.wantStripeAPIKey)
			}
		})
	}
}

func TestDefaultConfig_CognitoRegion(t *testing.T) {
	d := DefaultConfig()
	if d.Cognito.Region != "us-west-2" {
		t.Errorf("default Cognito.Region = %q, want us-west-2", d.Cognito.Region)
	}
}
