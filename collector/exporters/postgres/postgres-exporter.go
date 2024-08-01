package postgres

import (
	"fmt"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/prometheus-community/postgres_exporter/collector"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
)

func RegisterPostgresCollector(registry *prometheus.Registry, secret map[string]interface{}, logger log.Logger) error {
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	// kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	dsn := fmt.Sprintf("postgresql://%s:%s@%s:%v/%s?sslmode=disable", secret["username"], secret["password"], secret["host"], secret["port"], secret["dbname"])

	pgCollector, err := collector.NewPostgresCollector(
		logger,
		[]string{}, // no custom queries
		dsn,
		[]string{}, // no exclude databases
	)
	if err != nil {
		fmt.Println("msg", "Failed to create PostgresCollector", "err", err.Error())
	} else {
		registry.MustRegister(pgCollector)
	}
	return nil
}
