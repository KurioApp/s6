.PHONY: agent
agent:
	dep ensure
	go build -o s6.agent agent/main.go

.PHONY: lambda
lambda:
	dep ensure
	go build -o s6.lambda lambda/main.go