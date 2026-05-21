// Package config provides application configuration via Viper.
package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	Env     string        `mapstructure:"env"`
	MQTT    MQTTConfig    `mapstructure:"mqtt"`
	USB     USBConfig     `mapstructure:"usb"`
	DB      DBConfig      `mapstructure:"db"`
	Log     LogConfig     `mapstructure:"log"`
	Cognito CognitoConfig `mapstructure:"cognito"`
	Stripe  StripeConfig  `mapstructure:"stripe"`
}

// CognitoConfig holds AWS Cognito user pool settings used for account signup.
type CognitoConfig struct {
	AppClientID string `mapstructure:"app_client_id"`
	Region      string `mapstructure:"region"`
}

// StripeConfig holds Stripe API credentials used for billing customer creation.
type StripeConfig struct {
	APIKey string `mapstructure:"api_key"`
}

// MQTTConfig holds AWS IoT / MQTT broker settings.
type MQTTConfig struct {
	BrokerURL       string `mapstructure:"broker_url"`
	Port            int    `mapstructure:"port"`
	TopicPattern    string `mapstructure:"topic_pattern"`
	ThingsPattern   string `mapstructure:"things_pattern"`
	LogTopicPattern string `mapstructure:"log_topic_pattern"`
	IoTPolicyName   string `mapstructure:"iot_policy_name"`
}

// USBConfig holds USB serial device identification settings.
type USBConfig struct {
	VendorID  uint16 `mapstructure:"vendor_id"`
	ProductID uint16 `mapstructure:"product_id"`
	BaudRate  int    `mapstructure:"baud_rate"`
}

