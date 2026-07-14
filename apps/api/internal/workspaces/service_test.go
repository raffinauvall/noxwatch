package workspaces

import "testing"

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Night Operations": "night-operations",
		"  API / EU  ":     "api-eu",
		"---":              "workspace",
	}
	for input, want := range cases {
		if got := slug(input); got != want {
			t.Fatalf("slug(%q) = %q, want %q", input, got, want)
		}
	}
}
