package notifications

import (
	"net"
	"testing"

	"github.com/raffinauvall/noxwatch/apps/api/internal/config"
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

func TestWebhookURLValidation(t *testing.T) {
	handler := Handler{cfg: config.Config{AppEnv: "production"}}
	if fields := handler.validate("workspace", "Operations", "https://hooks.example.com/noxwatch"); len(fields) != 0 {
		t.Fatalf("valid webhook rejected: %v", fields)
	}
	for _, raw := range []string{"http://hooks.example.com", "https://user:pass@example.com", "http://%"} {
		if fields := handler.validate("workspace", "Operations", raw); fields["url"] == "" {
			t.Fatalf("unsafe webhook accepted: %s", raw)
		}
	}
}
