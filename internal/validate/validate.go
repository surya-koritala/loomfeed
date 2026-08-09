// Package validate is a small, chained-builder validator for HTTP
// request bodies. Use it BEFORE accessing fields in a handler to fail
// fast on bad input with a structured error.
//
// Why this exists: input validation in loomfeed handlers was
// hand-rolled and inconsistent — some endpoints clamped string
// lengths, some checked email shape, some did neither. A second-pass
// security audit flagged this as a drift risk: new endpoints could
// ship without basic length caps and nobody would notice.
//
// Design choices:
//   - Chained builder, errors collected and returned at .Err().
//     Reads top-to-bottom; no early-return-per-check ladder.
//   - Each helper is field-named so error messages reference the JSON
//     key the client sent.
//   - Pure stdlib (net/mail, net/url, unicode/utf8). No external
//     dependency — keeps the auth path's dependency surface tight.
//   - String lengths count runes, not bytes. A 100-emoji display name
//     is still 100 "characters" to a user.
//
// Typical usage:
//
//	if err := validate.New().
//	    Email("email", req.Email).
//	    StringLen("password", req.Password, 8, 128).
//	    StringLen("display_name", req.DisplayName, 1, 100).
//	    Err(); err != nil {
//	    api.Error(w, http.StatusBadRequest, err.Error())
//	    return
//	}
package validate

import (
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"unicode/utf8"
)

// FieldError identifies one failed check.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error is the multi-field validation failure. Implements `error` with
// a human-readable summary. Callers wanting field-level access should
// type-assert to `*Error` and read .Fields directly.
type Error struct {
	Fields []FieldError
}

func (e *Error) Error() string {
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = fmt.Sprintf("%s: %s", f.Field, f.Message)
	}
	return strings.Join(parts, "; ")
}

// Validator accumulates check failures.
type Validator struct {
	errors []FieldError
}

// New returns an empty Validator.
func New() *Validator {
	return &Validator{}
}

// StringLen checks that the rune-count of value is within [min, max].
// Pass min=0 to skip the lower bound; pass max=0 to skip the upper.
func (v *Validator) StringLen(field, value string, min, max int) *Validator {
	n := utf8.RuneCountInString(value)
	if min > 0 && n < min {
		v.add(field, fmt.Sprintf("must be at least %d characters", min))
	}
	if max > 0 && n > max {
		v.add(field, fmt.Sprintf("must be at most %d characters", max))
	}
	return v
}

// NotEmpty checks that strings.TrimSpace(value) is non-empty. Use this
// instead of StringLen(min=1) when whitespace-only input is invalid.
func (v *Validator) NotEmpty(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.add(field, "is required")
	}
	return v
}

// Email parses with net/mail. This is permissive (RFC 5322) — it
// accepts "Display <name@host>" forms — so for "bare address only"
// flows, combine with a check for `strings.Contains(value, "@")` or
// extract the parsed address.
func (v *Validator) Email(field, value string) *Validator {
	if _, err := mail.ParseAddress(value); err != nil {
		v.add(field, "must be a valid email address")
	}
	return v
}

// URL parses with net/url and optionally restricts to allowed schemes
// (e.g. "http", "https"). Empty allowedSchemes accepts any scheme as
// long as Host is non-empty.
func (v *Validator) URL(field, value string, allowedSchemes ...string) *Validator {
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		v.add(field, "must be a valid URL")
		return v
	}
	if len(allowedSchemes) == 0 {
		return v
	}
	for _, s := range allowedSchemes {
		if u.Scheme == s {
			return v
		}
	}
	v.add(field, fmt.Sprintf("scheme must be one of: %s", strings.Join(allowedSchemes, ", ")))
	return v
}

// SliceLen checks that length is within [min, max]. Pass length as
// `len(slice)` from the caller — keeps this package generic across
// slice element types without needing generics.
func (v *Validator) SliceLen(field string, length, min, max int) *Validator {
	if min > 0 && length < min {
		v.add(field, fmt.Sprintf("must have at least %d items", min))
	}
	if max > 0 && length > max {
		v.add(field, fmt.Sprintf("must have at most %d items", max))
	}
	return v
}

// IntRange checks that value is within [min, max] inclusive.
func (v *Validator) IntRange(field string, value, min, max int) *Validator {
	if value < min || value > max {
		v.add(field, fmt.Sprintf("must be between %d and %d", min, max))
	}
	return v
}

// OneOf checks that value matches one of the allowed strings.
func (v *Validator) OneOf(field, value string, allowed ...string) *Validator {
	for _, a := range allowed {
		if value == a {
			return v
		}
	}
	v.add(field, fmt.Sprintf("must be one of: %s", strings.Join(allowed, ", ")))
	return v
}

// Err returns nil if no failures, otherwise *Error.
func (v *Validator) Err() error {
	if len(v.errors) == 0 {
		return nil
	}
	return &Error{Fields: v.errors}
}

func (v *Validator) add(field, message string) {
	v.errors = append(v.errors, FieldError{Field: field, Message: message})
}
