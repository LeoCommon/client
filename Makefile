build:
	go build -o bin/ ./cmd/client
	go build -o bin/ ./cmd/modem_manager

run:
	go run cmd/client/main.go --config ./cmd/client/config.yml
