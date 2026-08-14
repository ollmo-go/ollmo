package clients

import (
	"fmt"
	"net/url"
)

// ValidateEndpoint checks a user-configured provider endpoint. Self-hosted
// models on intranet hosts (Ollama, vLLM) are a core use case, so private
// addresses stay allowed; only an absolute http(s) URL is enforced.
func ValidateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("endpoint must start with http:// or https://")
	}
	if u.Host == "" {
		return fmt.Errorf("endpoint host is required")
	}
	return nil
}

// Snippet trims a response body for inclusion in error messages returned to
// API clients: a bounded echo for debugging without leaking full upstream
// payloads (SSRF response disclosure).
func Snippet(b []byte) string {
	const max = 200
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}
