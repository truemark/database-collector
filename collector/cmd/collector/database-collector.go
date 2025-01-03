package main

import (
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	"github.com/robfig/cron/v3"
	"github.com/truemark/database-collector/exporters/mysql"
	"github.com/truemark/database-collector/exporters/oracle"
	"github.com/truemark/database-collector/exporters/postgres"
	"github.com/truemark/database-collector/internal/aws"
	"github.com/truemark/database-collector/internal/utils"
	"log/slog"
	"os"
	"sync"
)

func collectMetrics(secretValueMap map[string]interface{}, engine string, logger log.Logger, wg *sync.WaitGroup) {
	defer wg.Done()

	registry := prometheus.NewRegistry()
	switch engine {
	case "mysql":
		mysql.RegisterMySQLCollector(registry, secretValueMap, new(slog.Logger))
	case "postgres":
		postgres.RegisterPostgresCollector(registry, secretValueMap, new(slog.Logger))
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
		fmt.Println("Failed to send metrics to APS", err)
	} else {
		fmt.Println("Successfully sent metrics to APS ", response)
	}
}

func HandleRequest() {
	promlogConfig := &promlog.Config{
		Level: &promlog.AllowedLevel{},
	}
	if err := promlogConfig.Level.Set("info"); err != nil {
		fmt.Println("Error setting log level:", err)
		return
	}

	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", "Starting database collector")

	listSecretsResult := aws.ListSecrets()
	var wg sync.WaitGroup

	for i := 0; i < len(listSecretsResult.SecretList); i++ {
		secretValue := aws.GetSecretsValue(*listSecretsResult.SecretList[i].Name)
		secretValueMap := map[string]interface{}{}
		err := json.Unmarshal([]byte(secretValue), &secretValueMap)
		if err != nil {
			fmt.Println(err)
			continue
		}

		engine := secretValueMap["engine"].(string)
		wg.Add(1)
		go collectMetrics(secretValueMap, engine, logger, &wg)
	}

	wg.Wait()
}

func main() {
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
		_, err := c.AddFunc(cronSchedule, HandleRequest)
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
