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
	collectors          = make(map[string]map[string]prometheus.Collector) // Store collectors per engine
	registries          = make(map[string]*prometheus.Registry)            // Store separate registries for each engine
	collectorsMutex     = sync.RWMutex{}                                   // Mutex for safe access
	secretCheckInterval = 15 * time.Minute                                 // How often to check for new secrets
)

func InitializeCollectors(logger log.Logger) {
	listSecretsResult := aws.ListSecrets()
	slogLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	collectorsMutex.Lock() // Lock for safe update
	defer collectorsMutex.Unlock()

	for _, secretItem := range listSecretsResult.SecretList {
		secretName := *secretItem.Name

		secretValue := aws.GetSecretsValue(secretName)
		secretValueMap := map[string]interface{}{}
		err := json.Unmarshal([]byte(secretValue), &secretValueMap)
		if err != nil {
			fmt.Println("Error unmarshalling secret:", err)
			continue
		}

		engine := secretValueMap["engine"].(string)

		// Ensure each database has its own registry
		if _, exists := registries[secretName]; !exists {
			registries[secretName] = prometheus.NewRegistry()
		}

		// Ensure collectors map exists for this database
		if _, exists := collectors[secretName]; !exists {
			collectors[secretName] = make(map[string]prometheus.Collector)
		}

		// Register new collector for this specific database
		var collector prometheus.Collector
		switch engine {
		case "mysql":
			collector = mysql.RegisterMySQLCollector(registries[secretName], secretValueMap, slogLogger)
		case "postgres":
			collector = postgres.RegisterPostgresCollector(registries[secretName], secretValueMap, slogLogger)
		case "oracle", "oracle-ee", "custom-oracle-ee":
			collector = oracle.RegisterOracleDBCollector(registries[secretName], secretValueMap, logger)
		default:
			fmt.Println("Unsupported database engine:", engine)
			continue
		}

		if err != nil {
			fmt.Println("Error initializing collector:", err)
			continue
		}

		collectors[secretName][engine] = collector
	}
}

func RefreshSecrets(logger log.Logger) {
	ticker := time.NewTicker(secretCheckInterval)
	defer ticker.Stop()
	defer collectorsMutex.Unlock()

	for range ticker.C {
		fmt.Println("Refreshing secrets and updating collectors...")
		listSecretsResult := aws.ListSecrets()

		collectorsMutex.Lock()

		// Step 1: Track existing database instances
		existingSecrets := make(map[string]bool)
		for _, secretItem := range listSecretsResult.SecretList {
			existingSecrets[*secretItem.Name] = true
		}

		// Step 2: Add new secrets
		for _, secretItem := range listSecretsResult.SecretList {
			secretName := *secretItem.Name

			// Skip if collector already exists
			if _, exists := collectors[secretName]; exists {
				continue
			}

			// Fetch new secret
			secretValue := aws.GetSecretsValue(secretName)
			secretValueMap := map[string]interface{}{}
			err := json.Unmarshal([]byte(secretValue), &secretValueMap)
			if err != nil {
				fmt.Println("Error unmarshalling secret:", err)
				continue
			}

			engine := secretValueMap["engine"].(string)

			// Ensure each database has its own registry
			if _, exists := registries[secretName]; !exists {
				registries[secretName] = prometheus.NewRegistry()
			}

			// Ensure collectors map exists for this database
			if _, exists := collectors[secretName]; !exists {
				collectors[secretName] = make(map[string]prometheus.Collector)
			}

			slogLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

			// Register new collector
			var collector prometheus.Collector
			switch engine {
			case "mysql":
				collector = mysql.RegisterMySQLCollector(registries[secretName], secretValueMap, slogLogger)
			case "postgres":
				collector = postgres.RegisterPostgresCollector(registries[secretName], secretValueMap, slogLogger)
			case "oracle", "oracle-ee", "custom-oracle-ee":
				collector = oracle.RegisterOracleDBCollector(registries[secretName], secretValueMap, logger)
			default:
				fmt.Println("Unsupported database engine:", engine)
				continue
			}

			if err != nil {
				fmt.Println("Error registering new collector:", err)
				continue
			}

			collectors[secretName][engine] = collector
			fmt.Println("Added new collector for:", secretName)
		}

		// Step 3: Remove secrets that no longer exist
		for secretName, dbCollectors := range collectors {
			if _, found := existingSecrets[secretName]; !found {
				// Unregister all collectors for this database
				for _, collector := range dbCollectors {
					registries[secretName].Unregister(collector)
				}

				// Ensure all running Goroutines for this database are stopped
				delete(collectors, secretName)
				delete(registries, secretName)

				fmt.Println("Removed collector for deleted secret:", secretName)
			}
		}
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

func HandleRequest(logger log.Logger) {
	level.Info(logger).Log("msg", "Starting database collector")

	var wg sync.WaitGroup

	collectorsMutex.RLock() // Lock for safe read
	for secretName, dbCollectors := range collectors {
		// Ensure the database still exists before proceeding
		if _, exists := registries[secretName]; !exists {
			continue
		}

		dbRegistry := registries[secretName] // Get the database-specific registry

		for engine, collector := range dbCollectors {
			wg.Add(1)
			go func(secretName, engine string, collector prometheus.Collector, registry *prometheus.Registry) {
				defer wg.Done()

				// Fetch latest secret value
				secretValue := aws.GetSecretsValue(secretName)
				secretValueMap := map[string]interface{}{}
				if err := json.Unmarshal([]byte(secretValue), &secretValueMap); err != nil {
					fmt.Println("Error parsing secret:", err)
					return
				}

				// Ensure the database still exists before collecting metrics
				collectorsMutex.RLock()
				_, dbStillExists := collectors[secretName]
				collectorsMutex.RUnlock()

				if !dbStillExists {
					fmt.Println("Skipping metrics collection for removed database:", secretName)
					return
				}

				collectMetrics(collector, secretValueMap, logger, registry)
			}(secretName, engine, collector, dbRegistry)
		}
	}
	collectorsMutex.RUnlock() // Unlock after reading

	wg.Wait()
}

func lambdaHandler(logger log.Logger) func() {
	return func() {
		HandleRequest(logger)
	}
}

func main() {
	// Initialize logging
	promlogConfig := &promlog.Config{Level: &promlog.AllowedLevel{}}
	if err := promlogConfig.Level.Set("info"); err != nil {
		fmt.Println("Error setting log level:", err)
		return
	}
	logger := promlog.New(promlogConfig)

	mode := os.Getenv("RUN_MODE")

	// Initialize registries for each database engine
	registries["mysql"] = prometheus.NewRegistry()
	registries["postgres"] = prometheus.NewRegistry()
	registries["oracle"] = prometheus.NewRegistry()

	// Load initial database collectors
	InitializeCollectors(logger)

	// Start background secret refresh process
	go RefreshSecrets(logger) // Runs in a separate goroutine

	if mode == "LAMBDA" {
		// AWS Lambda Execution
		lambda.Start(func() { lambdaHandler(logger) })
	} else if mode == "CRON" {
		fmt.Println("Starting in CRON mode...")

		// Run as internal cron job
		c := cron.New()
		cronSchedule := os.Getenv("CRON_SCHEDULE")
		if cronSchedule == "" {
			cronSchedule = "@every 10s"
		}
		_, err := c.AddFunc(cronSchedule, func() {
			HandleRequest(logger)
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
