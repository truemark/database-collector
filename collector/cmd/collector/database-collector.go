package main

import (
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	cron "github.com/robfig/cron/v3"
	"github.com/truemark/database-collector/exporters/mysql"
	"github.com/truemark/database-collector/exporters/oracle"
	"github.com/truemark/database-collector/exporters/postgres"
	"github.com/truemark/database-collector/internal/aws"
	"github.com/truemark/database-collector/internal/utils"
	"log/slog"
	"os"
	"sync"
	"time"
)

type DatabaseInstance struct {
	Engine string
	Secret map[string]interface{}
}

var (
	collectors          = make(map[string]prometheus.Collector) // Store registered collectors
	collectorsMutex     = sync.RWMutex{}                        // Mutex to ensure thread safety
	secretCheckInterval = 15 * time.Minute                      // How often to check for new secrets
)

func InitializeCollectors(registry *prometheus.Registry, logger log.Logger) map[string]prometheus.Collector {
	dbInstances := make(map[string]DatabaseInstance)
	collectors := make(map[string]prometheus.Collector)

	listSecretsResult := aws.ListSecrets()
	slogLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	for _, secretItem := range listSecretsResult.SecretList {
		secretValue := aws.GetSecretsValue(*secretItem.Name)
		secretValueMap := map[string]interface{}{}
		err := json.Unmarshal([]byte(secretValue), &secretValueMap)
		if err != nil {
			fmt.Println("Error unmarshalling secret:", err)
			continue
		}

		engine := secretValueMap["engine"].(string)
		dbInstances[*secretItem.Name] = DatabaseInstance{Engine: engine, Secret: secretValueMap}

		// Initialize collectors once and store in map
		var collector prometheus.Collector
		switch engine {
		case "mysql":
			collector = mysql.RegisterMySQLCollector(registry, secretValueMap, slogLogger)
		case "postgres":
			collector = postgres.RegisterPostgresCollector(registry, secretValueMap, slogLogger)
		case "oracle", "oracle-ee", "custom-oracle-ee":
			collector = oracle.RegisterOracleDBCollector(registry, secretValueMap, logger)
		default:
			fmt.Println("Unsupported database engine:", engine)
			continue
		}

		if err != nil {
			fmt.Println("Error initializing collector:", err)
			continue
		}

		collectors[*secretItem.Name] = collector
	}

	return collectors
}

func RefreshSecrets(registry *prometheus.Registry, logger log.Logger) {
	ticker := time.NewTicker(secretCheckInterval) // Run every 1 minute (adjustable)
	defer ticker.Stop()

	for range ticker.C {
		fmt.Println("Refreshing secrets and updating collectors...")
		listSecretsResult := aws.ListSecrets()
		newCollectors := make(map[string]prometheus.Collector)

		collectorsMutex.Lock() // Lock for safe update
		for _, secretItem := range listSecretsResult.SecretList {
			secretName := *secretItem.Name

			// Skip if collector already exists
			if _, exists := collectors[secretName]; exists {
				continue
			}

			// Fetch secret value
			secretValue := aws.GetSecretsValue(secretName)
			secretValueMap := map[string]interface{}{}
			err := json.Unmarshal([]byte(secretValue), &secretValueMap)
			if err != nil {
				fmt.Println("Error unmarshalling secret:", err)
				continue
			}

			engine := secretValueMap["engine"].(string)
			slogLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

			// Register new collector
			var collector prometheus.Collector
			switch engine {
			case "mysql":
				collector = mysql.RegisterMySQLCollector(registry, secretValueMap, slogLogger)
			case "postgres":
				collector = postgres.RegisterPostgresCollector(registry, secretValueMap, slogLogger)
			case "oracle", "oracle-ee", "custom-oracle-ee":
				collector = oracle.RegisterOracleDBCollector(registry, secretValueMap, logger)
			default:
				fmt.Println("Unsupported database engine:", engine)
				continue
			}

			if err != nil {
				fmt.Println("Error registering new collector:", err)
				continue
			}

			// Store new collector
			newCollectors[secretName] = collector
			fmt.Println("Added new collector for:", secretName)
		}

		// Find removed secrets and unregister their collectors
		for secretName, collector := range collectors {
			found := false
			for _, secretItem := range listSecretsResult.SecretList {
				if *secretItem.Name == secretName {
					found = true
					break
				}
			}
			if !found {
				// Unregister collector
				registry.Unregister(collector)
				delete(collectors, secretName)
				fmt.Println("Removed collector for deleted secret:", secretName)
			}
		}

		// Add new collectors to global map
		for secretName, collector := range newCollectors {
			collectors[secretName] = collector
		}
		collectorsMutex.Unlock()
	}
}

func collectMetrics(collector prometheus.Collector, secretValueMap map[string]interface{}, logger log.Logger, registry *prometheus.Registry) {
	metricFamilies, err := registry.Gather()
	if err != nil {
		level.Error(logger).Log("msg", "Error gathering metrics", "err", err)
		return
	}

	response, err := utils.ConvertMetricFamilyToTimeSeries(metricFamilies, secretValueMap["host"].(string))
	if err != nil {
		fmt.Println("Failed to send metrics to APS", err)
	} else {
		fmt.Println("Successfully sent metrics to APS ", response)
	}
}

func HandleRequest(registry *prometheus.Registry, logger log.Logger) {
	level.Info(logger).Log("msg", "Starting database collector")

	var wg sync.WaitGroup

	collectorsMutex.RLock() // Lock for safe read
	for secretName, collector := range collectors {
		wg.Add(1)
		go func(secretName string, collector prometheus.Collector) {
			defer wg.Done()

			// Fetch latest secret value
			secretValue := aws.GetSecretsValue(secretName)
			secretValueMap := map[string]interface{}{}
			if err := json.Unmarshal([]byte(secretValue), &secretValueMap); err != nil {
				fmt.Println("Error parsing secret:", err)
				return
			}

			collectMetrics(collector, secretValueMap, logger, registry)
		}(secretName, collector)
	}
	collectorsMutex.RUnlock() // Unlock after reading

	wg.Wait()
}

func lambdaHandler(logger log.Logger) func() {
	registry := prometheus.NewRegistry()
	return func() {
		HandleRequest(registry, logger)
	}
}

func main() {
	promlogConfig := &promlog.Config{Level: &promlog.AllowedLevel{}}
	if err := promlogConfig.Level.Set("info"); err != nil {
		fmt.Println("Error setting log level:", err)
		return
	}

	logger := promlog.New(promlogConfig)
	mode := os.Getenv("RUN_MODE")
	registry := prometheus.NewRegistry()

	// Load existing secrets at startup
	collectors = InitializeCollectors(registry, logger)

	// Start background secret refresh
	go RefreshSecrets(registry, logger) // Run in a separate goroutine

	if mode == "LAMBDA" {
		lambda.Start(func() { HandleRequest(registry, logger) })
	} else if mode == "CRON" {
		fmt.Println("Starting in CRON mode...")

		c := cron.New()
		cronSchedule := os.Getenv("CRON_SCHEDULE")
		if cronSchedule == "" {
			cronSchedule = "@every 10s"
		}
		_, err := c.AddFunc(cronSchedule, func() {
			HandleRequest(registry, logger)
		})
		if err != nil {
			fmt.Println("Error setting up cron job:", err)
			return
		}
		c.Start()

		// Keep the program running
		select {}
	} else {
		fmt.Println("Invalid RUN_MODE. Set RUN_MODE to either 'LAMBDA' or 'CRON'")
	}
}
