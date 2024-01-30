package oracle

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
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

func getMetricType(metricType string, metricsType map[string]string) prometheus.ValueType {
	var strToPromType = map[string]prometheus.ValueType{
		"gauge":     prometheus.GaugeValue,
		"counter":   prometheus.CounterValue,
		"histogram": prometheus.UntypedValue,
	}

	strType, ok := metricsType[strings.ToLower(metricType)]
	if !ok {
		return prometheus.GaugeValue
	}
	valueType, ok := strToPromType[strings.ToLower(strType)]
	if !ok {
		panic(errors.New("Error while getting prometheus type " + strings.ToLower(strType)))
	}
	return valueType
}

func cleanName(s string) string {
	s = strings.ReplaceAll(s, " ", "_") // Remove spaces
	s = strings.ReplaceAll(s, "(", "")  // Remove open parenthesis
	s = strings.ReplaceAll(s, ")", "")  // Remove close parenthesis
	s = strings.ReplaceAll(s, "/", "")  // Remove forward slashes
	s = strings.ReplaceAll(s, "*", "")  // Remove asterisks
	s = strings.ToLower(s)
	return s
}

// Describe describes all the metrics exported by the Oracle DB exporter.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	// We cannot know in advance what metrics the exporter will generate
	// So we use the poor man's describe method: Run a collect
	// and send the descriptors of all the collected metrics. The problem
	// here is that we need to connect to the Oracle DB. If it is currently
	// unavailable, the descriptors will be incomplete. Since this is a
	// stand-alone exporter and not used as a library within other code
	// implementing additional metrics, the worst that can happen is that we
	// don't detect inconsistent metrics created by this exporter
	// itself. Also, a change in the monitored Oracle instance may change the
	// exported metrics during the runtime of the exporter.

	metricCh := make(chan prometheus.Metric)
	doneCh := make(chan struct{})

	go func() {
		for m := range metricCh {
			ch <- m.Desc()
		}
		close(doneCh)
	}()

	e.Collect(metricCh)
	close(metricCh)
	<-doneCh
}

// Collect implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	// they are running scheduled scrapes we should only scrape new data
	// on the interval
	if e.scrapeInterval != nil && *e.scrapeInterval != 0 {
		// read access must be checked
		e.mu.Lock()
		for _, r := range e.scrapeResults {
			ch <- r
		}
		e.mu.Unlock()
		return
	}

	// otherwise do a normal scrape per request
	e.mu.Lock() // ensure no simultaneous scrapes
	defer e.mu.Unlock()
	e.scrape(ch)
	ch <- e.duration
	ch <- e.totalScrapes
	ch <- e.error
	e.scrapeErrors.Collect(ch)
	ch <- e.up
}

