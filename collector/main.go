package main

import (
	"context"
	"database-collector/database/oracle"
	"database-collector/utils"
	"fmt"
	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"github.com/rs/zerolog"
	_ "github.com/sijms/go-ora/v2"
	"os"
	"time"
)

var (
	// Version will be set at build time.
	Version            = "0.0.0.dev"
	metricPath         = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics. (env: TELEMETRY_PATH)").Default(getEnv("TELEMETRY_PATH", "/metrics")).String()
	defaultFileMetrics = kingpin.Flag(
		"default.metrics",
		"File with default metrics in a TOML file. (env: DEFAULT_METRICS)",
	).Default(getEnv("DEFAULT_METRICS", "default-metrics.toml")).String()
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
	scrapeInterval = kingpin.Flag(
		"scrape.interval",
		"Interval between each scrape. Default is to scrape on collect requests",
	).Default("0s").Duration()
	toolkitFlags = webflag.AddFlags(kingpin.CommandLine, ":9161")
)

type MyEvent struct {
	Name string `json:"name"`
}

func oracleExporter(logger zerolog.Logger) {
	logger.Info().Msg("Oracle Exporter Started")
	var (
		customMetrics = kingpin.Flag(
			"custom.metrics",
			"File that may contain various custom metrics in a TOML file. (env: CUSTOM_METRICS)",
		).Default(getEnv("CUSTOM_METRICS", "")).String()
	)

	config := oracle.Config{
		DSN:                os.Getenv("DATA_SOURCE_NAME"),
		MaxOpenConns:       *maxOpenConns,
		MaxIdleConns:       *maxIdleConns,
		CustomMetrics:      *customMetrics,
		QueryTimeout:       *queryTimeout,
		DefaultMetricsFile: *defaultFileMetrics,
	}
}

func HandleRequest(ctx context.Context, event *MyEvent) (*string, error) {
	if event == nil {
		return nil, fmt.Errorf("received nil event")
	}
	message := fmt.Sprintf("Hello %s!", event.Name)
	return &message, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	promLogConfig := &promlog.Config{}
	logger := zerolog.New(
		zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339},
	).Level(zerolog.TraceLevel).With().Timestamp().Caller().Logger()
	flag.AddFlags(kingpin.CommandLine, promLogConfig)
	kingpin.HelpFlag.Short('\n')
	kingpin.Version(version.Print("oracledb_exporter"))
	kingpin.Parse()
	logger.Info().Msg("Database collector started")
	dsn := os.Getenv("DATA_SOURCE_NAME")
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
	// TODO: ADD loop over secrets we get from secrets manager and get data for all databases.
	if err != nil {
		logger.Error().Err(err).Msg("Unable to connect to databse")
	}

	logger.Info().Msg(fmt.Sprintf("Starting database_exporter, version: %s", version.Info()))
	logger.Info().Msg(fmt.Sprintf("Build context, build: %s", version.BuildContext()))
	logger.Info().Msg(fmt.Sprintf("Collect from metricPath: %s", *metricPath))

	utils.ListSecrets(logger)
	lambda.Start(HandleRequest)
}
