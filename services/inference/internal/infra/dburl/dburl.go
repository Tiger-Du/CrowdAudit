package dburl

import (
	"context"
	"errors"
	"os"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/infra/secrets"
)

func Load(ctx context.Context) (string, error) {
	// Best: Secrets Manager in AWS
	if arn := os.Getenv("PG_URL_SECRET_ARN"); arn != "" {
		return secrets.FetchPGURLFromSecretsManager(ctx, arn)
	}

	// Backwards-compatible env vars
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v, nil
	}
	if v := os.Getenv("PG_URL"); v != "" {
		return v, nil
	}

	return "", errors.New("missing DB url: set PG_URL_SECRET_ARN or DATABASE_URL or PG_URL")
}
