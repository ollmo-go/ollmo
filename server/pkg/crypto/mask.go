package crypto

import "strings"

// MaskSecret returns a display-safe form of a secret: the first and last
// four characters around a fixed ellipsis; short values become all
// asterisks. Used on API responses so plaintext keys never leave the
// server; the masked form is also accepted as "unchanged" by update
// endpoints.
func MaskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + "****" + s[len(s)-4:]
}
