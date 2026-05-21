package cmd

import (
	"fmt"

	"github.com/hellowynd/wyndctl/internal/auth"
	"github.com/hellowynd/wyndctl/internal/billing"
	"github.com/hellowynd/wyndctl/internal/database"
	"github.com/spf13/cobra"
)

var allowedClientTypes = map[string]bool{
	"hotel":       true,
	"multifamily": true,
}

var createAccountCmd = &cobra.Command{
	Use:   "create-account",
	Short: "Create a new user account (Stripe customer, Cognito user, platform DB row)",
	Long: `Mirrors the Python mass_deployment create-account flow:
  1. Find or create a Stripe customer for the email.
  2. Sign up the user in AWS Cognito.
  3. Insert the user row + default UserProfile into the platform database.

Order matches the Python script. Cognito and Stripe require configuration
(WYND_COGNITO_APP_CLIENT_ID / APP_CLIENT_ID and WYND_STRIPE_API_KEY /
STRIPE_API_KEY). The password is required by Cognito and is never logged.`,
	RunE: runCreateAccount,
}

func init() {
	createAccountCmd.Flags().String("email", "", "user email address")
	createAccountCmd.Flags().String("name", "", "user full name")
	createAccountCmd.Flags().String("password", "", "Cognito password for the new user")
	createAccountCmd.Flags().String("enterprise-name", "", "enterprise/organization name (Cognito custom:organization attribute)")
	createAccountCmd.Flags().String("stripe-description", "ENTERPRISE", "description used when creating the Stripe customer")
	createAccountCmd.Flags().Bool("is-smoke-only", false, "tag the user as smoke-only in user_profile.extra_data")
	createAccountCmd.Flags().String("client-type", "", "client type for user_profile.extra_data (hotel|multifamily)")
	_ = createAccountCmd.MarkFlagRequired("email")
	_ = createAccountCmd.MarkFlagRequired("name")
	_ = createAccountCmd.MarkFlagRequired("password")
	rootCmd.AddCommand(createAccountCmd)
}

func runCreateAccount(cmd *cobra.Command, _ []string) error {
	email, _ := cmd.Flags().GetString("email")
	name, _ := cmd.Flags().GetString("name")
	// SECURITY: password is intentionally not logged anywhere. Don't print it.
	password, _ := cmd.Flags().GetString("password")
	enterpriseName, _ := cmd.Flags().GetString("enterprise-name")
	stripeDesc, _ := cmd.Flags().GetString("stripe-description")
	isSmokeOnly, _ := cmd.Flags().GetBool("is-smoke-only")
	clientType, _ := cmd.Flags().GetString("client-type")

	if clientType != "" && !allowedClientTypes[clientType] {
		return fmt.Errorf("invalid --client-type %q: must be one of hotel, multifamily", clientType)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Creating account %s...\n", email)

	// Step 1: Stripe (matches Python order)
	if _, err := billing.FindOrCreateCustomer(appCfg.Stripe.APIKey, email, name, stripeDesc, out); err != nil {
		return fmt.Errorf("stripe: %w", err)
	}

	// Step 2: Cognito
	if err := auth.SignUp(appCfg.Cognito.AppClientID, appCfg.Cognito.Region, auth.SignUpInput{
		Email:          email,
		FullName:       name,
		Password:       password,
		EnterpriseName: enterpriseName,
	}); err != nil {
		return fmt.Errorf("cognito: %w", err)
	}
	fmt.Fprintln(out, "Cognito user signed up successfully")

	// Step 3: platform DB
	repo, err := database.NewRepository(appCfg.DB)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer repo.Close()

	user, err := repo.CreateUser(email, name, isSmokeOnly, clientType)
	if err != nil {
		return fmt.Errorf("creating account: %w", err)
	}

	fmt.Fprintf(out, "Account created: %s (user_id=%d)\n", user.Email, user.UserID)
	return nil
}
