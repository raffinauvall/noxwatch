package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/raffinauvall/noxwatch/agent/internal/collect"
	"github.com/raffinauvall/noxwatch/agent/internal/config"
)

type Credentials struct {
	ServerID         string `json:"server_id"`
	AgentID          string `json:"agent_id"`
	Credential       string `json:"credential"`
	HeartbeatSeconds int    `json:"heartbeat_interval_seconds"`
	MetricsSeconds   int    `json:"metrics_interval_seconds"`
}

type Client struct {
	endpoint string
	http     *http.Client
}

type HTTPError struct{ Status int }

func (e HTTPError) Error() string { return fmt.Sprintf("backend returned HTTP %d", e.Status) }

func (e HTTPError) Retryable() bool { return e.Status == http.StatusTooManyRequests || e.Status >= 500 }

func New(cfg config.Config) *Client {
	return &Client{endpoint: strings.TrimRight(cfg.Endpoint, "/"), http: &http.Client{Timeout: cfg.RequestTimeout}}
}

func (c *Client) Enroll(ctx context.Context, token string, identity collect.Identity) (Credentials, int, int, error) {
	input := map[string]any{
		"token": token, "hostname": identity.Hostname, "machine_id": identity.MachineID, "os": identity.OS,
		"os_version": identity.OSVersion, "kernel_version": identity.KernelVersion, "architecture": identity.Architecture, "agent_version": identity.AgentVersion,
	}
	var output struct {
		ServerID         string `json:"server_id"`
		AgentID          string `json:"agent_id"`
		Credential       string `json:"credential"`
		HeartbeatSeconds int    `json:"heartbeat_interval_seconds"`
		MetricsSeconds   int    `json:"metrics_interval_seconds"`
	}
	if err := c.request(ctx, http.MethodPost, "/api/v1/agent/enroll", input, "", &output); err != nil {
		return Credentials{}, 0, 0, err
	}
	return Credentials{ServerID: output.ServerID, AgentID: output.AgentID, Credential: output.Credential, HeartbeatSeconds: output.HeartbeatSeconds, MetricsSeconds: output.MetricsSeconds}, output.HeartbeatSeconds, output.MetricsSeconds, nil
}

func (c *Client) Heartbeat(ctx context.Context, credentials Credentials) error {
	return c.request(ctx, http.MethodPost, "/api/v1/agent/heartbeat", map[string]string{"server_id": credentials.ServerID}, credentials.Credential, nil)
}

func (c *Client) Metrics(ctx context.Context, credentials Credentials, payload collect.Payload) error {
	return c.request(ctx, http.MethodPost, "/api/v1/agent/metrics", payload, credentials.Credential, nil)
}

func (c *Client) Unregister(ctx context.Context, credentials Credentials) error {
	return c.request(ctx, http.MethodPost, "/api/v1/agent/unregister", map[string]string{"server_id": credentials.ServerID}, credentials.Credential, nil)
}

func (c *Client) request(ctx context.Context, method, path string, input any, credential string, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "noxwatch-agent")
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return HTTPError{Status: response.StatusCode}
	}
	if output == nil {
		return nil
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return err
	}
	return json.Unmarshal(envelope.Data, output)
}

func LoadCredentials(path string) (Credentials, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Credentials{}, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Credentials{}, errors.New("credential file permissions must be 0600")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, err
	}
	var credentials Credentials
	if err := json.Unmarshal(body, &credentials); err != nil {
		return Credentials{}, err
	}
	if credentials.ServerID == "" || credentials.AgentID == "" || credentials.Credential == "" {
		return Credentials{}, errors.New("credential file is incomplete")
	}
	return credentials, nil
}

func SaveCredentials(path string, credentials Credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".credential-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName) //nolint:errcheck
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if err := json.NewEncoder(temp).Encode(credentials); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}
