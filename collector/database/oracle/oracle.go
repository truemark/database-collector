package oracle

import (
	"context"
	"database-collector/utils"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/prometheus/client_golang/prometheus"
)

type Exporter struct {
	config          *Config
	mu              *sync.Mutex
	metricsToScrape Metrics
	scrapeInterval  *time.Duration
	dsn             string
	duration, error prometheus.Gauge
	totalScrapes    prometheus.Counter
	scrapeErrors    *prometheus.CounterVec
	scrapeResults   []prometheus.Metric
	up              prometheus.Gauge
	db              *sql.DB
	logger          zerolog.Logger
}

// Config is the configuration of the exporter
type Config struct {
	DSN                string
	MaxIdleConns       int
	MaxOpenConns       int
	CustomMetrics      string
	QueryTimeout       int
	DefaultMetricsFile string
	DatabaseIdentifier string
}

// Metric is an object description
type Metric struct {
	Context          string
	Labels           []string
	MetricsDesc      map[string]string
	CloudwatchType   map[string]string
	MetricsType      map[string]string
	MetricsBuckets   map[string]map[string]string
	FieldToAppend    string
	Request          string
	IgnoreZeroResult bool
}

// Metrics is a container structure for prometheus metrics
type Metrics struct {
	Metric []Metric
}

var (
	additionalMetrics Metrics
	hashMap           = make(map[int][]byte)
	namespace         = "oracledb"
	exporterName      = "exporter"
)

func NewExporter(logger zerolog.Logger, cfg *Config) (*Exporter, error) {
	e := &Exporter{
		mu:  &sync.Mutex{},
		dsn: cfg.DSN,
		duration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "last_scrape_duration_seconds",
			Help:      "Duration of the last scrape of metrics from Oracle DB.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "scrapes_total",
			Help:      "Total number of times Oracle DB was scraped for metrics.",
		}),
		scrapeErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "scrape_errors_total",
			Help:      "Total number of times an error occurred scraping a Oracle database.",
		}, []string{"collector"}),
		error: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: exporterName,
			Name:      "last_scrape_error",
			Help:      "Whether the last scrape of metrics from Oracle DB resulted in an error (1 for error, 0 for success).",
		}),
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Whether the Oracle database server is up.",
		}),
		logger: logger,
		config: cfg,
	}
	e.metricsToScrape = e.DefaultMetrics(logger)
	err := e.connect()
	return e, err
}

func (e *Exporter) generateCloudwatchMetrics(context string, labelsMap map[string]string, metric string, value float64, metricType map[string]string) error {
	var dimensions []utils.Dimension
	dimensions = append(dimensions, utils.Dimension{
		Name:  "DatabaseIdentifier",
		Value: e.config.DatabaseIdentifier,
	})
	for label, labelValue := range labelsMap {
		dimensions = append(dimensions, utils.Dimension{
			Name:  label,
			Value: labelValue,
		})
	}
	e.logger.Info().Msg(fmt.Sprintf("Preparing to push Metric '%s': %f, Dimensions: %v, metricType: %s", metric, value, dimensions, metricType[metric]))
	//err := utils.PutCloudwatchMetrics(e.logger, utils.MetricDataInput{
	//	Namespace: namespace,
	//	MetricData: []utils.MetricDatum{
	//		{
	//			MetricName: fmt.Sprintf("%s_%s", context, metric),
	//			Unit:       metricType[metric], //How to sort unit types to proper cloudwatch units
	//			Value:      value,
	//			Dimensions: dimensions,
	//		},
	//	},
	//})
	//if err != nil {
	//	e.logger.Error().Err(err).Msg("Failed to push metric to CloudWatch")
	//} else {
	//	e.logger.Info().Msg(fmt.Sprintf("Success push Metric '%s': %f, Dimensions: %v", metric, value, dimensions))
	//}
	e.logger.Info().Msg(fmt.Sprintf("Success push Metric '%s': %f, Dimensions: %v", metric, value, dimensions))
	return nil
}

func (e *Exporter) generateMetrics(db *sql.DB, parse func(row map[string]string) error, query string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.config.QueryTimeout)*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("oracle query timed out")
	}

	if err != nil {
		return err
	}
	cols, err := rows.Columns()
	defer rows.Close()

	for rows.Next() {
		// Create a slice of interface{}'s to represent each column,
		// and a second slice to contain pointers to each item in the columns slice.
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		// Scan the result into the column pointers...
		if err := rows.Scan(columnPointers...); err != nil {
			return err
		}

		// Create our map, and retrieve the value for each column from the pointers slice,
		// storing it in the map with the name of the column as the key.
		m := make(map[string]string)
		for i, colName := range cols {
			val := columnPointers[i].(*interface{})
			m[strings.ToLower(colName)] = fmt.Sprintf("%v", *val)
		}
		// Call function to parse row
		if err := parse(m); err != nil {
			return err
		}
	}
	return nil
}
