package site

import (
	"encoding/base64"
	"fmt"
	"strings"

	"ollmo/ollmo/pkg/errs"
)

// LogoMaxBytes caps the decoded logo size. Logos are a few tens of KB; the
// cap protects the settings table and the settings API payload.
const LogoMaxBytes = 512 * 1024

// allowedLogoMimes are the formats the <img> renderer supports. SVG is
// included: rendered via <img src="data:..."> it executes no scripts, and
// the decoded markup is scanned below for obvious hazards.
var allowedLogoMimes = map[string]bool{
	"image/png":       true,
	"image/jpeg":      true,
	"image/webp":      true,
	"image/svg+xml":   true,
}

// ValidateLogo checks a site_logo value: empty clears the logo; otherwise it
// must be a data URL with an allowed image MIME, under the size cap, and —
// for SVG — free of script-related markup.
func ValidateLogo(value string) error {
	if value == "" {
		return nil
	}
	if !strings.HasPrefix(value, "data:") {
		return errs.BadRequest("logo must be a data URL")
	}
	comma := strings.IndexByte(value, ',')
	if comma <= 0 {
		return errs.BadRequest("malformed logo data URL")
	}
	head := value[:comma]
	if !strings.HasPrefix(head, "data:") || !strings.Contains(head, "image/") {
		return errs.BadRequest("logo must be an image")
	}
	mime := strings.TrimSuffix(strings.TrimPrefix(head, "data:"), ";base64")
	if !allowedLogoMimes[mime] {
		return errs.BadRequest("unsupported logo format: " + mime)
	}
	raw, err := base64.StdEncoding.DecodeString(value[comma+1:])
	if err != nil {
		return errs.BadRequest("logo is not valid base64")
	}
	if len(raw) > LogoMaxBytes {
		return errs.BadRequest(fmt.Sprintf("logo too large (max %d KB)", LogoMaxBytes/1024))
	}
	if mime == "image/svg+xml" {
		lower := strings.ToLower(string(raw))
		for _, bad := range []string{"<script", "onload=", "onclick=", "onerror=", "foreignobject", "javascript:"} {
			if strings.Contains(lower, bad) {
				return errs.BadRequest("svg logo contains unsafe markup")
			}
		}
	}
	return nil
}
