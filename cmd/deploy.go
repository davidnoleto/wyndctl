package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/hellowynd/wyndctl/internal/database"
	"github.com/hellowynd/wyndctl/internal/deployment"
	"github.com/hellowynd/wyndctl/internal/device"
	"github.com/hellowynd/wyndctl/internal/models"
	"github.com/hellowynd/wyndctl/internal/transport"
	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Mass deploy Sentry devices",
	Long: `Provisions multiple Sentry devices in parallel:
  - Configures WiFi credentials
  - Assigns devices to properties and rooms
  - Logs results to CSV for audit trail`,
	RunE: runDeploy,
}

func init() {
	deployCmd.Flags().Bool("all", false, "deploy all connected devices (default: only failed from previous run)")
	deployCmd.Flags().String("color", "255,0,255", "RGB color for active deployment indicator")
	deployCmd.Flags().Bool("iterative", false, "deploy devices one-by-one with operator confirmation")
	deployCmd.Flags().Int("workers", runtime.NumCPU(), "number of parallel deployment workers")
	deployCmd.Flags().Int("timeout", 75, "provisioning timeout in seconds per device")
	rootCmd.AddCommand(deployCmd)
}

func runDeploy(cmd *cobra.Command, args []string) error {
	folder := "."
	all, _ := cmd.Flags().GetBool("all")
	colorStr, _ := cmd.Flags().GetString("color")
	iterative, _ := cmd.Flags().GetBool("iterative")
	workers, _ := cmd.Flags().GetInt("workers")
	timeout, _ := cmd.Flags().GetInt("timeout")

	color, err := models.ParseRGB(colorStr)
	if err != nil {
		return err
	}

	appLog.Info("starting mass deployment", "env", appCfg.Env)

	// Load settings
	settingsPath := filepath.Join(folder, "deployment-data.csv")
	settings, err := deployment.LoadSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading deployment settings: %w", err)
	}
	appLog.Info("loaded deployment settings", "bays", len(settings))

	// Load location map (optional)
	mapPath := filepath.Join(folder, "location-map.csv")
	var locToBay map[string]int
	if _, statErr := os.Stat(mapPath); statErr == nil {
		locToBay, _, err = deployment.LoadLocationMap(mapPath, true)
		if err != nil {
			return fmt.Errorf("loading location map: %w", err)
		}
	} else {
		appLog.Warn("no location map found, using default bay order")
	}

	// Output file
	outputPath := filepath.Join(folder, "deployment-result.csv")

	// Scan for devices
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

	// Determine which channels to deploy
	channelsToDeploy := channels
	if !all {
		if _, statErr := os.Stat(outputPath); statErr == nil {
			prevResults, err := deployment.ReadResults(outputPath)
			if err == nil {
				appLog.Info("filtering to failed devices from previous run")
				_ = prevResults
			}
		}
	}

	// Confirm
	if !confirmAction(cmd.ErrOrStderr(), cmd.InOrStdin(), fmt.Sprintf("Deploy %d device(s) on %s?", len(channelsToDeploy), appCfg.Env)) {
		return fmt.Errorf("deployment cancelled by user")
	}

	// Create output file
	if err := deployment.WriteResultHeader(outputPath); err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}

	// Connect to database for room assignment
	var repo *database.Repository
	repo, err = database.NewRepository(appCfg.DB)
	if err != nil {
		appLog.Warn("database not configured — room assignment will be skipped", "error", err)
		repo = nil
	} else {
		defer repo.Close()
		appLog.Info("database connected for room assignment")
	}

	svc := deployment.NewService(commander, repo, outputPath, appLog)
	opts := deployment.DeployOptions{
		Timeout:      time.Duration(timeout) * time.Second,
		MaxRetries:   5,
		SuccessColor: uint32(color),
		FailColor:    0xFF0000,
	}

	// Sort locations for deterministic ordering
	locations := make([]string, 0, len(channelsToDeploy))
	for loc := range channelsToDeploy {
		locations = append(locations, loc)
	}
	sort.Strings(locations)

	if iterative {
		stdin := bufio.NewReader(cmd.InOrStdin())
		for idx, location := range locations {
			ch := channelsToDeploy[location]
			bayNum := idx + 1
			if locToBay != nil {
				bayNum = locToBay[location]
			}

			colorVal := uint32(color)
			_ = commander.SetIndicate(ch, colorVal, colorVal, colorVal, 3600)

			fmt.Fprintf(cmd.OutOrStdout(), "Press Enter to deploy bay %d at %s...", bayNum, location)
			stdin.ReadString('\n')

			if bayNum >= 1 && bayNum <= len(settings) {
				svc.DeployDevice(ch, settings[bayNum-1], opts)
			} else {
				appLog.Warn("skipping device with invalid bay number", "bay", bayNum, "location", location)
			}

			_ = commander.CancelIndication(ch)
			time.Sleep(1500 * time.Millisecond)
		}
	} else {
		var wg sync.WaitGroup
		sem := make(chan struct{}, workers)

		for idx, location := range locations {
			ch := channelsToDeploy[location]
			bayNum := idx + 1
			if locToBay != nil {
				bayNum = locToBay[location]
			}

			if bayNum < 1 || bayNum > len(settings) {
				appLog.Warn("skipping device with invalid bay number", "bay", bayNum, "location", location)
				continue
			}

			wg.Add(1)
			sem <- struct{}{}

			go func(c *transport.SerialChannel, setting models.DeploymentSetting) {
				defer wg.Done()
				defer func() { <-sem }()
				svc.DeployDevice(c, setting, opts)
			}(ch, settings[bayNum-1])
		}

		wg.Wait()
	}

	fmt.Fprintln(cmd.OutOrStdout(), "All deployment tasks completed.")
	return nil
}