// DBConfig holds database connection settings.
type DBConfig struct {
	DSN      string `mapstructure:"dsn"`
	Driver   string `mapstructure:"driver"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

// BuildDSN returns a connection string from either the DSN field or individual PG_* fields.
func (c *DBConfig) BuildDSN() string {
	if c.DSN != "" {
		return c.DSN
	}
	if c.Host == "" {
		return ""
	}
	sslmode := c.SSLMode
	if sslmode == "" {
		sslmode = "require"
	}
	port := c.Port
	if port == 0 {
		port = 5432
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   fmt.Sprintf("%s:%d", c.Host, port),
		Path:   "/" + c.DBName,
		RawQuery: url.Values{"sslmode": {sslmode}}.Encode(),
	}
	return u.String()
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// DefaultConfig returns the default configuration values.
func DefaultConfig() *Config {
	return &Config{
		Env: "dev",
		MQTT: MQTTConfig{
			BrokerURL:       "a2dfrytx410fez-ats.iot.us-west-2.amazonaws.com",
			Port:            8883,
			TopicPattern:    "wynd/%s/sentry/{name}/data/air-quality",
			ThingsPattern:   "$aws/things/{name}",
			LogTopicPattern: "wynd/%s/data/aq_device_error_logs",
		},
		USB: USBConfig{
			VendorID:  0x2fe3,
			ProductID: 0x0100,
			BaudRate:  9600,
		},
		DB: DBConfig{
			Driver: "postgres",
			DBName: "sentry",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
		Cognito: CognitoConfig{
			Region: "us-west-2",
		},
	}
}

func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	return scanner.Err()
}

func findAndLoadEnvFile(env string) {
	filename := env + ".env"

	searchPaths := []string{
		filepath.Join(".", "configs", filename),
		filepath.Join("..", "configs", filename),
		filepath.Join("..", "..", "configs", filename),
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		searchPaths = append(searchPaths,
			filepath.Join(home, "code", "wynd-sentry", "backend", "configs", filename),
		)
	}

	for _, p := range searchPaths {
		if _, err := os.Stat(p); err == nil {
			_ = loadEnvFile(p)
			return
		}
	}
}

// Load reads configuration from file, environment variables, and merges defaults.
func Load(cfgFile string) (*Config, error) {
	setDefaults()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, _ := os.UserHomeDir()
		viper.AddConfigPath(".")
		viper.AddConfigPath(home)
		viper.AddConfigPath("/etc/wynd")
		viper.SetConfigName("wyndctl")
		viper.SetConfigType("yaml")
	}

	viper.SetEnvPrefix("WYND")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			if cfgFile != "" {
				return nil, fmt.Errorf("reading config file: %w", err)
			}
			fmt.Fprintf(os.Stderr, "warning: ignoring invalid config file: %v\n", err)
		}
	}

	env := viper.GetString("env")
	findAndLoadEnvFile(env)
	mapPGEnvVars()
	mapIOTEnvVars()
	mapAuthEnvVars()

	cfg := DefaultConfig()
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	if cfg.Stripe.APIKey != "" && !strings.HasPrefix(cfg.Stripe.APIKey, "sk_") {
		if resolved, err := resolveAWSRawSecret(cfg.Stripe.APIKey); err == nil {
			cfg.Stripe.APIKey = resolved
		} else {
			fmt.Fprintf(os.Stderr, "warning: could not resolve Stripe API key secret %q: %v\n", cfg.Stripe.APIKey, err)
		}
	}

	if cfg.DB.Host == "" || isSecretReference(cfg.DB.Host) {
		if cfg.Env == "dev" || cfg.Env == "prod" {
			fmt.Fprintf(os.Stderr, "Resolving database credentials from AWS Secrets Manager (wynd-%s-sentrydb)...\n", cfg.Env)
			host, port, user, password, dbname, err := resolveDBFromSecrets(cfg.Env)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not resolve DB secrets: %v\n", err)
			} else {
				cfg.DB.Host = host
				cfg.DB.Port = port
				cfg.DB.User = user
				cfg.DB.Password = password
				cfg.DB.DBName = dbname
				fmt.Fprintf(os.Stderr, "Database credentials resolved (host=%s, port=%d, user=%s, dbname=%s)\n",
					host, port, user, dbname)
			}
		}
	}

	return cfg, nil
}

func isSecretReference(val string) bool {
	return strings.Contains(val, "sentrydb.") || strings.Contains(val, "sentry-db.")
}

func mapPGEnvVars() {
	stringMappings := map[string]string{
		"PG_HOST":     "db.host",
		"PG_USER":     "db.user",
		"PG_PASSWORD": "db.password",
		"PG_DBNAME":   "db.dbname",
	}
	for envKey, viperKey := range stringMappings {
		if val := os.Getenv(envKey); val != "" && !isSecretReference(val) {
			viper.Set(viperKey, val)
		}
	}

	if val := os.Getenv("PG_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			viper.Set("db.port", port)
		}
	}
}

func mapIOTEnvVars() {
	if val := os.Getenv("IOT_ENDPOINT_URL"); val != "" {
		val = strings.TrimPrefix(val, "https://")
		val = strings.TrimPrefix(val, "http://")
		viper.Set("mqtt.broker_url", val)
	}
	if val := os.Getenv("IOT_MQTT_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			viper.Set("mqtt.port", port)
		}
	}
	if val := os.Getenv("IOT_POLICY_NAME"); val != "" {
		viper.Set("mqtt.iot_policy_name", val)
	}
}

func mapAuthEnvVars() {
	if val := os.Getenv("APP_CLIENT_ID"); val != "" {
		viper.Set("cognito.app_client_id", val)
	}
	if val := os.Getenv("STRIPE_API_KEY"); val != "" {
		viper.Set("stripe.api_key", val)
	}
}

func setDefaults() {
	d := DefaultConfig()

	viper.SetDefault("env", d.Env)
	viper.SetDefault("mqtt.broker_url", d.MQTT.BrokerURL)
	viper.SetDefault("mqtt.port", d.MQTT.Port)
	viper.SetDefault("mqtt.topic_pattern", d.MQTT.TopicPattern)
	viper.SetDefault("mqtt.things_pattern", d.MQTT.ThingsPattern)
	viper.SetDefault("mqtt.log_topic_pattern", d.MQTT.LogTopicPattern)
	viper.SetDefault("usb.vendor_id", d.USB.VendorID)
	viper.SetDefault("usb.product_id", d.USB.ProductID)
	viper.SetDefault("usb.baud_rate", d.USB.BaudRate)
	viper.SetDefault("db.driver", d.DB.Driver)
	viper.SetDefault("db.dbname", d.DB.DBName)
	viper.SetDefault("log.level", d.Log.Level)
	viper.SetDefault("log.format", d.Log.Format)
	viper.SetDefault("cognito.region", d.Cognito.Region)
}
