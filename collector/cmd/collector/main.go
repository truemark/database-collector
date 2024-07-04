package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promlog"
	"github.com/truemark/database-collector/exporters/mysql"
	"github.com/truemark/database-collector/exporters/oracle"
	"github.com/truemark/database-collector/exporters/postgres"
	"github.com/truemark/database-collector/internal/aws"
	"github.com/truemark/database-collector/internal/utils"
)

var registry = prometheus.NewRegistry()

func collectMetrics(secretValueMap map[string]interface{}, engine string, logger log.Logger, wg *sync.WaitGroup) {
	defer wg.Done()

	registry := prometheus.NewRegistry()
	switch engine {
	case "mysql":
		mysql.RegisterMySQLCollector(registry, secretValueMap, logger)
	case "postgres":
		postgres.RegisterPostgresCollector(registry, secretValueMap, logger)
	case "oracle":
		oracle.RegisterOracleDBCollector(registry, secretValueMap, logger)
	}

	metricFamilies, err := registry.Gather()
	if err != nil {
		level.Error(logger).Log("msg", "Error gathering metrics", "err", err)
		return
	}

	response, err := utils.ConvertMetricFamilyToTimeSeries(metricFamilies, secretValueMap["host"].(string))
	if err != nil {
		fmt.Println("Failed to send metrics to APS")
	} else {
		fmt.Println("Successfully sent metrics to APS ", response)
	}

	// Print gathered metrics
	// for _, mf := range metricFamilies {
	// 	fmt.Printf("Metric Family: %s\n", *mf.Name)
	// 	for _, m := range mf.Metric {
	// 		fmt.Printf("  Metric: %v\n", m)
	// 	}
	// }
}

func main() {
	promlogConfig := &promlog.Config{
		Level: &promlog.AllowedLevel{},
	}
	if err := promlogConfig.Level.Set("info"); err != nil {
		fmt.Println("Error setting log level:", err)
		return
	}

	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", "Starting database collector")

	listSecretsResult := aws.ListSecrets()
	var wg sync.WaitGroup

	for i := 0; i < len(listSecretsResult.SecretList); i++ {
		secretValue := aws.GetSecretsValue(listSecretsResult.SecretList[i].Name)
		secretValueMap := map[string]interface{}{}
		err := json.Unmarshal([]byte(secretValue), &secretValueMap)
		if err != nil {
			fmt.Println(err)
			continue
		}

		engine := secretValueMap["engine"].(string)
		wg.Add(1)
		go collectMetrics(secretValueMap, engine, logger, &wg)
	}

	wg.Wait()
}
