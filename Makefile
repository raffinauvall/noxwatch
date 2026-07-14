SHELL := /usr/bin/env bash
GOCACHE ?= /tmp/noxwatch-go-cache
export GOCACHE

ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: dev build test lint migrate-up migrate-down seed agent-build agent-install-local api-dev web-dev

dev:
	docker compose up --build

api-dev:
	cd apps/api && go run ./cmd/api

web-dev:
	npm --workspace apps/web run dev

build:
	cd apps/api && go build ./cmd/api
	$(MAKE) agent-build
	npm --workspace apps/web run build

test:
	cd apps/api && go test ./...
	cd agent && go test ./...

lint:
	test -z "$$(gofmt -l apps/api)"
	test -z "$$(gofmt -l agent)"
	npm --workspace apps/web run lint

migrate-up:
	cd apps/api && go run ./cmd/api -migrate up

migrate-down:
	cd apps/api && go run ./cmd/api -migrate down

seed:
	@echo "Seed data starts in Phase 2."

agent-build:
	mkdir -p dist
	cd agent && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../dist/noxwatch-agent ./cmd/noxwatch-agent

agent-install-local:
	sudo ./deployments/scripts/install-local.sh
