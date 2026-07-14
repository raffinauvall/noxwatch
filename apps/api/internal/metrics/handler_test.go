package metrics

import (
	"testing"
	"time"
)

func TestValidatePayload(t *testing.T) {
	now := time.Now().UTC()
	valid := Payload{ServerID: "server", Sequence: 1, CollectedAt: now, CPU: CPUMetrics{UsagePercent: 50}, Memory: MemoryMetrics{UsagePercent: 40}, Swap: SwapMetrics{UsagePercent: 0}}
	if fields := validate(valid, now); len(fields) != 0 {
		t.Fatalf("valid payload rejected: %v", fields)
	}
	valid.CollectedAt = now.Add(3 * time.Minute)
	valid.CPU.UsagePercent = 101
	if fields := validate(valid, now); len(fields) != 2 {
		t.Fatalf("invalid fields = %v", fields)
	}
}
