// Package modfilter provides content moderation filtering for posts, comments,
// and community names. It checks text against block and flag word lists with
// word-boundary matching, leet-speak normalization, and context-aware exceptions
// to minimize false positives.
package modfilter

import (
	"log/slog"
	"regexp"
	"strings"
	"unicode"
)

// Severity indicates how seriously a content match should be treated.
type Severity int

const (
	SeverityNone  Severity = 0 // clean content
	SeverityFlag  Severity = 1 // publish but flag for mod review
	SeverityBlock Severity = 2 // reject immediately
)

// CheckResult holds the outcome of a content moderation check.
type CheckResult struct {
	Severity    Severity
	Reason      string // human-readable reason
	MatchedWord string // the word/phrase that triggered (internal use only — never expose to users)
	Category    string // "hate", "violence", "sexual", "spam", "illegal", "profanity"
}

// compiledPattern holds a pre-compiled regex and its associated metadata.
type compiledPattern struct {
	re       *regexp.Regexp
	word     string
	severity Severity
	category string
}

// Pre-compiled patterns initialized at package load time for performance.
var (
	blockWordPatterns   []compiledPattern
	flagWordPatterns    []compiledPattern
	blockPhrasePatterns []compiledPattern
	spamPhrasePatterns  []compiledPattern
	exceptionSet        map[string]bool
)

// contextWindowSize is the number of characters around a match to scan for exceptions.
const contextWindowSize = 80

func init() {
	// Build exception lookup set.
	exceptionSet = make(map[string]bool, len(contextExceptions))
	for _, ex := range contextExceptions {
		exceptionSet[strings.ToLower(ex)] = true
	}

	// Compile block word patterns with word boundaries.
	blockWordPatterns = compileWordPatterns(blockWords, SeverityBlock, "hate")

	// Compile flag word patterns with word boundaries.
	flagWordPatterns = compileWordPatterns(flagWords, SeverityFlag, "profanity")

	// Compile block phrase patterns (no word boundaries needed — whole phrase).
	blockPhrasePatterns = compilePhrasePatterns(blockPhrases, SeverityBlock, "violence")

	// Compile spam phrase patterns.
	spamPhrasePatterns = compilePhrasePatterns(spamPhrases, SeverityFlag, "spam")
}

// compileWordPatterns builds compiled regex patterns with word boundaries for a list of words.
func compileWordPatterns(words []string, severity Severity, category string) []compiledPattern {
	patterns := make([]compiledPattern, 0, len(words))
	for _, w := range words {
		escaped := regexp.QuoteMeta(strings.ToLower(w))
		// Use word boundary matching. For multi-word entries (like "kill yourself"),
		// apply boundaries around the whole phrase.
		pattern := `(?i)\b` + escaped + `\b`
		re, err := regexp.Compile(pattern)
		if err != nil {
			slog.Error("failed to compile modfilter pattern", "word", w, "error", err)
			continue
		}
		patterns = append(patterns, compiledPattern{
			re:       re,
			word:     w,
			severity: severity,
			category: category,
		})
	}
	return patterns
}

// compilePhrasePatterns builds compiled regex patterns for multi-word phrases.
func compilePhrasePatterns(phrases []string, severity Severity, category string) []compiledPattern {
	patterns := make([]compiledPattern, 0, len(phrases))
	for _, p := range phrases {
		escaped := regexp.QuoteMeta(strings.ToLower(p))
		pattern := `(?i)` + escaped
		re, err := regexp.Compile(pattern)
		if err != nil {
			slog.Error("failed to compile modfilter phrase pattern", "phrase", p, "error", err)
			continue
		}
		patterns = append(patterns, compiledPattern{
			re:       re,
			word:     p,
			severity: severity,
			category: category,
		})
	}
	return patterns
}

// leetMap maps common leet-speak substitutions to their alphabetic equivalents.
var leetMap = map[rune]rune{
	'@': 'a',
	'0': 'o',
	'1': 'i',
	'3': 'e',
	'$': 's',
	'5': 's',
	'7': 't',
	'4': 'a',
	'!': 'i',
	'+': 't',
	'8': 'b',
}

