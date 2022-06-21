build:
	go build -o bin/ ./cmd/apogee-client
	go build -o bin/ ./cmd/modem_manager

run:
	go run cmd/modem_manager/main.go --config ./cmd/apogee-client/config.yml

client:
	make build
	go run cmd/apogee-client/main.go --config ./cmd/apogee-client/config.yml

arm64:
	GOOS=linux GOARCH=arm64 go build -o bin/client_arm64 ./cmd/apogee-client
	GOOS=linux GOARCH=arm64 go build -o bin/modem_manager_arm64 ./cmd/modem_manager