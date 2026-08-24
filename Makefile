.PHONY: build serve

build:
	cd web && npm install && npm run build
	go build ./cmd/bangumi

serve: build
	./bangumi serve

all: serve