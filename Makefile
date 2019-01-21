.PHONY: agent
agent:
	go build -o s6.agent agent/main.go

.PHONY: lambda
lambda:
	go build -o s6.lambda lambda/main.go