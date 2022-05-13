build:
	go build -o bin/ ./cmd/apogee-client
	go build -o bin/ ./cmd/modem_manager

run:
	go run cmd/modem_manager/main.go --config ./cmd/client/config.yml
