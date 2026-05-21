package cmd

import (
	"fmt"
	"sort"
	"time"

	"github.com/hellowynd/wyndctl/internal/device"
	"github.com/spf13/cobra"
)

var unprovisionCmd = &cobra.Command{
	Use:   "unprovision",
	Short: "Clear WiFi and MQTT credentials from connected Sentry devices",
	Long: `Sends an empty SetProvisionInformation RPC to each connected device,
wiping its WiFi and MQTT configuration and returning it to UNPROVISIONED state.
Use --device-id to scope to a single device; omit to unprovision all connected devices.`,
	RunE: runUnprovision,
}

func init() {
	unprovisionCmd.Flags().String("device-id", "", "AWS Thing name of a specific device to unprovision")
	rootCmd.AddCommand(unprovisionCmd)
}

func runUnprovision(cmd *cobra.Command, _ []string) error {
	deviceID, _ := cmd.Flags().GetString("device-id")

	commander := device.NewCommander(appCfg.USB, appCfg.MQTT, appCfg.Env, appLog)
	channels, err := commander.Scan()
	if err != nil {
		return err
	}
	if len(channels) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No Sentry devices found.")
		return nil
	}

	if err := commander.ActivateAll(); err != nil {
		return fmt.Errorf("activating channels: %w", err)
	}
	defer commander.CloseAll()

	locations := make([]string, 0, len(channels))
	for loc := range channels {
		locations = append(locations, loc)
	}
	sort.Strings(locations)

	count := 0
	for _, loc := range locations {
		ch := channels[loc]

		if deviceID != "" {
			info, err := commander.GetDeviceInfo(ch)
			if err != nil {
				appLog.Warn("failed to get device info", "location", loc, "error", err)
				continue
			}
			if info.AWSThingName != deviceID {
				continue
			}
		}

		_ = commander.SetAdvertising(ch, false)
		_ = commander.CancelIndication(ch)
		time.Sleep(1500 * time.Millisecond)

		if err := commander.Unprovision(ch); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "  %s: unprovision failed: %v\n", loc, err)
			continue
		}

		fmt.Fprintf(cmd.OutOrStdout(), "  %s: unprovisioned.\n", loc)
		count++
	}

	if deviceID != "" && count == 0 {
		return fmt.Errorf("device %q not found among connected devices", deviceID)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Unprovision complete: %d device(s).\n", count)
	return nil
}
