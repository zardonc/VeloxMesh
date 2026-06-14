.PHONY: run test fmt vet

run:
	go run cmd/gateway/main.go

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...
