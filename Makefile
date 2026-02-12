.PHONY: build test simulate clean deps backtest

deps:
	go mod download
	go mod tidy

build:
	go build -o bin/simulate cmd/simulate/main.go
	go build -o bin/scan cmd/scan/main.go
	go build -o bin/backtest cmd/backtest/main.go

test:
	go test -v ./...

simulate:
	go run cmd/simulate/main.go

clean:
	rm -rf bin/
	go clean

backtest:
	go run cmd/backtest/main.go \
		--rpc $(RPC_URL) \
		--db data/mempool.db \
		--start 17916526 \
		--end 17916626