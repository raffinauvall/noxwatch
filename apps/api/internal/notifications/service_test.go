package notifications

import (
	"net"
	"testing"
)

func TestPublicWebhookAddressPolicy(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1"} {
		if isPublicIP(net.ParseIP(raw)) {
			t.Fatalf("unsafe address considered public: %s", raw)
		}
	}
	if !isPublicIP(net.ParseIP("1.1.1.1")) {
		t.Fatal("public address rejected")
	}
}
