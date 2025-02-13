package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/truemark/database-collector/internal/utils"
)

type Event struct {
	Detail    json.RawMessage `json:"Detail"`
	AccountID string          `json:"AccountID"`
}

type RdsEventMessage struct {
	EventCategories  []string `json:"EventCategories"`
	SourceType       string   `json:"SourceType"`
	SourceArn        string   `json:"SourceArn"`
	Date             string   `json:"Date"`
	SourceIdentifier string   `json:"SourceIdentifier"`
	Message          string   `json:"Message"`
	EventID          string   `json:"EventID"`
}

var EventsCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "rds_service_events",
		Help: "This metric indicates on whats happening on various aws services, e.g RDS",
	},
	[]string{"event_id", "event_message", "event_source"},
)

func handler(ctx context.Context, e events.CloudWatchEvent) {
	registry := prometheus.NewRegistry()
	registry.Unregister(prometheus.NewGoCollector())
	registry.Unregister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	registry.MustRegister(EventsCounter)
	event := RdsEventMessage{}
	print(string(e.Detail))
	err := json.Unmarshal(e.Detail, &event)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(event.EventID)
	if len(event.EventID) == 1 {
		EventsCounter.WithLabelValues("none", event.Message, event.SourceIdentifier).Inc()
	} else {
		EventsCounter.WithLabelValues(event.EventID, event.Message, event.SourceIdentifier).Inc()
	}
	print(EventsCounter)
	gatherers := prometheus.Gatherers{
		registry,
	}
	metricFamilies, err := gatherers.Gather()
	response, err := utils.ConvertMetricFamilyToTimeSeries(metricFamilies, event.EventID, "NA")
	if err != nil {
		fmt.Println(err, "Failed to convert metric family to time series")
	} else {
		fmt.Println("Successfully sent metrics to APS", response)
	}
}

func main() {
	lambda.Start(handler)
}
