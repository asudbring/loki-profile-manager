package log

import (
	"log/slog"
	"sort"
	"strings"
	"sync"
)

const Redacted = "[REDACTED]"

var sensitiveKeys = map[string]struct{}{
	"access_token":  {},
	"apikey":        {},
	"api_key":       {},
	"authorization": {},
	"bearer":        {},
	"client_secret": {},
	"credential":    {},
	"credentials":   {},
	"passwd":        {},
	"password":      {},
	"private_key":   {},
	"refresh_token": {},
	"secret":        {},
	"secrets":       {},
	"ssh_key":       {},
	"token":         {},
}

var sensitiveKeyTokens = []string{
	"api_key",
	"apikey",
	"authorization",
	"client_secret",
	"credential",
	"password",
	"private_key",
	"refresh_token",
	"secret",
	"ssh_key",
	"token",
}

// Redactor centralizes secret redaction for logs and user-facing diagnostics.
type Redactor struct {
	mu     sync.RWMutex
	values []string
}

func NewRedactor(values ...string) *Redactor {
	r := &Redactor{}
	for _, value := range values {
		r.Register(value)
	}
	return r
}

func (r *Redactor) Register(value string) {
	if r == nil {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.values {
		if existing == value {
			return
		}
	}
	r.values = append(r.values, value)
	sort.Slice(r.values, func(i, j int) bool { return len(r.values[i]) > len(r.values[j]) })
}

func IsSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(key)
	if _, ok := sensitiveKeys[key]; ok {
		return true
	}
	for _, token := range sensitiveKeyTokens {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

func (r *Redactor) RedactString(value string) string {
	if r == nil || value == "" {
		return value
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, secret := range r.values {
		if secret == "" {
			continue
		}
		value = strings.ReplaceAll(value, secret, Redacted)
	}
	return value
}

func (r *Redactor) RedactAttrs(attrs []slog.Attr) []slog.Attr {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		out[i] = r.RedactAttr(attr)
	}
	return out
}

func (r *Redactor) RedactAttr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if IsSensitiveKey(attr.Key) {
		attr.Value = slog.StringValue(Redacted)
		return attr
	}
	attr.Value = r.redactValue(attr.Value)
	return attr
}

func (r *Redactor) redactValue(value slog.Value) slog.Value {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return slog.StringValue(r.RedactString(value.String()))
	case slog.KindGroup:
		return slog.GroupValue(r.RedactAttrs(value.Group())...)
	case slog.KindAny:
		if s, ok := value.Any().(string); ok {
			return slog.StringValue(r.RedactString(s))
		}
	}
	return value
}
