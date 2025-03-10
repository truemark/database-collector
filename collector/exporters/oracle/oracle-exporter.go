package oracle

import (
	"fmt"
	_ "github.com/godror/godror"
	"github.com/oracle/oracle-db-appdev-monitoring/collector"
	"github.com/prometheus/client_golang/prometheus"
	_ "github.com/sijms/go-ora/v2"
	"log/slog"
)

func RegisterOracleDBCollector(registry *prometheus.Registry, secret map[string]interface{}, logger *slog.Logger) *collector.Exporter {
	logger.Info("msg", "Registering OracleDB collector")
	dsn := fmt.Sprintf("%s:%v/%s", secret["host"], secret["port"], secret["dbname"])
	config := &collector.Config{
		User:               secret["username"].(string),
		Password:           secret["password"].(string),
		ConnectString:      dsn,
		MaxOpenConns:       1,
		MaxIdleConns:       1,
		QueryTimeout:       10,
		DefaultMetricsFile: "",
		CustomMetrics:      "/Volumes/code/truemark/database-collector/collector/exporters/oracle/custom-metrics.toml",
	}

	oracleExporter, err := collector.NewExporter(logger, config)
	if err != nil {
		logger.Error("unable to connect to DB", err)
	}

	registry.MustRegister(oracleExporter)
	return oracleExporter
}
