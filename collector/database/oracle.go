package database

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database-collector/utils"
	"database/sql"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
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
}

// CreateDefaultConfig returns the default configuration of the Exporter
// it is to be of note that the DNS will be empty when
func CreateDefaultConfig() *Config {
	return &Config{
		MaxIdleConns:       0,
		MaxOpenConns:       10,
		CustomMetrics:      "",
		QueryTimeout:       5,
		DefaultMetricsFile: "",
	}
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

func maskDsn(dsn string) string {
	parts := strings.Split(dsn, "@")
	if len(parts) > 1 {
		maskedURL := "***@" + parts[1]
		return maskedURL
	}
	fmt.Println(dsn)
	return dsn
}

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
	err := e.connect(logger)
	return e, err
}

func (e *Exporter) connect(logger zerolog.Logger) error {
	logger.Debug().Msg(fmt.Sprintf("launching connection: %s", maskDsn(e.dsn)))
	db, err := sql.Open("oracle", e.dsn)
	if err != nil {
		logger.Debug().Msg(fmt.Sprintf("error while connecting to: %s", maskDsn(e.dsn)))
		return err
	}
	logger.Debug().Msg(fmt.Sprintf("set max idle connections to %d", e.config.MaxIdleConns))
	db.SetMaxIdleConns(e.config.MaxIdleConns)
	logger.Debug().Msg(fmt.Sprintf("set max open connections to %d", e.config.MaxOpenConns))
	db.SetMaxOpenConns(e.config.MaxOpenConns)
	logger.Debug().Msg(fmt.Sprintf("successfully connected to: %s", maskDsn(e.dsn)))
	e.db = db
	e.scrape(logger)
	return nil
}

func hashFile(h hash.Hash, fn string) error {
	f, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	return nil
}

func (e *Exporter) checkIfMetricsChanged(logger zerolog.Logger) bool {
	for i, _customMetrics := range strings.Split(e.config.CustomMetrics, ",") {
		if len(_customMetrics) == 0 {
			continue
		}
		logger.Debug().Msg(fmt.Sprintf("checking modifications in following metrics definition file: %s", _customMetrics))
		h := sha256.New()
		if err := hashFile(h, _customMetrics); err != nil {
			logger.Error().Err(errors.New(err.Error())).Msg("Unable to get file has.")
			return false
		}
		// If any of files has been changed reload metrics
		if !bytes.Equal(hashMap[i], h.Sum(nil)) {
			logger.Info().Msg(fmt.Sprintf("%s has been changed. Reloading metrics", _customMetrics))
			hashMap[i] = h.Sum(nil)
			return true
		}
	}
	return false
}

func (e *Exporter) reloadMetrics(logger zerolog.Logger) {
	// Truncate metricsToScrape
	e.metricsToScrape.Metric = []Metric{}

	// Load default metrics
	defaultMetrics := e.DefaultMetrics(logger)
	e.metricsToScrape.Metric = defaultMetrics.Metric

	// If custom metrics, load it
	if strings.Compare(e.config.CustomMetrics, "") != 0 {
		for _, _customMetrics := range strings.Split(e.config.CustomMetrics, ",") {
			if _, err := toml.DecodeFile(_customMetrics, &additionalMetrics); err != nil {
				logger.Error().Err(errors.New(err.Error()))
				panic(errors.New("Error while loading " + _customMetrics))
			} else {
				logger.Info().Msg(fmt.Sprintf("Successfully loaded custom metrics from: %s", _customMetrics))
			}
			e.metricsToScrape.Metric = append(e.metricsToScrape.Metric, additionalMetrics.Metric...)
			fmt.Println(e.metricsToScrape)
		}
	} else {
		logger.Debug().Msg("No custom metrics defined")
	}
}

