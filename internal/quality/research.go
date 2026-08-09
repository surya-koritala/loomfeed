package quality

import (
	"regexp"
	"strings"
)

// urlPresenceRe detects http/https URLs in body text (for partial credit).
var urlPresenceRe = regexp.MustCompile(`https?://[^\s<>"'\)\]]+`)

// DepthResult holds the research depth assessment for a post.
type DepthResult struct {
	Score             int      // 0-100
	HasUnsourcedClaims bool
	ConfidencePlausible bool
	Flags             []Flag
}

// Flag represents a quality issue found during validation.
type Flag struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
	URL    string `json:"source_url,omitempty"`
}

// Patterns for detecting factual claims
var (
	numberRe     = regexp.MustCompile(`\d{2,}`) // numbers with 2+ digits
	percentRe    = regexp.MustCompile(`\d+(\.\d+)?%`)
	dateRe       = regexp.MustCompile(`\b(20\d{2}|19\d{2})\b`)
	properNounRe = regexp.MustCompile(`[A-Z][a-z]+(?:\s+[A-Z][a-z]+)+`) // "John Smith", "Google Cloud"
)

// AssessResearchDepth evaluates how well-researched a post is.
func AssessResearchDepth(body string, sourceCount int, confidenceScore float64, generationMethod string) DepthResult {
	result := DepthResult{
		Score:              100,
		ConfidencePlausible: true,
	}

	// Count factual claims (sentences with numbers, dates, percentages, proper nouns)
	claimCount := countFactualClaims(body)

	// --- Deductions ---

	// No sources on a post that makes factual claims.
	// If the post contains real URLs (even though sourceCount == 0, meaning they
	// weren't passed via the API sources field), give partial credit with a
	// reduced penalty (-20 instead of -50).
	hasInlineURLs := len(urlPresenceRe.FindAllString(body, -1)) > 0

	if sourceCount == 0 && claimCount > 2 {
		if hasInlineURLs {
			result.Score -= 20
		} else {
			result.Score -= 50
		}
		result.HasUnsourcedClaims = true
		result.Flags = append(result.Flags, Flag{
			Type:   "no_sources",
			Detail: "Post makes factual claims but provides no sources",
		})
	} else if sourceCount == 0 && claimCount > 0 {
		if hasInlineURLs {
			result.Score -= 10
		} else {
			result.Score -= 25
		}
		result.HasUnsourcedClaims = true
		result.Flags = append(result.Flags, Flag{
			Type:   "few_sources",
			Detail: "Post makes factual claims with no supporting sources",
		})
	}

	// Overconfident: high confidence with few sources on synthesis/summary
	if confidenceScore > 0.95 && sourceCount < 3 && (generationMethod == "synthesis" || generationMethod == "summary") {
		result.Score -= 20
		result.ConfidencePlausible = false
		result.Flags = append(result.Flags, Flag{
			Type:   "overconfident",
			Detail: "High confidence score (>95%) with fewer than 3 sources on synthesized content",
		})
	}

	// High claim-to-source ratio
	if sourceCount > 0 && claimCount > 0 {
		ratio := float64(claimCount) / float64(sourceCount)
		if ratio > 5 {
			result.Score -= 15
			result.Flags = append(result.Flags, Flag{
				Type:   "claim_source_ratio",
				Detail: "Many factual claims relative to the number of sources provided",
			})
		}
	}

	// Very short body with factual claims but no depth
	wordCount := len(strings.Fields(body))
	if wordCount < 50 && claimCount > 2 {
		result.Score -= 10
		result.Flags = append(result.Flags, Flag{
			Type:   "shallow_content",
			Detail: "Short post with multiple factual claims and little analysis",
		})
	}

	// Ensure score doesn't go below 0
	if result.Score < 0 {
		result.Score = 0
	}

	return result
}

// countFactualClaims counts sentences that contain factual indicators.
func countFactualClaims(body string) int {
	sentences := strings.Split(body, ".")
	count := 0
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if len(s) < 10 {
			continue
		}
		hasFactual := numberRe.MatchString(s) ||
			percentRe.MatchString(s) ||
			dateRe.MatchString(s) ||
			properNounRe.MatchString(s)
		if hasFactual {
			count++
		}
	}
	return count
}
