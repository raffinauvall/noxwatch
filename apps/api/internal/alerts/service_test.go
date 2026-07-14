package alerts

import "testing"

func TestSeverity(t *testing.T) {
	warning, critical := 80.0, 90.0
	rule := Rule{WarningThreshold: &warning, CriticalThreshold: &critical}
	tests := []struct {
		value     float64
		severity  string
		violating bool
	}{{79.9, "", false}, {80, "warning", true}, {95, "critical", true}}
	for _, test := range tests {
		got, _, violating := severity(rule, test.value)
		if got != test.severity || violating != test.violating {
			t.Fatalf("severity(%v) = %q, %v", test.value, got, violating)
		}
	}
}

func TestValidateRule(t *testing.T) {
	warning, critical := 80.0, 90.0
	valid := ruleInput{WorkspaceID: "workspace", ServerID: "server", Name: "High CPU", Metric: "cpu_usage", WarningThreshold: &warning, CriticalThreshold: &critical, EvaluationSeconds: 300, CooldownSeconds: 900}
	if fields := validateRule(valid); len(fields) != 0 {
		t.Fatalf("valid rule rejected: %v", fields)
	}
	invalid := valid
	invalid.Metric, invalid.Name, invalid.CriticalThreshold = "temperature", "", &warning
	if fields := validateRule(invalid); len(fields) != 2 {
		t.Fatalf("invalid rule fields = %v", fields)
	}
}
