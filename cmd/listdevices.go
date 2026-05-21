package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"

	"github.com/hellowynd/wyndctl/internal/database"
	"github.com/spf13/cobra"
)

var listDevicesCmd = &cobra.Command{
	Use:   "list-devices",
	Short: "List devices assigned to a user account",
	Long: `Lists all devices currently assigned to rooms across a user's properties.
Use --lodging-id to filter to a single property.
Output formats: text (default), json, csv (stdout).`,
	RunE: runListDevices,
}

func init() {
	listDevicesCmd.Flags().String("account", "", "user email address")
	listDevicesCmd.Flags().Int("lodging-id", 0, "filter to a specific lodging")
	listDevicesCmd.Flags().StringP("output", "o", "text", "output format: text, json, csv")
	_ = listDevicesCmd.MarkFlagRequired("account")
	rootCmd.AddCommand(listDevicesCmd)
}

func runListDevices(cmd *cobra.Command, _ []string) error {
	account, _ := cmd.Flags().GetString("account")
	lodgingIDFlag, _ := cmd.Flags().GetInt("lodging-id")
	outputFmt, _ := cmd.Flags().GetString("output")

	repo, err := database.NewRepository(appCfg.DB)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer repo.Close()

	user, err := repo.GetUserByEmail(account)
	if err != nil {
		return fmt.Errorf("user %q not found: %w", account, err)
	}

	var lodgingID *int
	if lodgingIDFlag > 0 {
		lodgingID = &lodgingIDFlag
	}

	rows, err := repo.ListDevices(user.UserID, lodgingID)
	if err != nil {
		return fmt.Errorf("listing devices: %w", err)
	}

	switch outputFmt {
	case "json":
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(rows)

	case "csv":
		w := csv.NewWriter(cmd.OutOrStdout())
		_ = w.Write([]string{"device_id", "lodging_id", "lodging_name", "zone_id", "zone_name"})
		for _, r := range rows {
			_ = w.Write([]string{
				r.DeviceID,
				fmt.Sprintf("%d", r.LodgingID),
				r.LodgingName,
				fmt.Sprintf("%d", r.ZoneID),
				r.ZoneName,
			})
		}
		w.Flush()
		return w.Error()

	default:
		fmt.Fprintf(cmd.OutOrStdout(), "Found %d device(s) for %s:\n", len(rows), account)
		for _, r := range rows {
			fmt.Fprintf(cmd.OutOrStdout(), "  %-40s  lodging=%-4d %-20s  zone=%-4d %s\n",
				r.DeviceID, r.LodgingID, r.LodgingName, r.ZoneID, r.ZoneName)
		}
		return nil
	}
}
