package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hellowynd/wyndctl/internal/deployment"
	"github.com/hellowynd/wyndctl/internal/device"
	"github.com/hellowynd/wyndctl/internal/transport"
	"github.com/hellowynd/wyndctl/internal/tui"
	"github.com/spf13/cobra"
)

var fwUpdateTUICmd = &cobra.Command{
	Use:   "fw-update-tui",
	Short: "Update firmware with a live TUI (experimental)",
	Long: `Same as fw-update but renders a live per-device status table instead of
streaming log lines. Slog output is redirected to fw-update-tui.log while the
TUI runs. Results still land in fw-update-result.csv.

This command is experimental — fw-update remains the canonical entry point.`,
	RunE: runFWUpdateTUI,
}

func init() {
	fwUpdateTUICmd.Flags().String("firmware", "", "path to main firmware binary")
	fwUpdateTUICmd.Flags().String("wifi-firmware", "", "path to WiFi (bw16) firmware binary")
	fwUpdateTUICmd.Flags().String("pmm-firmware", "", "path to PMM firmware binary")
	fwUpdateTUICmd.Flags().Bool("all", false, "update all connected devices (default: only failed from previous run)")
	fwUpdateTUICmd.Flags().Int("workers", runtime.NumCPU(), "number of parallel update workers")
	rootCmd.AddCommand(fwUpdateTUICmd)
}

func runFWUpdateTUI(cmd *cobra.Command, args []string) error {
	fwPath, _ := cmd.Flags().GetString("firmware")
	wifiFWPath, _ := cmd.Flags().GetString("wifi-firmware")
	pmmFWPath, _ := cmd.Flags().GetString("pmm-firmware")
	all, _ := cmd.Flags().GetBool("all")
	workers, _ := cmd.Flags().GetInt("workers")

	if fwPath == "" && wifiFWPath == "" && pmmFWPath == "" {
		return fmt.Errorf("at least one of --firmware, --wifi-firmware, or --pmm-firmware is required")
	}

	var firmwares []fwEntry
	var fwDescParts []string
	for _, pair := range []struct{ path, target, label string }{
		{fwPath, fwTargetMain, "main"},
		{wifiFWPath, fwTargetWiFi, "wifi"},
		{pmmFWPath, fwTargetPMM, "pmm"},
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
		fwDescParts = append(fwDescParts, fmt.Sprintf("%s=%s", pair.label, filepath.Base(pair.path)))
	}
	firmwareDesc := joinComma(fwDescParts)

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

	locations := make([]string, 0, len(channelsToUpdate))
	for loc := range channelsToUpdate {
		locations = append(locations, loc)
	}
	sort.Strings(locations)

	bayForLoc := make(map[string]int, len(locations))
	bays := make([]int, 0, len(locations))
	for idx, loc := range locations {
		bay := idx + 1
		if locToBay != nil {
			if b, ok := locToBay[loc]; ok {
				bay = b
			}
		}
		bayForLoc[loc] = bay
		bays = append(bays, bay)
	}

	// Redirect slog to a file so it doesn't fight the TUI for stderr.
	logFile, err := os.Create(filepath.Join(folder, "fw-update-tui.log"))
	if err != nil {
		return fmt.Errorf("opening tui log file: %w", err)
	}
	defer logFile.Close()

	prevLog := appLog
	appLog = slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug}))
	defer func() { appLog = prevLog }()

	model := tui.NewFWModel(bays, appCfg.Env, firmwareDesc)
	program := tea.NewProgram(model)
	progressFn := tui.NewFWProgressFunc(program)

	var (
		wg       sync.WaitGroup
		sem      = make(chan struct{}, workers)
		resultMu sync.Mutex
	)

	go func() {
		for _, loc := range locations {
			ch := channelsToUpdate[loc]
			bay := bayForLoc[loc]
			wg.Add(1)
			sem <- struct{}{}
			go func(c *transport.SerialChannel, bayNum int) {
				defer wg.Done()
				defer func() { <-sem }()
				updateDeviceFirmware(commander, c, bayNum, firmwares, outputPath, &resultMu, progressFn)
			}(ch, bay)
		}
		wg.Wait()
		program.Send(tui.FWAllDoneMsg{})
	}()

	if _, err := program.Run(); err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}

	// If the user quit before workers finished, wait for them so we don't
	// pull the rug out from open serial channels.
	wg.Wait()

	fmt.Fprintf(cmd.OutOrStdout(), "Results written to %s\n", outputPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Log written to %s\n", filepath.Join(folder, "fw-update-tui.log"))
	return nil
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