func (e *Exporter) scrape(logger zerolog.Logger) {
	var err error
	if err = e.db.Ping(); err != nil {
		if strings.Contains(err.Error(), "sql: database is closed") {
			logger.Info().Msg("Reconnecting to DB")
			err = e.connect(logger)
			if err != nil {
				logger.Error().Err(errors.New(err.Error())).Msg("Error reconnecting to DB")
			}
		}
	}
	if err = e.db.Ping(); err != nil {
		logger.Error().Err(errors.New(err.Error())).Msg("Error pinging oracle.")
		e.up.Set(0)
		return
	}

	logger.Info().Msg(fmt.Sprintf("Successfully pinged oracle database: %s", maskDsn(e.dsn)))
	e.up.Set(1)

	if e.checkIfMetricsChanged(logger) {
		e.reloadMetrics(logger)
	}
	wg := sync.WaitGroup{}
	for _, metric := range e.metricsToScrape.Metric {
		wg.Add(1)
		metric := metric
		f := func() {
			defer wg.Done()

			logger.Debug().Msg("About to scrape metric: ")
			logger.Debug().Msg(fmt.Sprintf("- Metric MetricsDesc: %s", metric.MetricsDesc))
			logger.Debug().Msg(fmt.Sprintf("- Metric Context: %s", metric.Context))
			logger.Debug().Msg(fmt.Sprintf("- Metric MetricsType: %s", metric.MetricsType))
			logger.Debug().Msg(fmt.Sprintf("- Metric MetricsBuckets: %s \"(Ignored unless Histogram type)\"", metric.MetricsBuckets))
			logger.Debug().Msg(fmt.Sprintf("- Metric Labels: %s", metric.Labels))
			logger.Debug().Msg(fmt.Sprintf("- Metric FieldToAppend: %s", metric.FieldToAppend))
			logger.Debug().Msg(fmt.Sprintf("- Metric IgnoreZeroResult: %t", metric.IgnoreZeroResult))
			logger.Debug().Msg(fmt.Sprintf("- Metric Request: %s", metric.Request))

			if len(metric.Request) == 0 {
				logger.Error().Msg(fmt.Sprintf("Error scraping for %s. Did you forget to define request in your toml file? ", metric.MetricsDesc))
				return
			}

			if len(metric.MetricsDesc) == 0 {
				logger.Error().Msg(fmt.Sprintf("Error scraping for query %s. Did you forget to define metricsdesc  in your toml file?", metric.Request))
				return
			}

			for column, metricType := range metric.MetricsType {
				if metricType == "histogram" {
					_, ok := metric.MetricsBuckets[column]
					if !ok {
						logger.Error().Msg(fmt.Sprintf("Unable to find MetricsBuckets configuration key for metric. (metric=\" %s \")", column))
						return
					}
				}
			}
			scrapeStart := time.Now()
			fmt.Println("Starting scrape for metrics: ")
			if err = e.ScrapeMetric(e.db, logger, metric); err != nil {
				logger.Error().Err(errors.New(err.Error())).Msg(fmt.Sprintf("error scraping for %s_%s, %s", metric.Context, metric.MetricsDesc, time.Since(scrapeStart)))
				e.scrapeErrors.WithLabelValues(metric.Context).Inc()
			} else {
				logger.Info().Msg(fmt.Sprintf("successfully scraped metric: %s %s %s", metric.Context, metric.MetricsDesc, time.Since(scrapeStart)))
			}
		}
		go f()
	}
	wg.Wait()
}

func (e *Exporter) ScrapeMetric(db *sql.DB, logger zerolog.Logger, metricDefinition Metric) error {
	logger.Info().Msg("calling function ScrapeGenericValues()")
	//return e.generatePrometheusMetrics(db, logger, metricDefinition.Request)
	fmt.Println("MetricsDefinition: ", metricDefinition)
	return e.scrapeGenericValues(db, logger, metricDefinition.Context, metricDefinition.Labels,
		metricDefinition.MetricsDesc, metricDefinition.MetricsType, metricDefinition.MetricsBuckets,
		metricDefinition.FieldToAppend, metricDefinition.IgnoreZeroResult,
		metricDefinition.Request, metricDefinition.CloudwatchType)
}

