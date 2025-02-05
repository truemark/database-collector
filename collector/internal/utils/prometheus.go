package utils

import (
	"bytes"
	"errors"
	"fmt"
	ioprometheusclient "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/prompb"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
)

func ConvertMetricFamilyToTimeSeries(metricFamilies []*ioprometheusclient.MetricFamily, identifier string, engine string) (*http.Response, error) {
	var timeSeries []prompb.TimeSeries

	for _, mf := range metricFamilies {
		for _, m := range mf.Metric {
			var timestamp int64
			if m.GetTimestampMs() != 0 {
				timestamp = m.GetTimestampMs()
			} else {
				timestamp = time.Now().UnixNano() / 1e6
			}

			ts := prompb.TimeSeries{}
			labels := make([]prompb.Label, len(m.Label)+6)
			labels[0] = prompb.Label{
				Name:  "__name__",
				Value: mf.GetName(), // Assuming the metric name is stored here
			}
			for i, l := range m.Label {
				labels[i+1] = prompb.Label{
					Name:  l.GetName(),
					Value: l.GetValue(),
				}
			}
			labels[len(m.Label)+1] = prompb.Label{
				Name:  "identifier",
				Value: strings.Split(identifier, ".")[0],
			}
			labels[len(m.Label)+2] = prompb.Label{
				Name:  "job",
				Value: "database-collector",
			}
			labels[len(m.Label)+3] = prompb.Label{
				Name:  "region",
				Value: os.Getenv("AWS_REGION"),
			}
			labels[len(m.Label)+4] = prompb.Label{
				Name:  "accountId",
				Value: os.Getenv("AWS_ACCOUNT_ID"),
			}
			labels[len(m.Label)+5] = prompb.Label{
				Name:  "engine",
				Value: engine,
			}
			// add accountId and region as labels
			ts.Labels = labels

			var value float64
			switch *mf.Type {
			case ioprometheusclient.MetricType_COUNTER:
				if m.Counter != nil {
					value = m.Counter.GetValue()
				}
			case ioprometheusclient.MetricType_GAUGE:
				if m.Gauge != nil {
					value = m.Gauge.GetValue()
				}
			case ioprometheusclient.MetricType_HISTOGRAM:
				if m.Histogram != nil {
					for _, bucket := range m.Histogram.Bucket {
						ts.Samples = append(ts.Samples, prompb.Sample{
							Value:     float64(bucket.GetCumulativeCount()),
							Timestamp: timestamp,
						})
					}
					value = m.Histogram.GetSampleSum()
				}
			case ioprometheusclient.MetricType_SUMMARY:
				if m.Summary != nil {
					for _, quantile := range m.Summary.Quantile {
						ts.Samples = append(ts.Samples, prompb.Sample{
							Value:     quantile.GetValue(),
							Timestamp: timestamp,
						})
					}
					value = m.Summary.GetSampleSum()
				}
			}

			if timestamp != 0 {
				sample := prompb.Sample{
					Value:     value,
					Timestamp: timestamp,
				}
				ts.Samples = []prompb.Sample{sample}
				timeSeries = append(timeSeries, ts)
			}
		}
	}

	writeRequest := &prompb.WriteRequest{
		Timeseries: timeSeries,
	}
	body, err := encodeWriteRequestIntoProtoAndSnappy(writeRequest)
	if err != nil {
		return nil, err
	}
	return sendRequestToAPS(body)
}

func encodeWriteRequestIntoProtoAndSnappy(writeRequest *prompb.WriteRequest) (*bytes.Reader, error) {
	data, err := proto.Marshal(writeRequest)
	if err != nil {
		return nil, err
	}
	encoded := snappy.Encode(nil, data)
	return bytes.NewReader(encoded), nil
}

func sendRequestToAPS(body *bytes.Reader) (*http.Response, error) {
	remoteWriteURL := os.Getenv("PROMETHEUS_REMOTE_WRITE_URL")
	if remoteWriteURL == "" {
		return nil, errors.New("PROMETHEUS_REMOTE_WRITE_URL is not set")
	}
	req, err := http.NewRequest("POST", remoteWriteURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create new request: %w", err)
	}

	sess, _ := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	})

	signer := v4.NewSigner(sess.Config.Credentials)
	_, err = signer.Sign(req, body, "aps", *sess.Config.Region, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to sign the request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to APS failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyString := string(bodyBytes)
		return nil, fmt.Errorf("request to AMP failed with status: %d, %s", resp.StatusCode, bodyString)
	}

	return resp, nil
}
