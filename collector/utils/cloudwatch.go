package utils

import (
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/rs/zerolog"
)

type MetricDataInput struct {
	Namespace  string
	MetricData []MetricDatum
}

type MetricDatum struct {
	MetricName string
	Unit       string
	Value      float64
	Dimensions []Dimension
}

type Dimension struct {
	Name  string
	Value string
}

func (mdi *MetricDataInput) toCloudWatchPutMetricDataInput() *cloudwatch.PutMetricDataInput {
	metricData := make([]*cloudwatch.MetricDatum, len(mdi.MetricData))
	for i, m := range mdi.MetricData {
		dimensions := make([]*cloudwatch.Dimension, len(m.Dimensions))
		for j, d := range m.Dimensions {
			dimensions[j] = &cloudwatch.Dimension{
				Name:  aws.String(d.Name),
				Value: aws.String(d.Value),
			}
		}
		metricData[i] = &cloudwatch.MetricDatum{
			MetricName: aws.String(m.MetricName),
			Unit:       aws.String(m.Unit),
			Value:      aws.Float64(m.Value),
			Dimensions: dimensions,
		}
	}
	out := &cloudwatch.PutMetricDataInput{
		Namespace:  aws.String(mdi.Namespace),
		MetricData: metricData,
	}
	return out
}

func PutCloudwatchMetrics(logger zerolog.Logger, metricInput MetricDataInput) error {
	region := os.Getenv("AWS_REGION")
	sess := session.Must(session.NewSession())
	svc := cloudwatch.New(sess, aws.NewConfig().WithRegion(region))
	cwInput := metricInput.toCloudWatchPutMetricDataInput()
	out, err := svc.PutMetricData(cwInput)

	if err != nil {
		logger.Error().Err(errors.New(err.Error())).Msg("Failed to push metrics to cloudwatch.")
	} else {
		logger.Info().Msg(fmt.Sprintf("Successfully sent metrics to cloudWatch: %s", out))
	}
	return err
}
