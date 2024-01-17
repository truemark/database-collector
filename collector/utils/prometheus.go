package utils

//
//import (
//	"database-collector/database"
//	"database/sql"
//	"github.com/prometheus/client_golang/prometheus"
//	"github.com/rs/zerolog"
//	"sync"
//	"time"
//)
//
//type Exporter struct {
//	config          *database.Config
//	mu              *sync.Mutex
//	metricsToScrape database.Metrics
//	scrapeInterval  *time.Duration
//	dsn             string
//	duration, error prometheus.Gauge
//	totalScrapes    prometheus.Counter
//	scrapeErrors    *prometheus.CounterVec
//	scrapeResults   []prometheus.Metric
//	up              prometheus.Gauge
//	db              *sql.DB
//	logger          zerolog.Logger
//}
//
//var (
//	additionalMetrics database.Metrics
//	hashMap           = make(map[int][]byte)
//	namespace         = "oracledb"
//	exporterName      = "exporter"
//)
//
//func NewExporter(logger zerolog.Logger, cfg *database.Config) (*database.Exporter, error) {
//	e := &database.Exporter{
//		mu:  &sync.Mutex{},
//		dsn: cfg.DSN,
//		duration: prometheus.NewGauge(prometheus.GaugeOpts{
//			Namespace: namespace,
//			Subsystem: exporterName,
//			Name:      "last_scrape_duration_seconds",
//			Help:      "Duration of the last scrape of metrics from Oracle DB.",
//		}),
//		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
//			Namespace: namespace,
//			Subsystem: exporterName,
//			Name:      "scrapes_total",
//			Help:      "Total number of times Oracle DB was scraped for metrics.",
//		}),
//		scrapeErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
//			Namespace: namespace,
//			Subsystem: exporterName,
//			Name:      "scrape_errors_total",
//			Help:      "Total number of times an error occurred scraping a Oracle database.",
//		}, []string{"collector"}),
//		error: prometheus.NewGauge(prometheus.GaugeOpts{
//			Namespace: namespace,
//			Subsystem: exporterName,
//			Name:      "last_scrape_error",
//			Help:      "Whether the last scrape of metrics from Oracle DB resulted in an error (1 for error, 0 for success).",
//		}),
//		up: prometheus.NewGauge(prometheus.GaugeOpts{
//			Namespace: namespace,
//			Name:      "up",
//			Help:      "Whether the Oracle database server is up.",
//		}),
//		logger: logger,
//		config: cfg,
//	}
//	e.metricsToScrape = e.DefaultMetrics(logger)
//	err := e.connect(logger)
//	return e, err
//}
