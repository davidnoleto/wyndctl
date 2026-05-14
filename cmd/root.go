package cmd

import (
	"fmt"
	"log/slog"

	"github.com/hellowynd/wyndctl/internal/config"
	"github.com/hellowynd/wyndctl/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// BuildMetadata holds compile-time info populated from main.go.
type BuildMetadata struct {
	Version   string
	BuildTime string
	GitSHA    string
	GitDirty  bool
}

func (b BuildMetadata) String() string {
	dirty := ""
	if b.GitDirty {
		dirty = " (dirty)"
	}
	if b.BuildTime != "" {
		return fmt.Sprintf("%s sha=%s%s built=%s", b.Version, b.GitSHA, dirty, b.BuildTime)
	}
	return fmt.Sprintf("%s sha=%s%s", b.Version, b.GitSHA, dirty)
}

var (
	cfgFile string
	appCfg  *config.Config
	appLog  *slog.Logger
	// BuildInfo is set by main.go from -ldflags. Available to all commands.
	BuildInfo BuildMetadata
)

var rootCmd = &cobra.Command{
	Use:     "wyndctl",
	Version: "filled in init()",
	Short:   "Wynd Sentry device CLI",
	Long: `CLI for scanning and deploying Wynd Sentry IoT air-quality devices over USB serial.

Configuration can be provided via:
  - Config file (wyndctl.yaml)
  - Environment variables (WYND_* or PG_*/IOT_*/ENV for Python CLI compat)
  - Sentry backend .env files (auto-discovered from configs/{env}.env)
  - CLI flags`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		appCfg = cfg
		appLog = logger.New(cfg.Log)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./wyndctl.yaml)")
	rootCmd.PersistentFlags().String("env", "dev", "deployment environment (dev, staging, prod)")
	rootCmd.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("log-format", "text", "log format (text, json)")
	_ = viper.BindPFlag("env", rootCmd.PersistentFlags().Lookup("env"))
	_ = viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))
	_ = viper.BindPFlag("log.format", rootCmd.PersistentFlags().Lookup("log-format"))
	_ = viper.BindEnv("env", "ENV")
}

// Execute runs the root command.
func Execute() error {
	rootCmd.Version = BuildInfo.String()
	return rootCmd.Execute()
}