// normalizeLeet converts common leet-speak character substitutions to their
// alphabetic equivalents, allowing the filter to catch evasion attempts like
// "h@te" or "sh1t". It also strips zero-width characters and collapses
// repeated characters beyond 2 (e.g., "fuuuck" -> "fuuck").
func normalizeLeet(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	var prevRune rune
	var runCount int

	for _, r := range s {
		// Skip zero-width characters used for evasion.
		if r == '\u200B' || r == '\u200C' || r == '\u200D' || r == '\uFEFF' {
			continue
		}

		// Apply leet-speak mapping.
		if mapped, ok := leetMap[r]; ok {
			r = mapped
		}

		// Collapse runs of the same character beyond 2.
		if r == prevRune {
			runCount++
			if runCount > 2 {
				continue
			}
		} else {
			runCount = 1
			prevRune = r
		}

		b.WriteRune(r)
	}
	return b.String()
}

// stripNonAlphanumeric removes characters that are not letters, digits, or spaces.
// Used to create a secondary representation for matching against patterns
// that use character insertions (e.g., "f.u.c.k").
func stripNonAlphanumeric(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// hasContextException checks if any exception words appear near the match location
// in the original text, suggesting the flagged word is used in a benign context.
func hasContextException(text string, matchStart, matchEnd int) bool {
	// Expand the window around the match.
	windowStart := matchStart - contextWindowSize
	if windowStart < 0 {
		windowStart = 0
	}
	windowEnd := matchEnd + contextWindowSize
	if windowEnd > len(text) {
		windowEnd = len(text)
	}
	window := strings.ToLower(text[windowStart:windowEnd])

	for ex := range exceptionSet {
		if strings.Contains(window, ex) {
			return true
		}
	}
	return false
}

// Check inspects text for prohibited content and returns a CheckResult.
// It performs case-insensitive matching with leet-speak normalization,
// word-boundary enforcement, and context-aware exception handling.
//
// The check order is: block phrases -> block words -> spam phrases -> flag words.
// The first match at the highest severity wins.
func Check(text string) CheckResult {
	if text == "" {
		return CheckResult{Severity: SeverityNone}
	}

	lower := strings.ToLower(text)
	normalized := normalizeLeet(lower)
	stripped := stripNonAlphanumeric(normalized)

	// We check multiple representations of the text to catch evasion.
	representations := []string{lower, normalized}
	if stripped != normalized {
		representations = append(representations, stripped)
	}

	// 1. Check block phrases first (highest severity, most specific).
	for _, p := range blockPhrasePatterns {
		for _, repr := range representations {
			if loc := p.re.FindStringIndex(repr); loc != nil {
				result := CheckResult{
					Severity:    SeverityBlock,
					Reason:      "content contains prohibited phrase",
					MatchedWord: p.word,
					Category:    p.category,
				}
				logFiltered(result, text)
				return result
			}
		}
	}

	// 2. Check block words (unambiguous slurs — no context exceptions).
	for _, p := range blockWordPatterns {
		for _, repr := range representations {
			if p.re.MatchString(repr) {
				result := CheckResult{
					Severity:    SeverityBlock,
					Reason:      "content contains prohibited language",
					MatchedWord: p.word,
					Category:    p.category,
				}
				logFiltered(result, text)
				return result
			}
		}
	}

	// 3. Check spam phrases (flag severity).
	for _, p := range spamPhrasePatterns {
		for _, repr := range representations {
			if loc := p.re.FindStringIndex(repr); loc != nil {
				result := CheckResult{
					Severity:    SeverityFlag,
					Reason:      "content appears to contain spam",
					MatchedWord: p.word,
					Category:    "spam",
				}
				logFiltered(result, text)
				return result
			}
		}
	}

	// 4. Check flag words (with context exceptions to reduce false positives).
	for _, p := range flagWordPatterns {
		for _, repr := range representations {
			if loc := p.re.FindStringIndex(repr); loc != nil {
				// Check for context exceptions before flagging.
				if hasContextException(lower, loc[0], loc[1]) {
					continue
				}
				result := CheckResult{
					Severity:    SeverityFlag,
					Reason:      "content flagged for moderator review",
					MatchedWord: p.word,
					Category:    p.category,
				}
				logFiltered(result, text)
				return result
			}
		}
	}

	return CheckResult{Severity: SeverityNone}
}

// logFiltered logs filtered content for audit trail purposes.
func logFiltered(result CheckResult, originalText string) {
	// Truncate the original text for logging to avoid filling logs with huge content.
	logText := originalText
	if len(logText) > 200 {
		logText = logText[:200] + "..."
	}

	slog.Warn("content moderation triggered",
		"severity", result.Severity,
		"category", result.Category,
		"reason", result.Reason,
		"text_excerpt", logText,
	)
}
