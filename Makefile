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
	npm --workspace apps/web run build

test:
	cd apps/api && go test ./...

lint:
	test -z "$$(gofmt -l apps/api)"
	npm --workspace apps/web run lint

migrate-up:
	cd apps/api && go run ./cmd/api -migrate up

migrate-down:
	cd apps/api && go run ./cmd/api -migrate down

seed:
	@echo "Seed data starts in Phase 2."

agent-build:
	@echo "Agent implementation starts in Phase 4."

agent-install-local:
	@echo "Agent systemd packaging starts in Phase 4."
