package utils

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	ioprometheusclient "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/prompb"
	"net/http"
	"os"
	"strconv"
	"time"

	// Import the AWS SDK packages required for authentication.
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
)

func ConvertMetricFamilyToTimeSeries(metricFamilies []*ioprometheusclient.MetricFamily) (*http.Response, error) {
	var timeSeries []prompb.TimeSeries

	for _, mf := range metricFamilies {
		for _, m := range mf.Metric {
			ts := prompb.TimeSeries{}

			// Convert labels
			labels := make([]prompb.Label, len(m.Label)) // Note the change here from []*prompb.Label to []prompb.Label
			for i, l := range m.Label {
				labels[i] = prompb.Label{
					Name:  l.GetName(),  // Directly using the string value
					Value: l.GetValue(), // Directly using the string value
				}
			}
			ts.Labels = labels

			// Convert metric value based on its type (this example only handles Gauge and Counter types)
			var value float64
			var timestamp int64
			switch *mf.Type {
			case ioprometheusclient.MetricType_COUNTER:
				if m.Counter != nil {
					value = m.Counter.GetValue()
					timestamp = time.Now().Unix()
				}
			case ioprometheusclient.MetricType_GAUGE:
				if m.Gauge != nil {
					value = m.Gauge.GetValue()
					timestamp = time.Now().Unix()
				}
				// Add cases for other metric types (Histogram, Summary, etc.) as needed
			}

			// Add sample
			sample := prompb.Sample{
				Value:     value,
				Timestamp: timestamp,
			}
			ts.Samples = []prompb.Sample{sample}

			timeSeries = append(timeSeries, ts)
		}
	}
	writeRequest := &prompb.WriteRequest{
		Timeseries: timeSeries,
	}
	body := encodeWriteRequestIntoProtoAndSnappy(writeRequest)
	response, err := sendRequestToAPS(body)
	return response, err
}

func CreateWriteRequestAndSendToAPS(timeseries []prompb.TimeSeries) (*http.Response, error) {
	writeRequest := &prompb.WriteRequest{
		Timeseries: timeseries,
	}

	body := encodeWriteRequestIntoProtoAndSnappy(writeRequest)
	fmt.Println("ROBIN")
	fmt.Println(body)
	response, err := sendRequestToAPS(body)
	fmt.Println(response)
	return response, err
}

func encodeWriteRequestIntoProtoAndSnappy(writeRequest *prompb.WriteRequest) *bytes.Reader {
	data, err := proto.Marshal(writeRequest)

	if err != nil {
		panic(err)
	}

	encoded := snappy.Encode(nil, data)
	body := bytes.NewReader(encoded)
	return body
}

func roleSessionName() string {
	suffix, err := os.Hostname()

	if err != nil {
		now := time.Now().Unix()
		suffix = strconv.FormatInt(now, 10)
	}

	return "aws-sigv4-proxy-" + suffix
}

func sendRequestToAPS(body *bytes.Reader) (*http.Response, error) {
	// Create an HTTP request from the body content and set necessary parameters.
	remoteWriteURL := ""
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

	// Determine AWS credentials
	roleArn := os.Getenv("AWS_AMP_ROLE_ARN")
	var awsCredentials *credentials.Credentials
	if roleArn != "" {
		awsCredentials = stscreds.NewCredentials(sess, roleArn, func(p *stscreds.AssumeRoleProvider) {
			p.RoleSessionName = roleSessionName()
		})
	} else {
		awsCredentials = sess.Config.Credentials
	}

	// Sign the request
	signer := v4.NewSigner(awsCredentials)
	_, err = signer.Sign(req, body, "aps", os.Getenv("PROMETHEUS_REGION"), time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to sign the request: %w", err)
	}

	// Set request headers
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	// Perform the HTTP request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to APS failed: %w", err)
	}

	// Optionally, you might want to check the response status code here
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request to AMP failed with status: %d", resp.StatusCode)
	}

	return resp, nil
}
