package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/KurioApp/s6"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

var (
	agentURL   string
	httpClient *http.Client
)

func main() {
	agentURL = os.Getenv("AGENT_URL")
	if agentURL == "" {
		log.Fatal("Invalid Agent URL")
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
			log.Fatalf("Error: %v", err)
		}

		resp, err := httpClient.Post(agentURL, "application/json", bytes.NewBuffer(body))
		if err != nil || resp == nil {
			log.Fatal("Failed sending to agent")
		}

		if resp.StatusCode != http.StatusOK {
			log.Fatalf("Return from agent: %v", resp.Status)
		}

		log.Print("OK")
	}
}
