package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/hellowynd/wyndctl/internal/deployment"
	"github.com/hellowynd/wyndctl/internal/device"
	"github.com/hellowynd/wyndctl/internal/models"
	"github.com/hellowynd/wyndctl/internal/transport"
	"github.com/spf13/cobra"
)

const (
	fwTargetMain = "$firmware_path"
	fwTargetWiFi = "$bw16_firmware_path"
	fwTargetPMM  = "$pm_firmware_path"

	// Time the device needs to apply each image after rebooting.
	// Derived from the Python mass_deployment script's wait_for_fw_update_in_sec.
	fwWaitBase = 2 * time.Second
	fwWaitMain = 35 * time.Second
	fwWaitWiFi = 140 * time.Second
	fwWaitPMM  = 100 * time.Second
)

type fwEntry struct {
	target string
	data   []byte
}

var fwUpdateCmd = &cobra.Command{
	Use:   "fw-update",
	Short: "Update firmware on connected Sentry devices",
	Long: `Writes one or more firmware images to connected Sentry devices over USB:
  - Main firmware  (--firmware)
  - WiFi firmware  (--wifi-firmware)
  - PMM firmware   (--pmm-firmware)

Results are logged to fw-update-result.csv. Re-runs without --all
only update devices that failed in the previous run.`,
	RunE: runFWUpdate,
}

func init() {
	fwUpdateCmd.Flags().String("firmware", "", "path to main firmware binary")
	fwUpdateCmd.Flags().String("wifi-firmware", "", "path to WiFi (bw16) firmware binary")
	fwUpdateCmd.Flags().String("pmm-firmware", "", "path to PMM firmware binary")
	fwUpdateCmd.Flags().Bool("all", false, "update all connected devices (default: only failed from previous run)")
	fwUpdateCmd.Flags().Int("workers", runtime.NumCPU(), "number of parallel update workers")
	rootCmd.AddCommand(fwUpdateCmd)
}

func runFWUpdate(cmd *cobra.Command, args []string) error {
	fwPath, _ := cmd.Flags().GetString("firmware")
	wifiFWPath, _ := cmd.Flags().GetString("wifi-firmware")
	pmmFWPath, _ := cmd.Flags().GetString("pmm-firmware")
	all, _ := cmd.Flags().GetBool("all")
	workers, _ := cmd.Flags().GetInt("workers")

	if fwPath == "" && wifiFWPath == "" && pmmFWPath == "" {
		return fmt.Errorf("at least one of --firmware, --wifi-firmware, or --pmm-firmware is required")
	}

	// Validate and load firmware files
	var firmwares []fwEntry
	for _, pair := range []struct{ path, target string }{
		{fwPath, fwTargetMain},
		{wifiFWPath, fwTargetWiFi},
		{pmmFWPath, fwTargetPMM},
	} {
		if pair.path == "" {
			continue
		}
		data, err := os.ReadFile(pair.path)
		if err != nil {
			return fmt.Errorf("reading firmware file %s: %w", pair.path, err)
		}
		appLog.Info("loaded firmware", "path", pair.path, "size", len(data))
		firmwares = append(firmwares, fwEntry{target: pair.target, data: data})
	}

	// Load location map (optional)
	folder := "."
	mapPath := filepath.Join(folder, "location-map.csv")
	var locToBay map[string]int
	if _, statErr := os.Stat(mapPath); statErr == nil {
		var err error
		locToBay, _, err = deployment.LoadLocationMap(mapPath, true)
		if err != nil {
			return fmt.Errorf("loading location map: %w", err)
		}
	} else {
		appLog.Warn("no location map found, using default bay order")
	}

	// Scan + activate
	commander := device.NewCommander(appCfg.USB, appCfg.MQTT, appCfg.Env, appLog)
	channels, err := commander.Scan()
	if err != nil {
		return err
	}
	if len(channels) == 0 {
		return fmt.Errorf("no devices found")
	}
	if err := commander.ActivateAll(); err != nil {
		return fmt.Errorf("activating channels: %w", err)
	}
	defer commander.CloseAll()

	// Determine which channels to update
	outputPath := filepath.Join(folder, "fw-update-result.csv")
	channelsToUpdate := channels
	if !all {
		if _, statErr := os.Stat(outputPath); statErr == nil {
			prevResults, err := deployment.ReadFWUpdateResults(outputPath)
			if err == nil && len(prevResults) > 0 {
				appLog.Info("filtering to failed devices from previous run")
				filtered := make(map[string]*transport.SerialChannel)
				for loc, ch := range channels {
					bay := locationToBay(loc, locToBay, channels)
					if r, ok := prevResults[bay]; ok && r.Succeeded {
						appLog.Info("skipping already-succeeded device", "bay", bay, "location", loc)
						continue
					}
					filtered[loc] = ch
				}
				channelsToUpdate = filtered
			}
		}
	}

	if len(channelsToUpdate) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No devices to update.")
		return nil
	}

	if !confirmAction(cmd.ErrOrStderr(), cmd.InOrStdin(),
		fmt.Sprintf("Update firmware on %d device(s) on %s?", len(channelsToUpdate), appCfg.Env)) {
		return fmt.Errorf("firmware update cancelled by user")
	}

	if err := deployment.WriteFWUpdateHeader(outputPath); err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}

	// Sort locations for deterministic ordering
	locations := make([]string, 0, len(channelsToUpdate))
	for loc := range channelsToUpdate {
		locations = append(locations, loc)
	}
	sort.Strings(locations)

	var (
		wg      sync.WaitGroup
		sem     = make(chan struct{}, workers)
		resultMu sync.Mutex
	)

	for idx, loc := range locations {
		ch := channelsToUpdate[loc]
		bay := idx + 1
		if locToBay != nil {
			if b, ok := locToBay[loc]; ok {
				bay = b
			}
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(c *transport.SerialChannel, bayNum int) {
			defer wg.Done()
			defer func() { <-sem }()
			updateDeviceFirmware(commander, c, bayNum, firmwares, outputPath, &resultMu)
		}(ch, bay)
	}

	wg.Wait()
	fmt.Fprintln(cmd.OutOrStdout(), "All firmware update tasks completed.")
	return nil
}

