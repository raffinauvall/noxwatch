package notifications

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrForbidden = errors.New("notification access denied")
	ErrNotFound  = errors.New("notification channel not found")
)

type Channel struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	Secret    string    `json:"secret,omitempty"`
}

type Event struct {
	WorkspaceID   string     `json:"-"`
	Workspace     string     `json:"workspace"`
	Server        string     `json:"server"`
	ServerID      string     `json:"server_id"`
	Severity      string     `json:"severity"`
	State         string     `json:"state"`
	Rule          string     `json:"alert_rule"`
	CurrentValue  float64    `json:"current_value"`
	Threshold     float64    `json:"threshold"`
	TriggeredAt   time.Time  `json:"triggered_at"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	DashboardLink string     `json:"dashboard_link"`
}

type Service struct {
	db     *pgxpool.Pool
	key    [32]byte
	client *http.Client
	webURL string
}

func NewService(db *pgxpool.Pool, secret, webURL string, allowPrivate bool) *Service {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if !allowPrivate {
		transport.DialContext = dialPublic
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || (!allowPrivate && req.URL.Scheme != "https") {
			return errors.New("unsafe webhook redirect")
		}
		return nil
	}
	return &Service{db: db, key: sha256.Sum256([]byte(secret)), client: client, webURL: strings.TrimRight(webURL, "/")}
}

func (s *Service) Create(ctx context.Context, userID, workspaceID, name, webhookURL, ip string) (Channel, error) {
	var allowed bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspace_members WHERE workspace_id=$1 AND user_id=$2 AND role IN ('owner','admin'))`, workspaceID, userID).Scan(&allowed); err != nil {
		return Channel{}, err
	}
	if !allowed {
		return Channel{}, ErrForbidden
	}
	encrypted, err := s.encrypt(webhookURL)
	if err != nil {
		return Channel{}, err
	}
	config, _ := json.Marshal(map[string]string{"url_encrypted": encrypted})
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Channel{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var channel Channel
	err = tx.QueryRow(ctx, `INSERT INTO notification_channels (workspace_id,type,name,config) VALUES ($1,'webhook',$2,$3) RETURNING id,type,enabled,created_at`, workspaceID, name, config).
		Scan(&channel.ID, &channel.Type, &channel.Enabled, &channel.CreatedAt)
	if err != nil {
		return Channel{}, err
	}
	channel.Name, channel.Secret = name, s.signingSecret(channel.ID)
	if _, err := tx.Exec(ctx, `UPDATE notification_channels SET secret_hash=$1 WHERE id=$2`, hash(channel.Secret), channel.ID); err != nil {
		return Channel{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (workspace_id,actor_user_id,action,target_type,target_id,ip_address) VALUES ($1,$2,'notification.create','notification_channel',$3,NULLIF($4,'')::inet)`, workspaceID, userID, channel.ID, ip); err != nil {
		return Channel{}, err
	}
	return channel, tx.Commit(ctx)
}

func (s *Service) List(ctx context.Context, userID, workspaceID string) ([]Channel, error) {
	rows, err := s.db.Query(ctx, `SELECT nc.id,nc.name,nc.type,nc.enabled,nc.created_at FROM notification_channels nc JOIN workspace_members wm ON wm.workspace_id=nc.workspace_id AND wm.user_id=$1 WHERE nc.workspace_id=$2 ORDER BY nc.created_at`, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Channel{}
	for rows.Next() {
		var channel Channel
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Type, &channel.Enabled, &channel.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, channel)
	}
	return result, rows.Err()
}

func (s *Service) Delete(ctx context.Context, userID, channelID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var workspaceID string
	err = tx.QueryRow(ctx, `DELETE FROM notification_channels nc USING workspace_members wm WHERE nc.id=$1 AND wm.workspace_id=nc.workspace_id AND wm.user_id=$2 AND wm.role IN ('owner','admin') RETURNING nc.workspace_id`, channelID, userID).Scan(&workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (workspace_id,actor_user_id,action,target_type,target_id) VALUES ($1,$2,'notification.delete','notification_channel',$3)`, workspaceID, userID, channelID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) Notify(ctx context.Context, event Event) error {
	rows, err := s.db.Query(ctx, `SELECT id,config->>'url_encrypted' FROM notification_channels WHERE workspace_id=$1 AND type='webhook' AND enabled=true`, event.WorkspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type destination struct{ id, encrypted string }
	var destinations []destination
	for rows.Next() {
		var item destination
		if err := rows.Scan(&item.id, &item.encrypted); err != nil {
			return err
		}
		destinations = append(destinations, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	event.DashboardLink = s.webURL + "/servers/" + event.ServerID
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	for _, destination := range destinations {
		webhookURL, err := s.decrypt(destination.encrypted)
		if err != nil {
			return err
		}
		if err := s.send(ctx, webhookURL, destination.id, body); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) send(ctx context.Context, webhookURL, channelID string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(s.signingSecret(channelID)))
	_, _ = mac.Write(body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-NoxWatch-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	response, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (s *Service) encrypt(value string) (string, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(value), nil)), nil
}

func (s *Service) decrypt(value string) (string, error) {
	encoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(encoded) < gcm.NonceSize() {
		return "", errors.New("invalid encrypted webhook URL")
	}
	plain, err := gcm.Open(nil, encoded[:gcm.NonceSize()], encoded[gcm.NonceSize():], nil)
	return string(plain), err
}

func (s *Service) signingSecret(channelID string) string {
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write([]byte("webhook:" + channelID))
	return "nox_whsec_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func dialPublic(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, errors.New("webhook host did not resolve")
	}
	for _, address := range addresses {
		if !isPublicIP(address.IP) {
			return nil, errors.New("webhook host resolves to a non-public address")
		}
	}
	return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
}

func isPublicIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}
