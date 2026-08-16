package site

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidateLogo(t *testing.T) {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	cases := []struct {
		name  string
		value string
		ok    bool
	}{
		{"empty clears", "", true},
		{"png data url ok", "data:image/png;base64,aGVsbG8=", true},
		{"jpeg data url ok", "data:image/jpeg;base64,aGVsbG8=", true},
		{"svg data url ok", "data:image/svg+xml;base64," + b64(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="4" height="4"/></svg>`), true},
		{"not a data url", "https://example.com/logo.png", false},
		{"not an image", "data:text/plain;base64,aGVsbG8=", false},
		{"unsupported mime", "data:image/gif;base64,aGVsbG8=", false},
		{"bad base64", "data:image/png;base64,!!!", false},
		{"svg with script rejected", "data:image/svg+xml;base64," + b64("<svg><script>alert(1)</script></svg>"), false},
		{"svg with onload rejected", "data:image/svg+xml;base64," + b64(`<svg onload="x()"></svg>`), false},
		{"svg with javascript url rejected", "data:image/svg+xml;base64," + b64(`<svg><a href="javascript:x()">x</a></svg>`), false},
		{"oversized rejected", "data:image/png;base64," + strings.Repeat("A", (LogoMaxBytes+16)*4/3), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateLogo(c.value)
			if c.ok && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !c.ok && err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
