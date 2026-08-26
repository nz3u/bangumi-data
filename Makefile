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