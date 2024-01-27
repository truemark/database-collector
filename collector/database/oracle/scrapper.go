package oracle

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/rs/zerolog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func maskDsn(dsn string) string {
	parts := strings.Split(dsn, "@")
	if len(parts) > 1 {
		maskedURL := "***@" + parts[1]
		return maskedURL
	}
	return dsn
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
			logger.Info().Msg(fmt.Sprintf("Starting scrape for metrics: %s", metric))
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

			switch os.Getenv("EXPORTER_TYPE") {
			case "cloudwatch":
				err := e.generateCloudwatchMetrics(logger, context, labelsMap, metric, value, cloudWatchType)
				if err != nil {
					logger.Error().Err(err).Msg("Failed to send metrics to cloudwatch")
				}
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