func (e *Exporter) connect() error {
	e.logger.Debug().Msg(fmt.Sprintf("launching connection: %s", maskDsn(e.dsn)))
	db, err := sql.Open("oracle", e.dsn)
	if err != nil {
		e.logger.Debug().Msg(fmt.Sprintf("error while connecting to: %s", maskDsn(e.dsn)))
		return err
	}
	e.logger.Debug().Msg(fmt.Sprintf("set max idle connections to %d", e.config.MaxIdleConns))
	db.SetMaxIdleConns(e.config.MaxIdleConns)
	e.logger.Debug().Msg(fmt.Sprintf("set max open connections to %d", e.config.MaxOpenConns))
	db.SetMaxOpenConns(e.config.MaxOpenConns)
	e.logger.Debug().Msg(fmt.Sprintf("successfully connected to: %s", maskDsn(e.dsn)))
	e.db = db
	if os.Getenv("EXPORTER_TYPE") == "cloudwatch" {
		metricCh := make(chan prometheus.Metric)
		e.scrape(metricCh)
	}
	return nil
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric) {
	var err error
	if err = e.db.Ping(); err != nil {
		if strings.Contains(err.Error(), "sql: database is closed") {
			e.logger.Info().Msg("Reconnecting to DB")
			err = e.connect()
			if err != nil {
				e.logger.Error().Err(errors.New(err.Error())).Msg("Error reconnecting to DB")
			}
		}
	}
	if err = e.db.Ping(); err != nil {
		e.logger.Error().Err(errors.New(err.Error())).Msg("Error pinging oracle.")
		e.up.Set(0)
		return
	}

	e.logger.Info().Msg(fmt.Sprintf("Successfully pinged oracle database: %s", maskDsn(e.dsn)))
	e.up.Set(1)

	wg := sync.WaitGroup{}
	for _, metric := range e.metricsToScrape.Metric {
		wg.Add(1)
		metric := metric
		f := func() {
			defer wg.Done()

			e.logger.Debug().Msg("About to scrape metric: ")
			e.logger.Debug().Msg(fmt.Sprintf("- Metric MetricsDesc: %s", metric.MetricsDesc))
			e.logger.Debug().Msg(fmt.Sprintf("- Metric Context: %s", metric.Context))
			e.logger.Debug().Msg(fmt.Sprintf("- Metric MetricsType: %s", metric.MetricsType))
			e.logger.Debug().Msg(fmt.Sprintf("- Metric MetricsBuckets: %s \"(Ignored unless Histogram type)\"", metric.MetricsBuckets))
			e.logger.Debug().Msg(fmt.Sprintf("- Metric Labels: %s", metric.Labels))
			e.logger.Debug().Msg(fmt.Sprintf("- Metric FieldToAppend: %s", metric.FieldToAppend))
			e.logger.Debug().Msg(fmt.Sprintf("- Metric IgnoreZeroResult: %t", metric.IgnoreZeroResult))
			e.logger.Debug().Msg(fmt.Sprintf("- Metric Request: %s", metric.Request))

			if len(metric.Request) == 0 {
				e.logger.Error().Msg(fmt.Sprintf("Error scraping for %s. Did you forget to define request in your toml file? ", metric.MetricsDesc))
				return
			}

			if len(metric.MetricsDesc) == 0 {
				e.logger.Error().Msg(fmt.Sprintf("Error scraping for query %s. Did you forget to define metricsdesc  in your toml file?", metric.Request))
				return
			}

			for column, metricType := range metric.MetricsType {
				if metricType == "histogram" {
					_, ok := metric.MetricsBuckets[column]
					if !ok {
						e.logger.Error().Msg(fmt.Sprintf("Unable to find MetricsBuckets configuration key for metric. (metric=\" %s \")", column))
						return
					}
				}
			}
			scrapeStart := time.Now()
			e.logger.Info().Msg(fmt.Sprintf("Starting scrape for metrics: %s", metric))
			if err = e.ScrapeMetric(e.db, ch, metric); err != nil {
				e.logger.Error().Err(errors.New(err.Error())).Msg(fmt.Sprintf("error scraping for %s_%s, %s", metric.Context, metric.MetricsDesc, time.Since(scrapeStart)))
				e.scrapeErrors.WithLabelValues(metric.Context).Inc()
			} else {
				e.logger.Info().Msg(fmt.Sprintf("successfully scraped metric: %s %s %s", metric.Context, metric.MetricsDesc, time.Since(scrapeStart)))
			}
		}
		go f()
	}
	wg.Wait()
}

func (e *Exporter) ScrapeMetric(db *sql.DB, ch chan<- prometheus.Metric, metricDefinition Metric) error {
	e.logger.Info().Msg("calling function ScrapeGenericValues()")
	//return e.generatePrometheusMetrics(db, logger, metricDefinition.Request)
	return e.scrapeGenericValues(db, ch, metricDefinition.Context, metricDefinition.Labels,
		metricDefinition.MetricsDesc, metricDefinition.MetricsType, metricDefinition.MetricsBuckets,
		metricDefinition.FieldToAppend, metricDefinition.IgnoreZeroResult,
		metricDefinition.Request, metricDefinition.CloudwatchType)
}

