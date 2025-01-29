package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	cron "github.com/robfig/cron/v3"
	"github.com/truemark/database-collector/exporters/mysql"
	"github.com/truemark/database-collector/exporters/oracle"
	"github.com/truemark/database-collector/exporters/postgres"
	"github.com/truemark/database-collector/internal/aws"
	"github.com/truemark/database-collector/internal/utils"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func collectMetrics(ctx context.Context, secretValueMap map[string]interface{}, engine string, logger log.Logger, wg *sync.WaitGroup) {
	defer wg.Done()

	registry := prometheus.NewRegistry()
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Adjust log level as needed
	})
	slogLogger := slog.New(handler)

	switch engine {
	case "mysql":
		mysql.RegisterMySQLCollector(registry, secretValueMap, slogLogger)
	case "postgres":
		postgres.RegisterPostgresCollector(registry, secretValueMap, slogLogger)
	case "oracle", "oracle-ee":
		oracle.RegisterOracleDBCollector(registry, secretValueMap, logger)
	}

	metricFamilies, err := registry.Gather()
	if err != nil {
		level.Error(logger).Log("msg", "Error gathering metrics", "err", err)
		return
	}

	response, err := utils.ConvertMetricFamilyToTimeSeries(metricFamilies, secretValueMap["host"].(string))
	if err != nil {
		level.Error(logger).Log("msg", "Failed to send metrics to APS", "err", err)
	} else {
		level.Info(logger).Log("msg", "Successfully sent metrics to APS", "response", response)
	}
}

func HandleRequest(ctx context.Context) {
	promlogConfig := &promlog.Config{
		Level: &promlog.AllowedLevel{},
	}

	logger := promlog.New(promlogConfig)
	level.Info(logger).Log("msg", "Starting database collector")

	listSecretsResult := aws.ListSecrets()

	var wg sync.WaitGroup
	var sem = make(chan struct{}, 10) // Limit to 10 concurrent goroutines

	for i := 0; i < len(listSecretsResult.SecretList); i++ {
		secretValue := aws.GetSecretsValue(*listSecretsResult.SecretList[i].Name)

		secretValueMap := map[string]interface{}{}
		if err := json.Unmarshal([]byte(secretValue), &secretValueMap); err != nil {
			level.Error(logger).Log("msg", "Failed to unmarshal secret value", "secret", *listSecretsResult.SecretList[i].Name, "err", err)
			continue
		}

		engine := secretValueMap["engine"].(string)
		wg.Add(1)
		sem <- struct{}{} // Acquire a slot
		go func(secretValueMap map[string]interface{}, engine string) {
			defer wg.Done()
			defer func() { <-sem }() // Release the slot
			collectMetrics(ctx, secretValueMap, engine, logger, &wg)
		}(secretValueMap, engine)
	}

	wg.Wait()
	level.Info(logger).Log("msg", "All goroutines have completed")
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupts
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		cancel() // Cancel the context on interrupt
	}()

	mode := os.Getenv("RUN_MODE")
	if mode == "LAMBDA" {
		// Run as AWS Lambda function
		lambda.Start(HandleRequest)
	} else if mode == "CRON" {
		fmt.Println("Starting in CRON mode...")

		// Run as internal cron job
		c := cron.New()
		cronSchedule := os.Getenv("CRON_SCHEDULE")
		if cronSchedule == "" {
			cronSchedule = "@every 5m"
		}
		_, err := c.AddFunc(cronSchedule, func() {
			HandleRequest(ctx)
		})
		if err != nil {
			fmt.Println("Error setting up cron job:", err)
			return
		}
		c.Start()

		// Keep the program running
		select {}
	} else {
		fmt.Println("Invalid RUN_MODE. Set RUN_MODE to either 'LAMBDA' or 'CRON'")
	}
}
