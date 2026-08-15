package site

import "time"

// Setting is a key-value row for system-wide configuration.
type Setting struct {
	Key       string    `gorm:"primaryKey;size:128" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Setting) TableName() string { return "site_settings" }

// Well-known setting keys.
const (
	KeySiteName          = "site_name"
	KeySiteDescription   = "site_description"
	KeyDefaultLanguage   = "default_language"
	KeyTimezone          = "timezone"
	KeyAllowRegistration = "allow_registration"
	KeyAutoMemory        = "auto_memory"
)

// Defaults returns the initial settings seeded on first run.
func Defaults() map[string]string {
	return map[string]string{
		KeySiteName:          "ollmo",
		KeySiteDescription:   "尽享 AI 奥妙",
		KeyDefaultLanguage:   "zh",
		KeyTimezone:          "Asia/Shanghai",
		// Registration is off by default after install: a fresh deployment
		// should stay closed until the admin explicitly enables it.
		KeyAllowRegistration: "false",
		KeyAutoMemory:        "true",
	}
}

// PublicKeys are non-sensitive settings exposed to unauthenticated clients.
var PublicKeys = []string{
	KeySiteName,
	KeySiteDescription,
	KeyDefaultLanguage,
	KeyTimezone,
	KeyAllowRegistration,
}