func (e *Exporter) scrapeGenericValues(db *sql.DB, ch chan<- prometheus.Metric, context string, labels []string,
	metricsDesc map[string]string, metricsType map[string]string, metricsBuckets map[string]map[string]string, fieldToAppend string, ignoreZeroResult bool, request string, cloudWatchType map[string]string) error {
	metricsCount := 0
	genericParser := func(row map[string]string) error {
		// Construct labels value
		labelsValues := make([]string, len(labels))
		labelsMap := make(map[string]string, len(labels))
		for i, label := range labels {
			value, exists := row[label]
			if !exists {
				e.logger.Error().Msg(fmt.Sprintf("Label '%s' does not exist in the row", label))
				continue
			}
			labelsValues[i] = value
			labelsMap[label] = value
		}

		// Process each metric
		for metric, metricHelp := range metricsDesc {
			value, err := strconv.ParseFloat(strings.TrimSpace(row[metric]), 64)
			// If not a float, skip current metric
			if err != nil {
				e.logger.Error().Err(errors.New("conversion Error")).Msg(fmt.Sprintf("Unable to convert current value to float (metric=%s, metricHelp=%s, value=<%v>)", metric, metricHelp, value))
				continue
			} else {
				e.logger.Debug().Msg(fmt.Sprintf("Query results looks like %v", value))
			}
			switch os.Getenv("EXPORTER_TYPE") {
			case "cloudwatch":
				err := e.generateCloudwatchMetrics(context, labelsMap, metric, value, cloudWatchType)
				if err != nil {
					e.logger.Error().Err(err).Msg("Failed to send metrics to cloudwatch")
				}
			case "prometheus":
				if strings.Compare(fieldToAppend, "") == 0 {
					desc := prometheus.NewDesc(
						prometheus.BuildFQName(namespace, context, metric),
						metricHelp,
						labels, nil,
					)
					if metricsType[strings.ToLower(metric)] == "histogram" {
						count, err := strconv.ParseUint(strings.TrimSpace(row["count"]), 10, 64)
						if err != nil {
							e.logger.Error().Err(err).Msg("Unable to convert")
							//e.logger.Error().Err(err).Msg("Unable to convert count value to int (metric=" + metric +
							//	",metricHelp=" + metricHelp + ",value=<" + row["count"] + ">)")
							continue
						}
						buckets := make(map[float64]uint64)
						for field, le := range metricsBuckets[metric] {
							lelimit, err := strconv.ParseFloat(strings.TrimSpace(le), 64)
							if err != nil {
								e.logger.Error().Err(err).Msg("Unable to convert")
								//e.logger.Error().Err(err).Msg("Unable to convert bucket limit value to float (metric=" + metric +
								//	",metricHelp=" + metricHelp + ",bucketlimit=<" + le + ">)")
								continue
							}
							counter, err := strconv.ParseUint(strings.TrimSpace(row[field]), 10, 64)
							if err != nil {
								e.logger.Error().Err(err).Msg("Unable to convert")
								//e.logger.Error().Err(err).Msg("Unable to convert ", field, " value to int (metric="+metric+
								//	",metricHelp="+metricHelp+",value=<"+row[field]+">)")
								continue
							}
							buckets[lelimit] = counter
						}
						ch <- prometheus.MustNewConstHistogram(desc, count, value, buckets, labelsValues...)
					} else {
						ch <- prometheus.MustNewConstMetric(desc, getMetricType(metric, metricsType), value, labelsValues...)
					}
					// If no labels, use metric name
				} else {
					desc := prometheus.NewDesc(
						prometheus.BuildFQName(namespace, context, cleanName(row[fieldToAppend])),
						metricHelp,
						nil, nil,
					)
					if metricsType[strings.ToLower(metric)] == "histogram" {
						count, err := strconv.ParseUint(strings.TrimSpace(row["count"]), 10, 64)
						if err != nil {
							e.logger.Error().Err(err).Msg("Unable to convert")
							//level.Error(e.logger).Log("Unable to convert count value to int (metric=" + metric +
							//	",metricHelp=" + metricHelp + ",value=<" + row["count"] + ">)")
							continue
						}
						buckets := make(map[float64]uint64)
						for field, le := range metricsBuckets[metric] {
							lelimit, err := strconv.ParseFloat(strings.TrimSpace(le), 64)
							if err != nil {
								e.logger.Error().Err(err).Msg("Unable to convert")
								//level.Error(e.logger).Log("Unable to convert bucket limit value to float (metric=" + metric +
								//	",metricHelp=" + metricHelp + ",bucketlimit=<" + le + ">)")
								continue
							}
							counter, err := strconv.ParseUint(strings.TrimSpace(row[field]), 10, 64)
							if err != nil {
								e.logger.Error().Err(err).Msg("Unable to convert")
								//level.Error(e.logger).Log("Unable to convert ", field, " value to int (metric="+metric+
								//	",metricHelp="+metricHelp+",value=<"+row[field]+">)")
								continue
							}
							buckets[lelimit] = counter
						}
						ch <- prometheus.MustNewConstHistogram(desc, count, value, buckets)
					} else {
						ch <- prometheus.MustNewConstMetric(desc, getMetricType(metric, metricsType), value)
					}
				}
			}
			metricsCount++
		}
		//logger.Info().Msg(fmt.Sprintf("Data recieved in genericParser: %s, for context: %s, lables: %s, metricsDesc: %s, metricsType: %s, metricsBuckets: %s, fieldToAppend: %s", row, context, labelsValues, metricsDesc, metricsType, metricsBuckets, fieldToAppend))
		return nil
	}
	//level.Debug(e.logger).Log("Calling function GeneratePrometheusMetrics()")
	err := e.generateMetrics(db, genericParser, request)
	//level.Debug(e.logger).Log("ScrapeGenericValues() - metricsCount: ", metricsCount)
	if err != nil {
		return err
	}
	if !ignoreZeroResult && metricsCount == 0 {
		return errors.New("no metrics found while parsing")
	}
	return err
}
