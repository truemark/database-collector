package main

import (
	"context"
	"database-collector/utils"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/rs/zerolog"
	"os"
	"time"
)

type MyEvent struct {
	Name string `json:"name"`
}

func HandleRequest(ctx context.Context, event *MyEvent) (*string, error) {
	if event == nil {
		return nil, fmt.Errorf("received nil event")
	}
	message := fmt.Sprintf("Hello %s!", event.Name)
	return &message, nil
}

func main() {
	logger := zerolog.New(
		zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339},
	).Level(zerolog.TraceLevel).With().Timestamp().Caller().Logger()

	logger.Info().Msg("Database collector started")
	utils.ListSecrets(logger)
	lambda.Start(HandleRequest)
}
