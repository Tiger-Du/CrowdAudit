package main

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/Tiger-Du/CrowdAudit/services/inference/internal/app"
)

var (
	once    sync.Once
	adapter *httpadapter.HandlerAdapter
	initErr error
)

func initOnce() {
	cfg, err := app.LoadConfigFromEnv()
	if err != nil {
		initErr = err
		return
	}

	// For Lambda: strongly recommend disabling outbox background publisher.
	// Enforce here too if you want:
	// Strongly recommended in Lambda:
	cfg.EnableOutbox = false

	built, err := app.Build(context.Background(), cfg)
	if err != nil {
		initErr = err
		return
	}

	var h http.Handler = built.Handler
	adapter = httpadapter.New(h)
	log.Printf("lambda init ok")
}

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	once.Do(initOnce)
	if initErr != nil {
		// Return a 500 with the init error rather than crashing the runtime repeatedly.
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Body:       "init error: " + initErr.Error(),
		}, nil
	}
	return adapter.ProxyWithContext(ctx, req)
}

func main() {
	lambda.Start(handler)
}
