.PHONY: all web build run

all:
	$(MAKE) web
	$(MAKE) run

web:
	cd web && npm run build

build:
	go build ./cmd/bangumi

run: build
	./bangumi serve
docs:
	go run github.com/swaggo/swag/cmd/swag init -d . -g cmd/bangumi/main.go -o docs --parseDependency --parseInternal
	go run ./cmd/openapiconv