func (e *Exporter) scrapeGenericValues(db *sql.DB, logger zerolog.Logger, context string, labels []string,
	metricsDesc map[string]string, metricsType map[string]string, metricsBuckets map[string]map[string]string, fieldToAppend string, ignoreZeroResult bool, request string, cloudWatchType map[string]string) error {
	metricsCount := 0
	genericParser := func(row map[string]string) error {
		// Construct labels value
		labelsValues := make([]string, len(labels))
		labelsMap := make(map[string]string, len(labels))
		for i, label := range labels {
			value, exists := row[label]
			if !exists {
				logger.Error().Msg(fmt.Sprintf("Label '%s' does not exist in the row", label))
				continue
			}
			labelsValues[i] = value
			labelsMap[label] = value
		}

		// Process each metric
		for metric, metricHelp := range metricsDesc {
			valueStr, exists := row[metric]
			logger.Debug().Msg(fmt.Sprintf("Running for metric: %s, Row: %s", row, row[metric]))
			if !exists {
				logger.Error().Msg(fmt.Sprintf("Metric '%s' does not exist in the row", metric))
				continue
			}
			value, err := strconv.ParseFloat(strings.TrimSpace(valueStr), 64)
			if err != nil {
				logger.Error().Err(errors.New("conversion Error")).Msg(fmt.Sprintf("Unable to convert current value to float (metric=%s, metricHelp=%s, value=<%s>)", metric, metricHelp, valueStr))
				continue
			}

			// Construct dimensions for each metric
			var dimensions []utils.Dimension
			for label, labelValue := range labelsMap {
				dimensions = append(dimensions, utils.Dimension{
					Name:  label,
					Value: labelValue,
				})
			}
			// Here you would use the 'dimensions' and the 'value' to create your metrics
			// For example, you might send them to a monitoring system or log them
			logger.Info().Msg(fmt.Sprintf("Preparing to push Metric '%s': %f, Dimensions: %v, metricType: %s", metric, value, dimensions, cloudWatchType[metric]))
			//Push data to cloudWatch
			err = utils.PutCloudwatchMetrics(logger, utils.MetricDataInput{
				Namespace: namespace,
				MetricData: []utils.MetricDatum{
					{
						MetricName: fmt.Sprintf("%s_%s", context, metric),
						Unit:       cloudWatchType[metric], //How to sort unit types to proper cloudwatch units
						Value:      value,
						Dimensions: dimensions,
					},
				},
			})
			if err != nil {
				logger.Error().Err(err).Msg("Failed to push metric to CloudWatch")
			} else {
				logger.Info().Msg(fmt.Sprintf("Success push Metric '%s': %f, Dimensions: %v", metric, value, dimensions))
			}
			metricsCount++
		}
		//logger.Info().Msg(fmt.Sprintf("Data recieved in genericParser: %s, for context: %s, lables: %s, metricsDesc: %s, metricsType: %s, metricsBuckets: %s, fieldToAppend: %s", row, context, labelsValues, metricsDesc, metricsType, metricsBuckets, fieldToAppend))
		return nil
	}
	//level.Debug(e.logger).Log("Calling function GeneratePrometheusMetrics()")
	err := e.generatePrometheusMetrics(db, genericParser, request, logger)
	//level.Debug(e.logger).Log("ScrapeGenericValues() - metricsCount: ", metricsCount)
	if err != nil {
		return err
	}
	if !ignoreZeroResult && metricsCount == 0 {
		return errors.New("no metrics found while parsing")
	}
	return err
}

func (e *Exporter) generatePrometheusMetrics(db *sql.DB, parse func(row map[string]string) error, query string, logger zerolog.Logger) error {
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
			//logger.Info().Msg(fmt.Sprintf("Column name: %s, Column value: %v", colName, *val))
			//fmt.Println(m)
		}
		logger.Debug().Msg(fmt.Sprintf("Calling parser for %s", m))
		if err := parse(m); err != nil {
			logger.Error().Err(err).Msg("Got error from parser.")
			return err
		}
	}
	return nil
}
