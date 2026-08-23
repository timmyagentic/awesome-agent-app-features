package feedback

import "regexp"

var (
	privateKeyRE      = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	authorizationRE   = regexp.MustCompile(`(?i)\b(authorization)(\s*:\s*)[^\r\n]+`)
	secretKVRE        = regexp.MustCompile(`(?i)\b(app[_-]?secret|api[_-]?key|apikey|secret|token|password|access[_-]?key|client[_-]?secret)\b(\s*[=:]\s*)("[^"]*"|'[^']*'|[^\s,;]+)`)
	identifierKVRE    = regexp.MustCompile(`(?i)\b(user[_-]?id|chat[_-]?id|open[_-]?id|account[_-]?id|tenant[_-]?id)\b(\s*[=:]\s*)("[^"]*"|'[^']*'|[^\s,;]+)`)
	emailRE           = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
	unixHomePathRE    = regexp.MustCompile(`(/Users/|/home/)[^\s"']+`)
	windowsHomePathRE = regexp.MustCompile(`[A-Za-z]:\\Users\\[^\s"']+`)
	knownIDRE         = regexp.MustCompile(`\b(ou|oc|om|on|cli)_[0-9A-Za-z_-]{8,}\b`)
	jwtRE             = regexp.MustCompile(`\b[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)
	longBlobRE        = regexp.MustCompile(`[A-Za-z0-9+/_=-]{32,}`)
)

// Redact aggressively removes common credential, identifier, email, and home
// path shapes. A host may wrap this function with additional product-specific
// patterns through Builder.Redact.
func Redact(text string) string {
	text = privateKeyRE.ReplaceAllString(text, "[REDACTED-PRIVATE-KEY]")
	text = authorizationRE.ReplaceAllString(text, "$1$2[REDACTED]")
	text = secretKVRE.ReplaceAllString(text, "$1$2[REDACTED]")
	text = identifierKVRE.ReplaceAllString(text, "$1$2[REDACTED-ID]")
	text = emailRE.ReplaceAllString(text, "[REDACTED-EMAIL]")
	text = unixHomePathRE.ReplaceAllString(text, "[REDACTED-PATH]")
	text = windowsHomePathRE.ReplaceAllString(text, "[REDACTED-PATH]")
	text = knownIDRE.ReplaceAllString(text, "[REDACTED-ID]")
	text = jwtRE.ReplaceAllString(text, "[REDACTED]")
	text = longBlobRE.ReplaceAllString(text, "[REDACTED]")
	return text
}
