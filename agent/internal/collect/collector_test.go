package collect

import (
	"strings"
	"testing"
)

func TestParsersAndRates(t *testing.T) {
	memory, swap, err := parseMemInfo(strings.NewReader("MemTotal: 1000 kB\nMemAvailable: 400 kB\nSwapTotal: 200 kB\nSwapFree: 150 kB\n"))
	if err != nil || memory.UsedBytes != 600*1024 || memory.UsagePercent != 60 || swap.UsedBytes != 50*1024 || swap.UsagePercent != 25 {
		t.Fatalf("memory=%+v swap=%+v err=%v", memory, swap, err)
	}
	networks, err := parseNetworks(strings.NewReader("Inter-| Receive | Transmit\n eth0: 1000 10 2 0 0 0 0 0 2000 20 3 0 0 0 0 0\n"))
	if err != nil || len(networks) != 1 || networks[0].RXBytesTotal != 1000 || networks[0].TXErrorsTotal != 3 {
		t.Fatalf("networks=%+v err=%v", networks, err)
	}
	usage := cpuUsage(cpuCounters{total: 100, idle: 50}, cpuCounters{total: 200, idle: 70})
	if usage != 80 {
		t.Fatalf("cpu usage = %v", usage)
	}
}
