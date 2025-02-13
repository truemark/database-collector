package oracle

import (
	"fmt"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/iamseth/oracledb_exporter/collector"
	"github.com/prometheus/client_golang/prometheus"
	_ "github.com/sijms/go-ora/v2"
)

func RegisterOracleDBCollector(registry *prometheus.Registry, secret map[string]interface{}, logger log.Logger) *collector.Exporter {
	level.Info(logger).Log("msg", "Registering OracleDB collector")
	dsn := fmt.Sprintf("oracle://%s:%s@%s:%v/%s?CONNECT_TIMEOUT=10", secret["username"], secret["password"], secret["host"], secret["port"], secret["dbname"])
	config := &collector.Config{
		DSN:                dsn,
		MaxOpenConns:       1,
		MaxIdleConns:       1,
		QueryTimeout:       10,
		DefaultMetricsFile: "",
		CustomMetrics:      "",
	}

	oracleExporter, err := collector.NewExporter(logger, config)
	if err != nil {
		level.Error(logger).Log("unable to connect to DB", err)
	}

	registry.MustRegister(oracleExporter)
	return oracleExporter
}
