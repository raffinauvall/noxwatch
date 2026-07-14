package servers

import "testing"

func TestValidateUpdate(t *testing.T) {
	name, environment := "api-01", "production"
	tags := []string{"role:api", "region:sg"}
	if fields := validateUpdate(&name, nil, &environment, &tags); len(fields) != 0 {
		t.Fatalf("valid update rejected: %v", fields)
	}
	empty, invalid := "", "invalid"
	badTags := []string{"bad tag"}
	if fields := validateUpdate(&empty, nil, &invalid, &badTags); len(fields) != 3 {
		t.Fatalf("invalid fields=%v", fields)
	}
}
