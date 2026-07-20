package diagnostics

import (
	"regexp"
	"sort"
	"strings"
)

var windowsUserPath = regexp.MustCompile(`(?i)[a-z]:\\users\\[^\\\r\n]+`)

func Sanitize(text string, secrets ...string) string {
	cleaned := text
	values := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if len(secret) >= 3 {
			values = append(values, secret)
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, secret := range values {
		cleaned = strings.ReplaceAll(cleaned, secret, redactionLabel(secret))
		cleaned = strings.ReplaceAll(cleaned, strings.ReplaceAll(secret, "\\", "/"), redactionLabel(secret))
	}
	return windowsUserPath.ReplaceAllString(cleaned, `C:\Users\<USER>`)
}

func redactionLabel(value string) string {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "temp") || strings.Contains(lower, "tmp") {
		return "<TEMP>"
	}
	if !strings.ContainsAny(value, `\/:`) {
		return "<DEVICE_SERIAL>"
	}
	return "<LOCAL_PATH>"
}
