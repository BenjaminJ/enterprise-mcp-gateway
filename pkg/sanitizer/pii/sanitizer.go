package pii

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/goschan/enterprise-mcp-gateway/pkg/mcp/protocol"
)

// Common Precompiled Regex Patterns
var (
	// Luhn-checkable card regex: 13 to 19 digits with optional hyphens/spaces
	cardCandidateRegex = regexp.MustCompile(`\b(?:\d[ -]*?){13,19}\b`)

	// SSN regex: standard 3-2-4 format
	ssnRegex = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)

	// Email regex
	emailRegex = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)

	// Phone regex
	phoneRegex = regexp.MustCompile(`(?:\+?1[-. ]?)?\(?[2-9][0-9]{2}\)?[-. ]?[2-9][0-9]{2}[-. ]?[0-9]{4}\b`)

	// JWT regex: header.payload.signature
	jwtRegex = regexp.MustCompile(`\beyJ[A-Za-z0-9-_]+\.eyJ[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\b`)

	// Private Key regex
	privateKeyRegex = regexp.MustCompile(`-----BEGIN[ A-Z0-9_-]*PRIVATE KEY-----[\s\S]*?-----END[ A-Z0-9_-]*PRIVATE KEY-----`)

	// AWS Access Key ID
	awsKeyRegex = regexp.MustCompile(`\b(AKIA|ABIA|ACCA|ASIA)[0-9A-Z]{16}\b`)

	// GitHub Token
	githubTokenRegex = regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{36,255}\b`)
)

// CustomPattern defines a user-configured regex replacement rule.
type CustomPattern struct {
	Name        string
	Regex       *regexp.Regexp
	Replacement string
}

// Config controls which sanitization filters are active.
type Config struct {
	Enabled          bool
	MaskCardNumbers  bool
	MaskSSN          bool
	MaskEmails       bool
	MaskPhoneNumbers bool
	MaskSecrets      bool
	SensitiveKeys    []string
	CustomPatterns   []CustomPattern
}

// DefaultConfig returns standard recommended sanitization options.
func DefaultConfig() Config {
	return Config{
		Enabled:          true,
		MaskCardNumbers:  true,
		MaskSSN:          true,
		MaskEmails:       false,
		MaskPhoneNumbers: false,
		MaskSecrets:      true,
		SensitiveKeys: []string{
			"password", "passwd", "secret", "token", "apikey", "api_key",
			"access_token", "refresh_token", "private_key", "ssn", "creditcard", "card_number",
		},
	}
}

// Sanitizer handles PII and secret redaction.
type Sanitizer struct {
	cfg          Config
	keyLookup    map[string]struct{}
	customRegexs []CustomPattern
}

// NewSanitizer creates a new Sanitizer instance.
func NewSanitizer(cfg Config) *Sanitizer {
	lookup := make(map[string]struct{})
	for _, k := range cfg.SensitiveKeys {
		lookup[strings.ToLower(strings.ReplaceAll(k, "_", ""))] = struct{}{}
		lookup[strings.ToLower(k)] = struct{}{}
	}

	return &Sanitizer{
		cfg:          cfg,
		keyLookup:    lookup,
		customRegexs: cfg.CustomPatterns,
	}
}

// IsSensitiveKey checks whether a JSON key name is classified as sensitive.
func (s *Sanitizer) IsSensitiveKey(key string) bool {
	norm := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	if _, ok := s.keyLookup[norm]; ok {
		return true
	}
	_, ok := s.keyLookup[strings.ToLower(key)]
	return ok
}

// SanitizeText scrubs sensitive patterns from a raw string.
func (s *Sanitizer) SanitizeText(text string) (string, int) {
	if !s.cfg.Enabled || text == "" {
		return text, 0
	}

	totalRedacted := 0

	// 1. Secrets (Private keys, JWTs, AWS keys, GitHub tokens)
	if s.cfg.MaskSecrets {
		if privateKeyRegex.MatchString(text) {
			text = privateKeyRegex.ReplaceAllString(text, "[REDACTED-PRIVATE-KEY]")
			totalRedacted++
		}
		if jwtRegex.MatchString(text) {
			text = jwtRegex.ReplaceAllString(text, "[REDACTED-JWT]")
			totalRedacted++
		}
		if awsKeyRegex.MatchString(text) {
			text = awsKeyRegex.ReplaceAllString(text, "[REDACTED-AWS-KEY]")
			totalRedacted++
		}
		if githubTokenRegex.MatchString(text) {
			text = githubTokenRegex.ReplaceAllString(text, "[REDACTED-GITHUB-TOKEN]")
			totalRedacted++
		}
	}

	// 2. SSN
	if s.cfg.MaskSSN {
		matches := ssnRegex.FindAllString(text, -1)
		if len(matches) > 0 {
			text = ssnRegex.ReplaceAllString(text, "[REDACTED-SSN]")
			totalRedacted += len(matches)
		}
	}

	// 3. Credit Cards with Luhn Check
	if s.cfg.MaskCardNumbers {
		text, totalRedacted = s.maskCreditCards(text, totalRedacted)
	}

	// 4. Emails
	if s.cfg.MaskEmails {
		matches := emailRegex.FindAllString(text, -1)
		if len(matches) > 0 {
			text = emailRegex.ReplaceAllString(text, "[REDACTED-EMAIL]")
			totalRedacted += len(matches)
		}
	}

	// 5. Phone Numbers
	if s.cfg.MaskPhoneNumbers {
		matches := phoneRegex.FindAllString(text, -1)
		if len(matches) > 0 {
			text = phoneRegex.ReplaceAllString(text, "[REDACTED-PHONE]")
			totalRedacted += len(matches)
		}
	}

	// 6. Custom Patterns
	for _, cp := range s.customRegexs {
		if cp.Regex != nil {
			matches := cp.Regex.FindAllString(text, -1)
			if len(matches) > 0 {
				rep := cp.Replacement
				if rep == "" {
					rep = "[REDACTED]"
				}
				text = cp.Regex.ReplaceAllString(text, rep)
				totalRedacted += len(matches)
			}
		}
	}

	return text, totalRedacted
}

func (s *Sanitizer) maskCreditCards(text string, count int) (string, int) {
	return cardCandidateRegex.ReplaceAllStringFunc(text, func(match string) string {
		// Clean non-digit characters
		var digits []rune
		for _, r := range match {
			if unicode.IsDigit(r) {
				digits = append(digits, r)
			}
		}

		if len(digits) < 13 || len(digits) > 19 {
			return match
		}

		if isValidLuhn(string(digits)) {
			count++
			return "[REDACTED-CARD]"
		}
		return match
	}), count
}

// isValidLuhn implements the standard Luhn checksum algorithm (Mod 10).
func isValidLuhn(num string) bool {
	sum := 0
	alt := false

	for i := len(num) - 1; i >= 0; i-- {
		n, err := strconv.Atoi(string(num[i]))
		if err != nil {
			return false
		}

		if alt {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}

		sum += n
		alt = !alt
	}

	return sum%10 == 0
}

// SanitizeJSON recursively parses and sanitizes a JSON object or string.
func (s *Sanitizer) SanitizeJSON(input []byte) ([]byte, int) {
	if !s.cfg.Enabled || len(input) == 0 {
		return input, 0
	}

	var parsed any
	if err := json.Unmarshal(input, &parsed); err != nil {
		// Not JSON, sanitize as raw text
		cleaned, c := s.SanitizeText(string(input))
		return []byte(cleaned), c
	}

	redactedCount := 0
	parsed = s.sanitizeJSONValue(parsed, &redactedCount)

	out, err := json.Marshal(parsed)
	if err != nil {
		return input, redactedCount
	}
	return out, redactedCount
}

func (s *Sanitizer) sanitizeJSONValue(v any, count *int) any {
	switch val := v.(type) {
	case map[string]any:
		res := make(map[string]any, len(val))
		for k, item := range val {
			if s.IsSensitiveKey(k) {
				res[k] = "[REDACTED]"
				*count++
			} else {
				res[k] = s.sanitizeJSONValue(item, count)
			}
		}
		return res
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = s.sanitizeJSONValue(item, count)
		}
		return res
	case string:
		cleaned, c := s.SanitizeText(val)
		*count += c
		return cleaned
	default:
		return v
	}
}

// SanitizeResult sanitizes all text content items inside a CallToolResult in-place.
func (s *Sanitizer) SanitizeResult(result *protocol.CallToolResult) int {
	if !s.cfg.Enabled || result == nil {
		return 0
	}

	totalRedacted := 0
	for i := range result.Content {
		if result.Content[i].Type == protocol.ContentTypeText {
			rawText := result.Content[i].Text
			// Attempt JSON sanitization first if it looks like JSON
			trimmed := strings.TrimSpace(rawText)
			if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
				(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
				sanitizedJSON, count := s.SanitizeJSON([]byte(rawText))
				result.Content[i].Text = string(sanitizedJSON)
				totalRedacted += count
			} else {
				sanitizedText, count := s.SanitizeText(rawText)
				result.Content[i].Text = sanitizedText
				totalRedacted += count
			}
		}
	}
	return totalRedacted
}
