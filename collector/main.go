package main

import (
	"bytes"
	"context"
	"database-collector/database/oracle"
	"database-collector/utils"
	"encoding/json"
	"fmt"
	"github.com/prometheus/common/expfmt"
	"os"
	"strings"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
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

var (
	defaultFileMetrics = kingpin.Flag(
		"default.metrics",
		"File with default metrics in a TOML file. (env: DEFAULT_METRICS)",
	).Default(getEnv("DEFAULT_METRICS", "")).String()
	customMetrics = kingpin.Flag(
		"custom.metrics",
		"File that may contain various custom metrics in a TOML file. (env: CUSTOM_METRICS)",
	).Default(getEnv("CUSTOM_METRICS", "")).String()
	queryTimeout = kingpin.Flag(
		"query.timeout",
		"Query timeout (in seconds). (env: QUERY_TIMEOUT)",
	).Default(getEnv("QUERY_TIMEOUT", "500")).Int()
	maxIdleConns = kingpin.Flag(
		"database.maxIdleConns",
		"Number of maximum idle connections in the connection pool. (env: DATABASE_MAXIDLECONNS)",
	).Default(getEnv("DATABASE_MAXIDLECONNS", "0")).Int()
	maxOpenConns = kingpin.Flag(
		"database.maxOpenConns",
		"Number of maximum open connections in the connection pool. (env: DATABASE_MAXOPENCONNS)",
	).Default(getEnv("DATABASE_MAXOPENCONNS", "10")).Int()
	toolkitFlags = webflag.AddFlags(kingpin.CommandLine, ":9161")
)

func oracleExporter(logger zerolog.Logger, dsn string, databaseIdentifier string) {
	logger.Info().Msg("Oracle Exporter Started")
	config := &oracle.Config{
		DSN:                dsn,
		MaxOpenConns:       *maxOpenConns,
		MaxIdleConns:       *maxIdleConns,
		CustomMetrics:      *customMetrics,
		QueryTimeout:       *queryTimeout,
		DefaultMetricsFile: *defaultFileMetrics,
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
		//data, err := serializeMetrics(metricFamilies) // You need to implement serializeMetrics.
		//if err != nil {
		//	logger.Error().Err(err).Msg("Failed to serialize metrics")
		//	return
		//}
		//
		//// Send serialized data to AMP.
		//err = utils.SendToAMP(data, ampEndpoint, region)
		//if err != nil {
		//	logger.Error().Err(err).Msg("Failed to send metrics to AMP")
		//}

		// Process gathered metrics. For example, log them.
		for _, mf := range metricFamilies {
			var writer bytes.Buffer
			encoder := expfmt.NewEncoder(&writer, expfmt.FmtText)
			err := encoder.Encode(mf)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to encode metric family")
				continue
			}
			logger.Info().Msg(writer.String())
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
	logger := initLogger()
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
					oracleExporter(logger, fmt.Sprintf(
						"oracle://%s:%s@%s:%d/%s",
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
	lambda.Start(HandleRequest)
}
