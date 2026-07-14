package alerts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raffinauvall/noxwatch/apps/api/internal/notifications"
)

var (
	ErrForbidden = errors.New("alert access denied")
	ErrNotFound  = errors.New("alert rule not found")
)

type Rule struct {
	ID                string   `json:"id"`
	WorkspaceID       string   `json:"workspace_id"`
	ServerID          string   `json:"server_id"`
	Name              string   `json:"name"`
	Metric            string   `json:"metric"`
	WarningThreshold  *float64 `json:"warning_threshold"`
	CriticalThreshold *float64 `json:"critical_threshold"`
	EvaluationSeconds int      `json:"evaluation_seconds"`
	CooldownSeconds   int      `json:"cooldown_seconds"`
	Enabled           bool     `json:"enabled"`
}

type Event struct {
	ID           string     `json:"id"`
	RuleID       string     `json:"alert_rule_id"`
	ServerID     string     `json:"server_id"`
	RuleName     string     `json:"rule_name"`
	Severity     string     `json:"severity"`
	State        string     `json:"state"`
	CurrentValue float64    `json:"current_value"`
	Threshold    float64    `json:"threshold"`
	TriggeredAt  time.Time  `json:"triggered_at"`
	ResolvedAt   *time.Time `json:"resolved_at"`
}

type Values struct {
	CPU, Memory, Disk, Swap float64
}

type Service struct {
	db       *pgxpool.Pool
	notifier *notifications.Service
	now      func() time.Time
}

func NewService(db *pgxpool.Pool, notifier *notifications.Service) *Service {
	return &Service{db: db, notifier: notifier, now: time.Now}
}

func (s *Service) Create(ctx context.Context, userID, workspaceID, serverID, name, metric string, warning, critical *float64, duration, cooldown int, ip string) (Rule, error) {
	var allowed bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspace_members wm JOIN servers s ON s.workspace_id=wm.workspace_id WHERE wm.workspace_id=$1 AND wm.user_id=$2 AND wm.role IN ('owner','admin') AND s.id=$3)`, workspaceID, userID, serverID).Scan(&allowed)
	if err != nil {
		return Rule{}, err
	}
	if !allowed {
		return Rule{}, ErrForbidden
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Rule{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var rule Rule
	err = tx.QueryRow(ctx, `INSERT INTO alert_rules (workspace_id,server_id,name,metric,warning_threshold,critical_threshold,evaluation_seconds,cooldown_seconds,created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,enabled`, workspaceID, serverID, name, metric, warning, critical, duration, cooldown, userID).Scan(&rule.ID, &rule.Enabled)
	if err != nil {
		return Rule{}, err
	}
	rule.WorkspaceID, rule.ServerID, rule.Name, rule.Metric, rule.WarningThreshold, rule.CriticalThreshold, rule.EvaluationSeconds, rule.CooldownSeconds = workspaceID, serverID, name, metric, warning, critical, duration, cooldown
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (workspace_id,actor_user_id,action,target_type,target_id,ip_address) VALUES ($1,$2,'alert_rule.create','alert_rule',$3,NULLIF($4,'')::inet)`, workspaceID, userID, rule.ID, ip); err != nil {
		return Rule{}, err
	}
	return rule, tx.Commit(ctx)
}

