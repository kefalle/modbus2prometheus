.PHONY: build test lint tidy clean

build:
	GOPATH= GOMODCACHE= GOCACHE= mkdir -p build && go build -o build/modbus2prometheus .

test:
	GOPATH= GOMODCACHE= GOCACHE= go test -v ./...

lint:
	GOPATH= GOMODCACHE= GOCACHE= go vet ./...

tidy:
	GOPATH= GOMODCACHE= GOCACHE= go mod tidy

clean:
	rm -rf build
