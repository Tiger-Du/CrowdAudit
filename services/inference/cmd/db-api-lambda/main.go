package main

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

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

	// DB Lambda should never run background publisher loops.
	// Never run background loops in Lambda
	cfg.EnableOutbox = false

	built, err := app.Build(context.Background(), cfg)
	if err != nil {
		initErr = err
		return
	}

	var h http.Handler = built.Handler
	adapter = httpadapter.New(h)
	log.Printf("db-api lambda init ok")
}

func handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	once.Do(initOnce)
	if initErr != nil {
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
