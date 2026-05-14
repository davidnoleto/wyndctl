package cmd

import (
	"fmt"

	"github.com/hellowynd/wyndctl/internal/database"
	"github.com/spf13/cobra"
)

var deleteDeviceCmd = &cobra.Command{
	Use:   "delete-device",
	Short: "Delete device assignments for a user",
	Long: `Removes device-to-room assignments from the database.
Device rows and AWS IoT certificates/policies are preserved so devices can
be redeployed. Pass --delete-room to also remove the associated zone records.`,
	RunE: runDeleteDevice,
}

func init() {
	deleteDeviceCmd.Flags().String("account", "", "user email address")
	deleteDeviceCmd.Flags().Int("lodging-id", 0, "lodging ID to scope deletion (optional)")
	deleteDeviceCmd.Flags().String("device-id", "", "specific device ID (AWS Thing name) to delete")
	deleteDeviceCmd.Flags().Bool("delete-room", false, "also delete the associated room/zone records")
	_ = deleteDeviceCmd.MarkFlagRequired("account")
	rootCmd.AddCommand(deleteDeviceCmd)
}

func runDeleteDevice(cmd *cobra.Command, args []string) error {
	account, _ := cmd.Flags().GetString("account")
	lodgingID, _ := cmd.Flags().GetInt("lodging-id")
	deviceID, _ := cmd.Flags().GetString("device-id")
	deleteRoom, _ := cmd.Flags().GetBool("delete-room")

	repo, err := database.NewRepository(appCfg.DB)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer repo.Close()

	user, err := repo.GetUserByEmail(account)
	if err != nil {
		return fmt.Errorf("user %q not found: %w", account, err)
	}

	var lidPtr *int
	if lodgingID > 0 {
		lidPtr = &lodgingID
	}
	var didPtr *string
	if deviceID != "" {
		didPtr = &deviceID
	}

	thingNames, err := repo.DeleteDevices(user.UserID, lidPtr, didPtr, deleteRoom)
	if err != nil {
		return fmt.Errorf("deleting devices: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Cleared %d device assignment(s) from database.\n", len(thingNames))
	return nil
}
