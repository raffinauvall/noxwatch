package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/raffinauvall/noxwatch/agent/internal/client"
	"github.com/raffinauvall/noxwatch/agent/internal/config"
	"github.com/raffinauvall/noxwatch/agent/internal/runner"
)

const version = "0.1.0"

func main() {
	configPath := flag.String("config", "/etc/noxwatch/agent.yaml", "agent configuration path")
	flag.Parse()
	command := "run"
	if flag.NArg() > 0 {
		command = flag.Arg(0)
	}
	if command == "version" {
		fmt.Println("noxwatch-agent " + version)
		return
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fail("invalid configuration", err)
	}
	switch command {
	case "run":
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		defer stop()
		logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
		if err := runner.Run(ctx, cfg, version, logger); err != nil {
			logger.Error("agent stopped", "error", err)
			os.Exit(1)
		}
	case "status":
		credentials, err := client.LoadCredentials(cfg.CredentialFile)
		if err != nil {
			fail("agent is not enrolled", err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "enrolled", "server_id": credentials.ServerID, "agent_id": credentials.AgentID})
	case "config":
		if flag.NArg() != 2 || flag.Arg(1) != "check" {
			fail("usage", fmt.Errorf("noxwatch-agent config check"))
		}
		fmt.Println("configuration is valid")
	case "unregister":
		credentials, err := client.LoadCredentials(cfg.CredentialFile)
		if err != nil {
			fail("agent is not enrolled", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), cfg.RequestTimeout)
		defer cancel()
		if err := client.New(cfg).Unregister(ctx, credentials); err != nil {
			fail("backend revocation failed", err)
		}
		if err := os.Remove(cfg.CredentialFile); err != nil {
			fail("remove local credentials", err)
		}
		fmt.Println("agent unregistered")
	default:
		fail("unknown command", fmt.Errorf("%s", command))
	}
}

func fail(message string, err error) {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	logger.Error(message, "error", err)
	os.Exit(1)
}
