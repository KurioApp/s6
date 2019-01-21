package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/KurioApp/s6"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/sirupsen/logrus"
)

var (
	agentURL   string
	httpClient *http.Client
)

func main() {
	agentURL = os.Getenv("AGENT_URL")
	if agentURL == "" {
		logrus.Fatal("Invalid Agent URL")
	}

	httpClient = &http.Client{Timeout: 5 * time.Second}

	lambda.Start(handle)
}

func handle(ctx context.Context, s3Event events.S3Event) {
	for _, record := range s3Event.Records {
		fileObj := s6.S3File{
			Region: record.AWSRegion,
			Bucket: record.S3.Bucket.Name,
			Key:    record.S3.Object.Key,
		}

		body, err := json.Marshal(fileObj)
		if err != nil {
			logrus.Fatalf("Error: %v", err)
		}

		resp, err := httpClient.Post(agentURL, "application/json", bytes.NewBuffer(body))
		if err != nil || resp == nil {
			logrus.Fatalf("Failed sending to agent: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			logrus.Fatalf("Return from agent: %v", resp.Status)
		}

		logrus.Print("OK")
	}
}
