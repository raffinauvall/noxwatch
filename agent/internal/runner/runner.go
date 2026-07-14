package runner

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"
	"strings"
	"time"

	"github.com/raffinauvall/noxwatch/agent/internal/client"
	"github.com/raffinauvall/noxwatch/agent/internal/collect"
	"github.com/raffinauvall/noxwatch/agent/internal/config"
)

const maxQueue = 100

func Run(ctx context.Context, cfg config.Config, version string, logger *slog.Logger) error {
	api := client.New(cfg)
	credentials, err := client.LoadCredentials(cfg.CredentialFile)
	if errors.Is(err, os.ErrNotExist) {
		credentials, err = enroll(ctx, api, cfg, version)
	}
	if err != nil {
		return err
	}
	heartbeatInterval := interval(credentials.HeartbeatSeconds, 20)
	metricsInterval := interval(credentials.MetricsSeconds, 45)
	collector := collect.New()
	queue := make([]collect.Payload, 0, maxQueue)
	sequence := time.Now().UnixNano()
	heartbeatTimer, metricsTimer := time.NewTimer(jitter(heartbeatInterval)), time.NewTimer(jitter(metricsInterval))
	defer heartbeatTimer.Stop()
	defer metricsTimer.Stop()

	logger.Info("agent started", "server_id", credentials.ServerID, "heartbeat_seconds", int(heartbeatInterval.Seconds()), "metrics_seconds", int(metricsInterval.Seconds()))
	for {
		select {
		case <-ctx.Done():
			logger.Info("agent stopping")
			return nil
		case <-heartbeatTimer.C:
			if err := retry(ctx, func() error { return api.Heartbeat(ctx, credentials) }); err != nil && ctx.Err() == nil {
				logger.Warn("heartbeat failed", "error", safeError(err))
			}
			heartbeatTimer.Reset(jitter(heartbeatInterval))
		case <-metricsTimer.C:
			sequence++
			payload, err := collector.Collect(credentials.ServerID, sequence)
			if err != nil {
				logger.Warn("metrics collection failed", "error", err)
			} else {
				queue = appendBounded(queue, payload)
			}
			for len(queue) > 0 {
				if err := retry(ctx, func() error { return api.Metrics(ctx, credentials, queue[0]) }); err != nil {
					if ctx.Err() == nil {
						logger.Warn("metrics delivery failed", "queued", len(queue), "error", safeError(err))
					}
					break
				}
				queue = queue[1:]
			}
			metricsTimer.Reset(jitter(metricsInterval))
		}
	}
}

func enroll(ctx context.Context, api *client.Client, cfg config.Config, version string) (client.Credentials, error) {
	token, err := os.ReadFile(cfg.EnrollmentFile)
	if err != nil {
		return client.Credentials{}, err
	}
	identity, err := collect.CollectIdentity(version)
	if err != nil {
		return client.Credentials{}, err
	}
	credentials, _, _, err := api.Enroll(ctx, strings.TrimSpace(string(token)), identity)
	if err != nil {
		return client.Credentials{}, err
	}
	if err := client.SaveCredentials(cfg.CredentialFile, credentials); err != nil {
		return client.Credentials{}, err
	}
	if err := os.Remove(cfg.EnrollmentFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return client.Credentials{}, err
	}
	return credentials, nil
}

func retry(ctx context.Context, operation func() error) error {
	backoff := time.Second
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		last = operation()
		if last == nil || !retryable(last) {
			return last
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		backoff = min(backoff*2, 30*time.Second)
	}
	return last
}

func retryable(err error) bool {
	var httpErr client.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Retryable()
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func appendBounded(queue []collect.Payload, payload collect.Payload) []collect.Payload {
	if len(queue) == maxQueue {
		copy(queue, queue[1:])
		queue = queue[:maxQueue-1]
	}
	return append(queue, payload)
}

func interval(value, fallback int) time.Duration {
	if value <= 0 {
		value = fallback
	}
	return time.Duration(value) * time.Second
}

func jitter(value time.Duration) time.Duration {
	return value + time.Duration(rand.Int64N(max(1, int64(value/10))))
}

func safeError(err error) string {
	var httpErr client.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Error()
	}
	return "network request failed"
}
