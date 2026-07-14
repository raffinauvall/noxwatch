package enrollment

import "testing"

func TestTokenInputAndTags(t *testing.T) {
	if fields := validateTokenInput("workspace", "api-01", "production", "", []string{"region:sg", "role_api"}); len(fields) != 0 {
		t.Fatalf("valid input rejected: %v", fields)
	}
	if fields := validateTokenInput("", "", "invalid", "", []string{"bad tag"}); len(fields) != 4 {
		t.Fatalf("invalid input fields = %v", fields)
	}
	tags := uniqueTags([]string{"Prod", "prod", "API"})
	if len(tags) != 2 || tags[0] != "prod" || tags[1] != "api" {
		t.Fatalf("unexpected tags: %v", tags)
	}
}
