package modfilter

import (
	"testing"
)

func TestCheck_CleanContent(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"empty string", ""},
		{"normal text", "This is a perfectly normal post about programming."},
		{"technical discussion", "We need to optimize the algorithm for better performance."},
		{"agent research", "The AI model achieved 95% accuracy on the benchmark."},
		{"markdown content", "## Introduction\n\nHere is a paragraph with **bold** text."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(tt.text)
			if result.Severity != SeverityNone {
				t.Errorf("expected SeverityNone for %q, got severity=%d reason=%q matched=%q",
					tt.text, result.Severity, result.Reason, result.MatchedWord)
			}
		})
	}
}

func TestCheck_BlockedSlurs(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"racial slur", "you are a nigger"},
		{"homophobic slur", "what a faggot"},
		{"uppercase evasion", "YOU ARE A NIGGER"},
		{"mixed case", "FaGgOt"},
		{"ethnic slur", "stupid kike"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(tt.text)
			if result.Severity != SeverityBlock {
				t.Errorf("expected SeverityBlock for %q, got severity=%d reason=%q",
					tt.text, result.Severity, result.Reason)
			}
		})
	}
}

func TestCheck_BlockedPhrases(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"bomb making", "I want to know how to make a bomb"},
		{"hit hire", "I need to hire a hitman"},
		{"kill threat", "I will kill you for this"},
		{"go kys", "go kill yourself loser"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(tt.text)
			if result.Severity != SeverityBlock {
				t.Errorf("expected SeverityBlock for %q, got severity=%d reason=%q",
					tt.text, result.Severity, result.Reason)
			}
		})
	}
}

func TestCheck_FlaggedContent(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"profanity", "this is such bullshit"},
		{"profanity f-word", "what the fuck is this"},
		{"offensive term", "that's so retarded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(tt.text)
			if result.Severity != SeverityFlag {
				t.Errorf("expected SeverityFlag for %q, got severity=%d reason=%q matched=%q",
					tt.text, result.Severity, result.Reason, result.MatchedWord)
			}
		})
	}
}

func TestCheck_SpamPhrases(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"buy now", "BUY NOW and get 50% off!"},
		{"free money", "Get free money by signing up today"},
		{"crypto scam", "Double your bitcoin in 24 hours!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(tt.text)
			if result.Severity != SeverityFlag {
				t.Errorf("expected SeverityFlag for %q, got severity=%d reason=%q",
					tt.text, result.Severity, result.Reason)
			}
			if result.Category != "spam" {
				t.Errorf("expected category 'spam' for %q, got %q", tt.text, result.Category)
			}
		})
	}
}

func TestCheck_TechnicalTermsNotFlagged(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"kill process", "You can kill the process with SIGTERM using kill -15"},
		{"master slave", "The database uses a master/slave replication setup"},
		{"execute command", "Execute the command in your terminal to deploy"},
		{"bomb in security context", "The security team found a fork bomb in the testing environment"},
		{"murder mystery game", "I love playing the murder mystery game with friends"},
		{"terrorist in news", "The article discusses how the news report covered terrorism prevention"},
		{"suicide prevention", "Resources for suicide prevention and awareness hotline"},
		{"shell kill signal", "The shell sends a kill signal to terminate the daemon process"},
		{"assassins creed", "I've been playing Assassin's Creed all weekend"},
		{"penetration testing", "We need to run penetration testing on the security framework"},
		{"exploit in cybersecurity", "The exploit vulnerability was reported as CVE-2024-1234"},
		{"rape in research context", "The research paper studies prevention and awareness of sexual assault"},
		{"nazi in historical context", "The historical documentary covers the rise and fall of the nazi regime"},
		{"fuck in code context", "The debugging function found the bug in the code"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(tt.text)
			if result.Severity != SeverityNone {
				t.Errorf("expected SeverityNone for %q, got severity=%d reason=%q matched=%q",
					tt.text, result.Severity, result.Reason, result.MatchedWord)
			}
		})
	}
}

func TestCheck_WordBoundaryMatching(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		shouldMatch bool
	}{
		{"class not flagged", "The classification system works well", false},
		{"assassin in game name", "Assassin's Creed is a great game", false},
		{"therapist", "My therapist recommended this book", false},
		{"scunthorpe", "I live in Scunthorpe", false},
		{"shitake", "I love shitake mushrooms", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(tt.text)
			if tt.shouldMatch && result.Severity == SeverityNone {
				t.Errorf("expected match for %q, got SeverityNone", tt.text)
			}
			if !tt.shouldMatch && result.Severity != SeverityNone {
				t.Errorf("expected no match for %q, got severity=%d reason=%q matched=%q",
					tt.text, result.Severity, result.Reason, result.MatchedWord)
			}
		})
	}
}

func TestCheck_LeetSpeakNormalization(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"leet slur @", "n1gg3r"},
		{"leet with $", "f@gg0t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(tt.text)
			if result.Severity != SeverityBlock {
				t.Errorf("expected SeverityBlock for leet speak %q, got severity=%d",
					tt.text, result.Severity)
			}
		})
	}
}

func TestCheck_ZeroWidthEvasion(t *testing.T) {
	// Zero-width characters inserted to break word detection.
	text := "nig\u200Bger"
	result := Check(text)
	if result.Severity != SeverityBlock {
		t.Errorf("expected SeverityBlock for zero-width evasion, got severity=%d matched=%q",
			result.Severity, result.MatchedWord)
	}
}

func TestCheck_RepeatedCharEvasion(t *testing.T) {
	text := "niggggger"
	result := Check(text)
	if result.Severity != SeverityBlock {
		t.Errorf("expected SeverityBlock for repeated-char evasion, got severity=%d matched=%q",
			result.Severity, result.MatchedWord)
	}
}

func TestNormalizeLeet(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"h@te", "hate"},
		{"sh1t", "shit"},
		{"a$$", "ass"},
		{"l33t", "leet"},
		{"fuuuuck", "fuuck"},
		{"normal text", "normal text"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeLeet(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeLeet(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCheck_CategoryAssignment(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		category string
	}{
		{"hate speech", "you are a nigger", "hate"},
		{"spam", "Buy now and get 50% off!", "spam"},
		{"profanity", "this is bullshit", "profanity"},
		{"violence phrase", "how to make a bomb please", "violence"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Check(tt.text)
			if result.Category != tt.category {
				t.Errorf("expected category %q for %q, got %q", tt.category, tt.text, result.Category)
			}
		})
	}
}

func TestCheck_SeverityOrdering(t *testing.T) {
	// A text that contains both a block word and a flag word should return Block.
	text := "you nigger, this is bullshit"
	result := Check(text)
	if result.Severity != SeverityBlock {
		t.Errorf("expected SeverityBlock when both block and flag words present, got severity=%d",
			result.Severity)
	}
}

func BenchmarkCheck_CleanContent(b *testing.B) {
	text := "This is a perfectly normal post about artificial intelligence research and language models. The agent achieved 95% accuracy on the benchmark dataset using a novel approach to few-shot learning."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Check(text)
	}
}

func BenchmarkCheck_WithMatch(b *testing.B) {
	text := "This post contains nigger which should be blocked immediately"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Check(text)
	}
}
