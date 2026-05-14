package cmd

import (
	"fmt"

	"github.com/hellowynd/wyndctl/internal/database"
	"github.com/spf13/cobra"
)

var createPropertyCmd = &cobra.Command{
	Use:   "create-property",
	Short: "Create a lodging property for a user account",
	RunE:  runCreateProperty,
}

var listPropertyCmd = &cobra.Command{
	Use:   "list-property",
	Short: "List lodging properties for a user account",
	RunE:  runListProperty,
}

func init() {
	createPropertyCmd.Flags().String("account", "", "user email address")
	createPropertyCmd.Flags().String("name", "", "property name")
	createPropertyCmd.Flags().String("address", "", "street address")
	createPropertyCmd.Flags().String("city", "", "city")
	createPropertyCmd.Flags().String("state", "", "state or province")
	createPropertyCmd.Flags().String("country", "", "country")
	createPropertyCmd.Flags().String("zip", "", "postal code")
	_ = createPropertyCmd.MarkFlagRequired("account")
	_ = createPropertyCmd.MarkFlagRequired("name")
	_ = createPropertyCmd.MarkFlagRequired("address")
	_ = createPropertyCmd.MarkFlagRequired("city")
	_ = createPropertyCmd.MarkFlagRequired("state")
	_ = createPropertyCmd.MarkFlagRequired("country")
	_ = createPropertyCmd.MarkFlagRequired("zip")
	rootCmd.AddCommand(createPropertyCmd)

	listPropertyCmd.Flags().String("account", "", "user email address")
	_ = listPropertyCmd.MarkFlagRequired("account")
	rootCmd.AddCommand(listPropertyCmd)
}

func runCreateProperty(cmd *cobra.Command, _ []string) error {
	account, _ := cmd.Flags().GetString("account")
	name, _ := cmd.Flags().GetString("name")
	address, _ := cmd.Flags().GetString("address")
	city, _ := cmd.Flags().GetString("city")
	state, _ := cmd.Flags().GetString("state")
	country, _ := cmd.Flags().GetString("country")
	zip, _ := cmd.Flags().GetString("zip")

	repo, err := database.NewRepository(appCfg.DB)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer repo.Close()

	user, err := repo.GetUserByEmail(account)
	if err != nil {
		return fmt.Errorf("user %q not found: %w", account, err)
	}

	extraData := map[string]interface{}{
		"country":     country,
		"state":       state,
		"city":        city,
		"postal_code": zip,
	}

	lodging, err := repo.CreateLodging(user.UserID, name, address, extraData)
	if err != nil {
		return fmt.Errorf("creating property: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Property %q (lodging_id=%d) created for user %s.\n", lodging.Name, lodging.LodgingID, account)
	return nil
}

func runListProperty(cmd *cobra.Command, _ []string) error {
	account, _ := cmd.Flags().GetString("account")

	repo, err := database.NewRepository(appCfg.DB)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer repo.Close()

	user, err := repo.GetUserByEmail(account)
	if err != nil {
		return fmt.Errorf("user %q not found: %w", account, err)
	}

	lodgings, err := repo.ListLodgings(user.UserID)
	if err != nil {
		return fmt.Errorf("listing properties: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Found %d property(s) for %s:\n", len(lodgings), account)
	for _, l := range lodgings {
		fmt.Fprintf(cmd.OutOrStdout(), "  - %s (lodging_id=%d)\n", l.Name, l.LodgingID)
	}
	return nil
}
