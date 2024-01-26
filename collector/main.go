package main

import (
	"context"
	"database-collector/database/oracle"
	"database-collector/utils"
	"encoding/json"
	"fmt"
	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/rs/zerolog"
	_ "github.com/sijms/go-ora/v2"
	"os"
	"reflect"
	"time"
)

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
	).Default(getEnv("QUERY_TIMEOUT", "5")).Int()
	maxIdleConns = kingpin.Flag(
		"database.maxIdleConns",
		"Number of maximum idle connections in the connection pool. (env: DATABASE_MAXIDLECONNS)",
	).Default(getEnv("DATABASE_MAXIDLECONNS", "0")).Int()
	maxOpenConns = kingpin.Flag(
		"database.maxOpenConns",
		"Number of maximum open connections in the connection pool. (env: DATABASE_MAXOPENCONNS)",
	).Default(getEnv("DATABASE_MAXOPENCONNS", "10")).Int()
)

func oracleExporter(logger zerolog.Logger, dsn string) {
	logger.Info().Msg("Oracle Exporter Started")
	config := &oracle.Config{
		DSN:                dsn,
		MaxOpenConns:       *maxOpenConns,
		MaxIdleConns:       *maxIdleConns,
		CustomMetrics:      *customMetrics,
		QueryTimeout:       *queryTimeout,
		DefaultMetricsFile: *defaultFileMetrics,
	}
	_, err := oracle.NewExporter(logger, config)
	if err != nil {
		logger.Error().Err(err).Msg("Failed connecting to database")
	}
	if err != nil {
		logger.Error().Err(err).Msg("Unable to connect to databse")
	}
}

func HandleRequest(ctx context.Context) {
	promLogConfig := &promlog.Config{}
	logger := zerolog.New(
		zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339},
	).Level(zerolog.TraceLevel).With().Timestamp().Caller().Logger()
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
					fmt.Println(reflect.TypeOf(secretValue))
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
					))
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
	lambda.Start(HandleRequest)
}
