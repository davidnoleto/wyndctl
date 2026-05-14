package config

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

var (
	secretCache   = make(map[string]map[string]interface{})
	secretCacheMu sync.Mutex
)

func resolveAWSSecret(reference string) (string, error) {
	parts := strings.SplitN(reference, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid secret reference %q: expected 'secret-name.key'", reference)
	}
	secretID := parts[0]
	key := parts[1]

	secretCacheMu.Lock()
	cached, hit := secretCache[secretID]
	secretCacheMu.Unlock()

	if hit {
		val, exists := cached[key]
		if !exists {
			return "", fmt.Errorf("key %q not found in secret %q", key, secretID)
		}
		return fmt.Sprintf("%v", val), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-west-2"))
	if err != nil {
		return "", fmt.Errorf("loading AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)
	result, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretID,
	})
	if err != nil {
		return "", fmt.Errorf("fetching secret %q: %w", secretID, err)
	}

	if result.SecretString == nil {
		return "", fmt.Errorf("secret %q has no string value", secretID)
	}

	var secretData map[string]interface{}
	if err := json.Unmarshal([]byte(*result.SecretString), &secretData); err != nil {
		return "", fmt.Errorf("parsing secret %q JSON: %w", secretID, err)
	}

	secretCacheMu.Lock()
	secretCache[secretID] = secretData
	secretCacheMu.Unlock()

	val, exists := secretData[key]
	if !exists {
		return "", fmt.Errorf("key %q not found in secret %q", key, secretID)
	}

	return fmt.Sprintf("%v", val), nil
}

func resolveDBFromSecrets(env string) (host string, port int, user string, password string, dbname string, err error) {
	secretName := fmt.Sprintf("wynd-%s-sentrydb", env)

	hostVal, err := resolveAWSSecret(secretName + ".host")
	if err != nil {
		return "", 0, "", "", "", fmt.Errorf("resolving DB host: %w", err)
	}

	portVal, err := resolveAWSSecret(secretName + ".port")
	if err != nil {
		return "", 0, "", "", "", fmt.Errorf("resolving DB port: %w", err)
	}

	userVal, err := resolveAWSSecret(secretName + ".username")
	if err != nil {
		return "", 0, "", "", "", fmt.Errorf("resolving DB user: %w", err)
	}

	passVal, err := resolveAWSSecret(secretName + ".password")
	if err != nil {
		return "", 0, "", "", "", fmt.Errorf("resolving DB password: %w", err)
	}

	dbnameVal := "sentry"
	if resolved, resolveErr := resolveAWSSecret(secretName + ".dbname"); resolveErr == nil {
		dbnameVal = resolved
	}

	p := 0
	fmt.Sscanf(portVal, "%d", &p)

	return hostVal, p, userVal, passVal, dbnameVal, nil
}
