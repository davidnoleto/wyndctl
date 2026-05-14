package cmd

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hellowynd/wyndctl/internal/device"
	"github.com/hellowynd/wyndctl/internal/models"
	"github.com/spf13/cobra"
)

type scannedDevice struct {
	Location string `json:"location"`
	DeviceID string `json:"device_id"`
	Firmware string `json:"firmware"`
	WiFiFW   string `json:"wifi_fw"`
	PMMFW    string `json:"pmm_fw"`
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan for connected Sentry devices",
	Long:  `Discovers all Sentry devices connected via USB and optionally labels their bay positions.`,
	RunE:  runScan,
}

func init() {
	scanCmd.Flags().Bool("label", false, "interactively label device bay positions")
	scanCmd.Flags().String("color", "255,165,0", "RGB color for device LEDs (e.g. 255,0,0)")
	scanCmd.Flags().StringP("output", "o", "text", "output format (text, json)")
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	label, _ := cmd.Flags().GetBool("label")
	colorStr, _ := cmd.Flags().GetString("color")
	outputFmt, _ := cmd.Flags().GetString("output")
	if !cmd.Flags().Changed("output") && appCfg.Log.Format == "json" {
		outputFmt = "json"
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	folder := wd

	color, err := models.ParseRGB(colorStr)
	if err != nil {
		return err
	}

	commander := device.NewCommander(appCfg.USB, appCfg.MQTT, appCfg.Env, appLog)
	channels, err := commander.Scan()
	if err != nil {
		return err
	}

	if len(channels) == 0 {
		if outputFmt == "json" {
			fmt.Fprintln(cmd.OutOrStdout(), "[]")
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "No Sentry devices found.")
		}
		return nil
	}

	if outputFmt != "json" {
		fmt.Fprintf(cmd.ErrOrStderr(), "Found %d connected Sentry device(s).\n", len(channels))
	}

	if err := commander.ActivateAll(); err != nil {
		return fmt.Errorf("activating channels: %w", err)
	}
	defer commander.CloseAll()

	// Turn off all LEDs first (sorted for deterministic order)
	locations := make([]string, 0, len(channels))
	for loc := range channels {
		locations = append(locations, loc)
	}
	sort.Strings(locations)

	for _, loc := range locations {
		_ = commander.CancelIndication(channels[loc])
		time.Sleep(1500 * time.Millisecond)
	}

	// If labeling, open CSV writer
	var writer *csv.Writer
	var mapFile *os.File
	if label {
		if err := os.MkdirAll(folder, 0755); err != nil {
			return fmt.Errorf("creating folder: %w", err)
		}

		mapPath := filepath.Join(folder, "location-map.csv")
		mapFile, err = os.Create(mapPath)
		if err != nil {
			return fmt.Errorf("creating location map: %w", err)
		}
		defer mapFile.Close()

		writer = csv.NewWriter(mapFile)
		defer writer.Flush()
		_ = writer.Write([]string{"bay", "location", "device_id"})
	}

	colorVal := uint32(color)
	var results []scannedDevice
	stdin := bufio.NewReader(cmd.InOrStdin())
	for _, location := range locations {
		ch := channels[location]
		info, err := commander.GetDeviceInfo(ch)
		if err != nil {
			appLog.Warn("failed to get device info", "location", location, "error", err)
			continue
		}

		results = append(results, scannedDevice{
			Location: location,
			DeviceID: info.AWSThingName,
			Firmware: info.FirmwareRevision,
			WiFiFW:   info.WiFiFWRevision,
			PMMFW:    info.PMFWRevision,
		})

		if outputFmt != "json" {
			fmt.Fprintf(cmd.OutOrStdout(), "\n  Device at %s:\n", location)
			fmt.Fprintf(cmd.OutOrStdout(), "    (AWSThingName)-ID: %s\n", info.AWSThingName)
			fmt.Fprintf(cmd.OutOrStdout(), "    Firmware:    %s\n", info.FirmwareRevision)
			fmt.Fprintf(cmd.OutOrStdout(), "    WiFi FW:     %s\n", info.WiFiFWRevision)
			fmt.Fprintf(cmd.OutOrStdout(), "    PM FW:       %s\n", info.PMFWRevision)
		}

		if label {
			_ = commander.SetIndicate(ch, colorVal, colorVal, colorVal, 3600)
			fmt.Fprintf(cmd.ErrOrStderr(), "  Bay number for %s: ", location)
			var bayNum string
			fmt.Fscan(stdin, &bayNum)
			_ = writer.Write([]string{bayNum, location, info.AWSThingName})
			_ = commander.CancelIndication(ch)
			time.Sleep(1500 * time.Millisecond)
		} else {
			_ = commander.SetIndicate(ch, colorVal, colorVal, colorVal, 3600)
		}
	}

	if outputFmt == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	return nil
}
