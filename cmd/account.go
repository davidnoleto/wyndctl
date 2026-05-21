package cmd

import (
	"fmt"

	"github.com/hellowynd/wyndctl/internal/database"
	"github.com/spf13/cobra"
)

var createAccountCmd = &cobra.Command{
	Use:   "create-account",
	Short: "Create a new user account in the platform database",
	RunE:  runCreateAccount,
}

func init() {
	createAccountCmd.Flags().String("email", "", "user email address")
	createAccountCmd.Flags().String("name", "", "user full name")
	_ = createAccountCmd.MarkFlagRequired("email")
	_ = createAccountCmd.MarkFlagRequired("name")
	rootCmd.AddCommand(createAccountCmd)
}

func runCreateAccount(cmd *cobra.Command, _ []string) error {
	email, _ := cmd.Flags().GetString("email")
	name, _ := cmd.Flags().GetString("name")

	repo, err := database.NewRepository(appCfg.DB)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer repo.Close()

	user, err := repo.CreateUser(email, name)
	if err != nil {
		return fmt.Errorf("creating account: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Account created: %s (user_id=%d)\n", user.Email, user.UserID)
	return nil
}