func updateDeviceFirmware(
	commander *device.Commander,
	ch *transport.SerialChannel,
	bay int,
	firmwares []fwEntry,
	outputPath string,
	mu *sync.Mutex,
) {
	log := appLog.With("bay", bay)

	info, err := commander.GetDeviceInfo(ch)
	if err != nil {
		log.Error("failed to get device info", "error", err)
		writeFWResult(mu, outputPath, &models.FirmwareUpdateResult{Bay: bay, Reason: err.Error()})
		return
	}

	log.Info("device identified",
		"device_id", info.AWSThingName,
		"serial", info.SerialNumber,
		"hw_rev", info.HardwareRevision,
		"fw_rev", info.FirmwareRevision,
		"wifi_fw_rev", info.WiFiFWRevision,
		"pm_fw_rev", info.PMFWRevision,
	)

	if err := commander.SetPower(ch, false); err != nil {
		log.Error("failed to power off device", "error", err)
		writeFWResult(mu, outputPath, &models.FirmwareUpdateResult{
			Bay: bay, DeviceID: info.AWSThingName, MACAddr: info.WiFiMAC, Reason: err.Error(),
		})
		return
	}

	applyWait := fwWaitBase
	for _, fw := range firmwares {
		log.Info("writing firmware", "target", fw.target, "size", len(fw.data))
		if err := commander.WriteFirmware(ch, fw.target, fw.data, 4*time.Second); err != nil {
			log.Error("firmware write failed", "target", fw.target, "error", err)
			writeFWResult(mu, outputPath, &models.FirmwareUpdateResult{
				Bay: bay, DeviceID: info.AWSThingName, MACAddr: info.WiFiMAC,
				Reason: fmt.Sprintf("%s: %s", fw.target, err),
			})
			return
		}
		log.Info("firmware written", "target", fw.target)
		switch fw.target {
		case fwTargetMain:
			applyWait += fwWaitMain
		case fwTargetWiFi:
			applyWait += fwWaitWiFi
		case fwTargetPMM:
			applyWait += fwWaitPMM
		}
	}

	if err := commander.Reboot(ch); err != nil {
		log.Warn("reboot error (likely normal during firmware apply)", "error", err)
	}
	_ = commander.CloseChannel(ch)

	log.Info("waiting for device to apply firmware and reboot", "wait", applyWait)
	time.Sleep(applyWait)

	log.Info("firmware update completed")
	writeFWResult(mu, outputPath, &models.FirmwareUpdateResult{
		Bay: bay, DeviceID: info.AWSThingName, MACAddr: info.WiFiMAC, Succeeded: true,
	})
}

func writeFWResult(mu *sync.Mutex, path string, r *models.FirmwareUpdateResult) {
	mu.Lock()
	defer mu.Unlock()
	if err := deployment.AppendFWUpdateResult(path, r); err != nil {
		appLog.Error("failed to write fw-update result", "error", err)
	}
}

// locationToBay returns the bay number for a USB location string.
// Falls back to a 1-based index derived from sorted position if no map is available.
func locationToBay(loc string, locToBay map[string]int, channels map[string]*transport.SerialChannel) int {
	if locToBay != nil {
		if b, ok := locToBay[loc]; ok {
			return b
		}
	}
	// Stable fallback: sort all locations and use 1-based position
	locs := make([]string, 0, len(channels))
	for l := range channels {
		locs = append(locs, l)
	}
	sort.Strings(locs)
	for i, l := range locs {
		if l == loc {
			return i + 1
		}
	}
	return 0
}
