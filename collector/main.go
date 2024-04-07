package main

import (
	"context"
	"database-collector/database/oracle"
	"database-collector/utils"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	"os"
	"strings"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	_ "github.com/sijms/go-ora/v2"
)

func initLogger() zerolog.Logger {
	logLevel := strings.ToLower(getEnv("LOG_LEVEL", "info"))
	switch logLevel {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "fatal":
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	case "panic":
		zerolog.SetGlobalLevel(zerolog.PanicLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	return log.Logger
}

func oracleExporter(logger zerolog.Logger, dsn string, databaseIdentifier string) {
	logger.Info().Msg("Oracle Exporter Started")
	config := &oracle.Config{
		DSN:                dsn,
		MaxOpenConns:       10,
		MaxIdleConns:       0,
		CustomMetrics:      os.Getenv("CUSTOM_METRICS"),
		QueryTimeout:       500,
		DefaultMetricsFile: os.Getenv("DEFAULT_METRICS"),
		DatabaseIdentifier: databaseIdentifier,
	}
	exporter, err := oracle.NewExporter(logger, config)

	if err != nil {
		logger.Error().Err(err).Msg("Failed connecting to database")
	}
	if os.Getenv("EXPORTER_TYPE") == "prometheus" {
		registry := prometheus.NewRegistry()
		registry.MustRegister(exporter)
		gatherers := prometheus.Gatherers{
			prometheus.DefaultGatherer,
			registry,
		}
		metricFamilies, err := gatherers.Gather()
		if err != nil {
			logger.Error().Err(err).Msg("Failed to gather metrics")
			return
		}
		response, err := utils.ConvertMetricFamilyToTimeSeries(metricFamilies, databaseIdentifier)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to send metrics to APS")
		} else {
			logger.Info().Msg(fmt.Sprintf("Successfully sent metrics to APS %s", response))
		}
	}

}

func HandleRequest(ctx context.Context) {
	promLogConfig := &promlog.Config{}
	logger := initLogger()
	flag.AddFlags(kingpin.CommandLine, promLogConfig)
	kingpin.HelpFlag.Short('\n')
	kingpin.Version(version.Print("oracledb_exporter"))
	kingpin.Parse()
	logger.Info().Msg("Database collector started")

	// Get db details to log in
	listSecretsResult := utils.ListSecrets(logger)
	for i := 0; i < len(listSecretsResult.SecretList); i++ {
		for x := 0; x < len(listSecretsResult.SecretList[i].Tags); x++ {
			if *listSecretsResult.SecretList[i].Tags[x].Key == "database-collector:enabled" {
				if *listSecretsResult.SecretList[i].Tags[x].Value == "true" {
					secretValue := utils.GetSecretsValue(logger, listSecretsResult.SecretList[i].Name)
					secretValueMap := map[string]interface{}{}
					err := json.Unmarshal([]byte(secretValue), &secretValueMap)
					if err != nil {
						logger.Error().Err(err).Msg("Failed to unmarshal secret values")
						panic("Cannot proceed")
					}
					port, _ := secretValueMap["port"].(float64)
					logger.Info().Msg(fmt.Sprintf("Gathering metrics for database: %s", secretValueMap["host"]))
					oracleExporter(logger, fmt.Sprintf("oracle://%s:%s@%s:%d/%s",
						secretValueMap["username"],
						secretValueMap["password"],
						secretValueMap["host"],
						int(port),
						secretValueMap["dbname"],
					),
						secretValueMap["host"].(string))
				}
			}
		}
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	// USE THIS IF RUNNING LOCALLY
	//logger := initLogger()
	//listSecretsResult := utils.ListSecrets(logger)
	//for i := 0; i < len(listSecretsResult.SecretList); i++ {
	//	for x := 0; x < len(listSecretsResult.SecretList[i].Tags); x++ {
	//		if *listSecretsResult.SecretList[i].Tags[x].Key == "database-collector:enabled" {
	//			if *listSecretsResult.SecretList[i].Tags[x].Value == "true" {
	//				secretValue := utils.GetSecretsValue(logger, listSecretsResult.SecretList[i].Name)
	//				secretValueMap := map[string]interface{}{}
	//				err := json.Unmarshal([]byte(secretValue), &secretValueMap)
	//				if err != nil {
	//					logger.Error().Err(err).Msg("Failed to unmarshal secret values")
	//					panic("Cannot proceed")
	//				}
	//				fmt.Println(secretValueMap)
	//				port, _ := secretValueMap["port"].(float64)
	//				logger.Info().Msg(fmt.Sprintf("Gathering metrics for database: %s", secretValueMap["host"]))
	//				oracleExporter(logger, fmt.Sprintf(
	//					"oracle://%s:%s@%s:%d/%s",
	//					secretValueMap["username"],
	//					secretValueMap["password"],
	//					secretValueMap["host"],
	//					int(port),
	//					secretValueMap["dbname"],
	//				),
	//					secretValueMap["host"].(string))
	//			}
	//		}
	//	}
	//}
	lambda.Start(HandleRequest)
}