func (s *Service) List(ctx context.Context, userID, workspaceID string) ([]Rule, error) {
	rows, err := s.db.Query(ctx, `SELECT ar.id,ar.workspace_id,ar.server_id,ar.name,ar.metric,ar.warning_threshold,ar.critical_threshold,ar.evaluation_seconds,ar.cooldown_seconds,ar.enabled FROM alert_rules ar JOIN workspace_members wm ON wm.workspace_id=ar.workspace_id AND wm.user_id=$1 WHERE ar.workspace_id=$2 ORDER BY ar.created_at DESC`, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Rule{}
	for rows.Next() {
		var rule Rule
		if err := scanRule(rows, &rule); err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

func (s *Service) Update(ctx context.Context, userID, ruleID string, name *string, warning, critical *float64, duration, cooldown *int, enabled *bool) (Rule, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Rule{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var rule Rule
	err = tx.QueryRow(ctx, `UPDATE alert_rules ar SET name=COALESCE($3,name),warning_threshold=COALESCE($4,warning_threshold),critical_threshold=COALESCE($5,critical_threshold),evaluation_seconds=COALESCE($6,evaluation_seconds),cooldown_seconds=COALESCE($7,cooldown_seconds),enabled=COALESCE($8,enabled),updated_at=now()
	 FROM workspace_members wm WHERE ar.id=$1 AND wm.workspace_id=ar.workspace_id AND wm.user_id=$2 AND wm.role IN ('owner','admin') RETURNING ar.id,ar.workspace_id,ar.server_id,ar.name,ar.metric,ar.warning_threshold,ar.critical_threshold,ar.evaluation_seconds,ar.cooldown_seconds,ar.enabled`, ruleID, userID, name, warning, critical, duration, cooldown, enabled).
		Scan(&rule.ID, &rule.WorkspaceID, &rule.ServerID, &rule.Name, &rule.Metric, &rule.WarningThreshold, &rule.CriticalThreshold, &rule.EvaluationSeconds, &rule.CooldownSeconds, &rule.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rule{}, ErrNotFound
	}
	if err != nil {
		return Rule{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (workspace_id,actor_user_id,action,target_type,target_id) VALUES ($1,$2,'alert_rule.update','alert_rule',$3)`, rule.WorkspaceID, userID, rule.ID); err != nil {
		return Rule{}, err
	}
	return rule, tx.Commit(ctx)
}

func (s *Service) Delete(ctx context.Context, userID, ruleID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var workspaceID string
	err = tx.QueryRow(ctx, `DELETE FROM alert_rules ar USING workspace_members wm WHERE ar.id=$1 AND wm.workspace_id=ar.workspace_id AND wm.user_id=$2 AND wm.role IN ('owner','admin') RETURNING ar.workspace_id`, ruleID, userID).Scan(&workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs (workspace_id,actor_user_id,action,target_type,target_id) VALUES ($1,$2,'alert_rule.delete','alert_rule',$3)`, workspaceID, userID, ruleID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) Events(ctx context.Context, userID, serverID string) ([]Event, error) {
	rows, err := s.db.Query(ctx, `SELECT ae.id,ae.alert_rule_id,ae.server_id,ar.name,ae.severity,ae.state,COALESCE(ae.current_value,0),COALESCE(ae.threshold,0),ae.triggered_at,ae.resolved_at FROM alert_events ae JOIN alert_rules ar ON ar.id=ae.alert_rule_id JOIN workspace_members wm ON wm.workspace_id=ae.workspace_id AND wm.user_id=$1 WHERE ae.server_id=$2 ORDER BY ae.triggered_at DESC LIMIT 100`, userID, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Event{}
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.RuleID, &event.ServerID, &event.RuleName, &event.Severity, &event.State, &event.CurrentValue, &event.Threshold, &event.TriggeredAt, &event.ResolvedAt); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (s *Service) EvaluateMetrics(ctx context.Context, serverID string, collectedAt time.Time, values Values) error {
	rules, err := s.rulesForServer(ctx, serverID, []string{"cpu_usage", "memory_usage", "disk_usage", "swap_usage"})
	if err != nil {
		return err
	}
	for _, rule := range rules {
		value := map[string]float64{"cpu_usage": values.CPU, "memory_usage": values.Memory, "disk_usage": values.Disk, "swap_usage": values.Swap}[rule.Metric]
		if err := s.evaluate(ctx, rule, value, collectedAt, collectedAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) EvaluateConnectivity(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `SELECT ar.id,ar.workspace_id,ar.server_id,ar.name,ar.metric,ar.warning_threshold,ar.critical_threshold,ar.evaluation_seconds,ar.cooldown_seconds,ar.enabled,COALESCE(s.last_seen_at,s.enrolled_at,s.created_at) FROM alert_rules ar JOIN servers s ON s.id=ar.server_id WHERE ar.enabled=true AND ar.metric IN ('server_offline','agent_disconnected')`)
	if err != nil {
		return err
	}
	defer rows.Close()
	now := s.now().UTC()
	type item struct {
		rule Rule
		last time.Time
	}
	var items []item
	for rows.Next() {
		var current item
		if err := scanRuleWithTime(rows, &current.rule, &current.last); err != nil {
			return err
		}
		items = append(items, current)
	}
	for _, item := range items {
		missingSeconds := now.Sub(item.last).Seconds()
		violating := missingSeconds >= float64(item.rule.EvaluationSeconds)
		if err := s.evaluateConnectivityRule(ctx, item.rule, missingSeconds, item.last, now, violating); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) evaluateConnectivityRule(ctx context.Context, rule Rule, value float64, breachSince, now time.Time, violating bool) error {
	if violating {
		return s.evaluateWithSeverity(ctx, rule, value, "critical", float64(rule.EvaluationSeconds), breachSince, now)
	}
	return s.resolve(ctx, rule, value, now)
}

func (s *Service) evaluate(ctx context.Context, rule Rule, value float64, breachSince, now time.Time) error {
	severity, threshold, violating := severity(rule, value)
	if !violating {
		return s.resolve(ctx, rule, value, now)
	}
	return s.evaluateWithSeverity(ctx, rule, value, severity, threshold, breachSince, now)
}

func (s *Service) evaluateWithSeverity(ctx context.Context, rule Rule, value float64, severity string, threshold float64, breachSince, now time.Time) error {
	var event Event
	var notified bool
	err := s.db.QueryRow(ctx, `SELECT id,state,severity,triggered_at,notified_at IS NOT NULL FROM alert_events WHERE alert_rule_id=$1 AND server_id=$2 AND state IN ('pending','firing','acknowledged') LIMIT 1`, rule.ID, rule.ServerID).Scan(&event.ID, &event.State, &event.Severity, &event.TriggeredAt, &notified)
	if errors.Is(err, pgx.ErrNoRows) {
		state := "pending"
		if now.Sub(breachSince) >= time.Duration(rule.EvaluationSeconds)*time.Second {
			state = "firing"
		}
		err = s.db.QueryRow(ctx, `INSERT INTO alert_events (workspace_id,alert_rule_id,server_id,severity,state,current_value,threshold,triggered_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, rule.WorkspaceID, rule.ID, rule.ServerID, severity, state, value, threshold, breachSince).Scan(&event.ID)
		if err != nil {
			return err
		}
		if state == "firing" {
			return s.notifyFiring(ctx, rule, event.ID, now)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if event.State == "pending" && now.Sub(event.TriggeredAt) >= time.Duration(rule.EvaluationSeconds)*time.Second {
		if _, err := s.db.Exec(ctx, `UPDATE alert_events SET state='firing',severity=$2,current_value=$3,threshold=$4 WHERE id=$1`, event.ID, severity, value, threshold); err != nil {
			return err
		}
		return s.notifyFiring(ctx, rule, event.ID, now)
	}
	if _, err = s.db.Exec(ctx, `UPDATE alert_events SET severity=$2,current_value=$3,threshold=$4 WHERE id=$1`, event.ID, severity, value, threshold); err != nil {
		return err
	}
	if event.State == "firing" && !notified {
		return s.notifyFiring(ctx, rule, event.ID, now)
	}
	return nil
}

func (s *Service) resolve(ctx context.Context, rule Rule, value float64, now time.Time) error {
	var eventID, previousState string
	var notified bool
	err := s.db.QueryRow(ctx, `WITH previous AS (
	 SELECT id,state,notified_at IS NOT NULL AS notified FROM alert_events WHERE alert_rule_id=$1 AND server_id=$2 AND state IN ('pending','firing','acknowledged') FOR UPDATE
	), resolved AS (
	 UPDATE alert_events ae SET state='resolved',current_value=$3,resolved_at=$4 FROM previous p WHERE ae.id=p.id RETURNING ae.id,p.state,p.notified
	) SELECT id,state,notified FROM resolved`, rule.ID, rule.ServerID, value, now).Scan(&eventID, &previousState, &notified)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if previousState == "pending" || !notified {
		return nil
	}
	return s.notify(ctx, rule, eventID)
}

func (s *Service) notifyFiring(ctx context.Context, rule Rule, eventID string, now time.Time) error {
	var recentlyNotified bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (
	 SELECT 1 FROM alert_events WHERE alert_rule_id=$1 AND id<>$2 AND notified_at IS NOT NULL AND notified_at > $3::timestamptz - make_interval(secs => $4)
	)`, rule.ID, eventID, now, rule.CooldownSeconds).Scan(&recentlyNotified)
	if err != nil || recentlyNotified {
		return err
	}
	if err := s.notify(ctx, rule, eventID); err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `UPDATE alert_events SET notified_at=$2 WHERE id=$1`, eventID, now)
	return err
}

func (s *Service) notify(ctx context.Context, rule Rule, eventID string) error {
	var event notifications.Event
	err := s.db.QueryRow(ctx, `SELECT ae.workspace_id,w.name,s.name,ae.server_id,ae.severity,ae.state,ar.name,COALESCE(ae.current_value,0),COALESCE(ae.threshold,0),ae.triggered_at,ae.resolved_at FROM alert_events ae JOIN alert_rules ar ON ar.id=ae.alert_rule_id JOIN workspaces w ON w.id=ae.workspace_id JOIN servers s ON s.id=ae.server_id WHERE ae.id=$1`, eventID).
		Scan(&event.WorkspaceID, &event.Workspace, &event.Server, &event.ServerID, &event.Severity, &event.State, &event.Rule, &event.CurrentValue, &event.Threshold, &event.TriggeredAt, &event.ResolvedAt)
	if err != nil {
		return err
	}
	return s.notifier.Notify(ctx, event)
}

func (s *Service) rulesForServer(ctx context.Context, serverID string, metrics []string) ([]Rule, error) {
	rows, err := s.db.Query(ctx, `SELECT id,workspace_id,server_id,name,metric,warning_threshold,critical_threshold,evaluation_seconds,cooldown_seconds,enabled FROM alert_rules WHERE server_id=$1 AND enabled=true AND metric=ANY($2)`, serverID, metrics)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Rule
	for rows.Next() {
		var rule Rule
		if err := scanRule(rows, &rule); err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanRule(row scanner, rule *Rule) error {
	return row.Scan(&rule.ID, &rule.WorkspaceID, &rule.ServerID, &rule.Name, &rule.Metric, &rule.WarningThreshold, &rule.CriticalThreshold, &rule.EvaluationSeconds, &rule.CooldownSeconds, &rule.Enabled)
}

func scanRuleWithTime(row scanner, rule *Rule, at *time.Time) error {
	return row.Scan(&rule.ID, &rule.WorkspaceID, &rule.ServerID, &rule.Name, &rule.Metric, &rule.WarningThreshold, &rule.CriticalThreshold, &rule.EvaluationSeconds, &rule.CooldownSeconds, &rule.Enabled, at)
}

func severity(rule Rule, value float64) (string, float64, bool) {
	if rule.CriticalThreshold != nil && value >= *rule.CriticalThreshold {
		return "critical", *rule.CriticalThreshold, true
	}
	if rule.WarningThreshold != nil && value >= *rule.WarningThreshold {
		return "warning", *rule.WarningThreshold, true
	}
	return "", 0, false
}
