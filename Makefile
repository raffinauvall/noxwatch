SHELL := /usr/bin/env bash
GOCACHE ?= /tmp/noxwatch-go-cache
export GOCACHE

ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: dev build test lint migrate-up migrate-down seed agent-build agent-install-local api-dev web-dev ssh-tunnel local-helper local-helper-build local-helper-install

API_PORT ?= 8080
REMOTE_API_PORT ?= 18082
PUBLIC_WEB_URL ?= http://localhost:3000
LOCAL_HELPER_ADDR ?= 127.0.0.1:9734

dev:
	docker compose up --build

api-dev:
	cd apps/api && go run ./cmd/api

web-dev:
	npm --workspace apps/web run dev

ssh-tunnel:
	./deployments/scripts/reverse-tunnel-ssh.sh --target "$(SSH_TARGET)" $(if $(SSH_PORT),--port "$(SSH_PORT)") --local-port "$(API_PORT)" --remote-port "$(REMOTE_API_PORT)"

local-helper: agent-build
	cd apps/api && go run ./cmd/local-helper -repo-root ../.. -origin "$(PUBLIC_WEB_URL)" -addr "$(LOCAL_HELPER_ADDR)" -local-api-port "$(API_PORT)"

local-helper-build:
	mkdir -p dist
	cd apps/api && go build -o ../../dist/noxwatch-local-helper ./cmd/local-helper

local-helper-install: agent-build local-helper-build
	./deployments/scripts/install-local-helper.sh --repo-root "$(CURDIR)" --origin "$(PUBLIC_WEB_URL)" --addr "$(LOCAL_HELPER_ADDR)" --local-api-port "$(API_PORT)"

build:
	cd apps/api && go build ./cmd/api
	$(MAKE) agent-build
	$(MAKE) local-helper-build
	npm --workspace apps/web run build

test:
	cd apps/api && go test ./...
	cd agent && go test ./...
	npm --workspace apps/web run test
	./deployments/scripts/bootstrap-ssh.test.sh
	./deployments/scripts/reverse-tunnel-ssh.test.sh

lint:
	test -z "$$(gofmt -l apps/api)"
	test -z "$$(gofmt -l agent)"
	npm --workspace apps/web run lint

migrate-up:
	cd apps/api && go run ./cmd/api -migrate up

migrate-down:
	cd apps/api && go run ./cmd/api -migrate down

seed:
	cd apps/api && go run ./cmd/seed

agent-build:
	mkdir -p dist
	cd agent && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../dist/noxwatch-agent ./cmd/noxwatch-agent

agent-install-local:
	sudo ./deployments/scripts/install-local.sh
