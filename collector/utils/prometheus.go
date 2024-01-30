package utils

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"time"

	// Import the AWS SDK packages required for authentication.
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/signer/v4"
)

func SendToAMP(data []byte, ampEndpoint string) error {
	// Create a new session for the AWS SDK.
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")), // Specify the AWS Region of your AMP workspace.
	})
	if err != nil {
		return err
	}

	// Create a new request.
	req, err := http.NewRequest("POST", ampEndpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	// Sign the request using AWS Signature Version 4.
	signer := v4.NewSigner(sess.Config.Credentials)
	signer.Sign(req, bytes.NewReader(data), "aps", *sess.Config.Region, time.Now())

	// Send the request.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check the response status.
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send metrics, status code: %d", resp.StatusCode)
	}

	return nil
}
