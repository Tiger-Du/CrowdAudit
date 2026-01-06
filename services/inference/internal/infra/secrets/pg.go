package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type pgSecret struct {
	URL string `json:"url"`
}

func FetchPGURLFromSecretsManager(ctx context.Context, secretARN string) (string, error) {
	if secretARN == "" {
		return "", errors.New("secret ARN is empty")
	}

	// Load AWS config (Lambda provides region & creds automatically)
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("load aws config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)

	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretARN),
	})
	if err != nil {
		return "", fmt.Errorf("get secret value: %w", err)
	}

	if out.SecretString == nil {
		return "", errors.New("secret has no SecretString")
	}

	var s pgSecret
	if err := json.Unmarshal([]byte(*out.SecretString), &s); err != nil {
		return "", fmt.Errorf("unmarshal secret json: %w", err)
	}

	if s.URL == "" {
		return "", errors.New("pg url missing in secret")
	}

	return s.URL, nil
}
