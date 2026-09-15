package observability

import (
	"net/url"
	"strings"
)

// sensitiveQueryParams are query parameters whose values must never reach a
// span attribute. `access_token` is the load-bearing one: EventSource cannot
// set an Authorization header, so SSE endpoints (e.g. /impact/v1/stream) pass
// a real bearer token in the query string. Recording it verbatim would persist
// live credentials in the trace backend, where they outlive the request and
// are readable by anyone with dashboard access. Weather stations add their
// own: Wunderground uploads send PASSWORD, Ecowitt sends PASSKEY.
//
//nolint:gochecknoglobals // fixed lookup table
var sensitiveQueryParams = map[string]bool{
	"access_token":  true,
	"token":         true,
	"refresh_token": true,
	"id_token":      true,
	"code":          true,
	"client_secret": true,
	"api_key":       true,
	"apikey":        true,
	"key":           true,
	"password":      true,
	"passkey":       true,
	"pwd":           true,
	"secret":        true,
}

// redactedValue replaces every secret the redactors find.
const redactedValue = "REDACTED"

// PathRedactor rewrites a request path before it is recorded, typically to
// mask a credential carried as a path segment.
type PathRedactor func(path string) string

// RedactURL renders a URL with the values of sensitive query parameters and
// any userinfo password replaced by "REDACTED". Parameter NAMES are preserved
// so traces still show the shape of the request. A URL whose query cannot be
// parsed is reduced to its path, which is the safe direction to fail.
func RedactURL(u *url.URL) string {
	return RedactURLWithPath(u, nil)
}

// RedactURLWithPath is RedactURL with the path first passed through redact.
// A nil redact leaves the path unchanged.
func RedactURLWithPath(u *url.URL, redact PathRedactor) string {
	if u == nil {
		return ""
	}

	clone := *u
	if redact != nil {
		clone.Path = redact(clone.Path)
		clone.RawPath = ""
	}
	if clone.User != nil {
		if _, hasPassword := clone.User.Password(); hasPassword {
			clone.User = url.UserPassword(clone.User.Username(), redactedValue)
		}
	}
	if clone.RawQuery == "" {
		return clone.String()
	}

	values, err := url.ParseQuery(clone.RawQuery)
	if err != nil {
		return clone.Path
	}

	redacted := false
	for key := range values {
		if sensitiveQueryParams[strings.ToLower(key)] {
			values.Set(key, redactedValue)
			redacted = true
		}
	}
	if redacted {
		clone.RawQuery = values.Encode()
	}
	return clone.String()
}
