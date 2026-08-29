.PHONY: build test lint tidy clean update

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

update:
	git pull --ff-only
	docker compose -f docker/docker-compose.yml build --pull
	docker compose -f docker/docker-compose.yml up -d --force-recreate
