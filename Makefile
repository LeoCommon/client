.PHONY : test build ci

all :
	go build -o bin/ ./cmd/apogee-client
	go build -o bin/ ./cmd/modem_manager

arm64:
	GOOS=linux GOARCH=arm64 go build -o bin/client_arm64 ./cmd/apogee-client
	GOOS=linux GOARCH=arm64 go build -o bin/modem_manager_arm64 ./cmd/modem_manager

run:
	go run cmd/modem_manager/main.go --config ./cmd/apogee-client/config.yml

client:
	make build
	./bin/apogee-client --config ./cmd/apogee-client/config.yml

codecov:
	go test -coverprofile coverage.out ./... 
	go tool cover -html=coverage.out

test:
	go test -v ./...

ci: | all test