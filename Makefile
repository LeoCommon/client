.PHONY : test build ci

all :
	go build -o bin/ ./cmd/client
	go build -o bin/ ./cmd/modem_manager

build: | all

arm64:
	GOOS=linux GOARCH=arm64 go build -o bin/client_arm64 ./cmd/client
	GOOS=linux GOARCH=arm64 go build -o bin/modem_manager_arm64 ./cmd/modem_manager

clean:
	rm -rf bin/

run:
	go run cmd/modem_manager/main.go --config ./config/config.toml

client:
	make build
	./bin/client --config ./config/$(config) --debug

sample_conf:
	go build -o bin/ ./cmd/sample_conf
	./bin/sample_conf

licenses:
	go install github.com/google/go-licenses@latest
	go install github.com/google/addlicense@latest
	addlicense -c "LeoCommon" -l "GPL-3.0-or-later" -s=only -ignore "**/vendor/**" -ignore "**/*.{toml,sh}" . 
	mkdir -p reports
	go-licenses report --template scripts/licenses.tpl ./... > reports/licenses.md


coverage:
	go test -coverprofile coverage.out ./... 
	go tool cover -html=coverage.out

test:
	go test --race -v ./...

ci: | all test
