package quality

import (
	"strings"
	"testing"
)

func TestGenerateTLDR_ShortPost(t *testing.T) {
	// Posts with fewer than 300 words should return empty string
	body := "This is a short post with fewer than 300 words."
	result := GenerateTLDR(body, nil)
	if result != "" {
		t.Errorf("expected empty TLDR for short post, got %q", result)
	}
}

func TestGenerateTLDR_LongPost(t *testing.T) {
	// Generate a body with >300 words
	sentences := []string{
		"The research team at OpenAI recently published groundbreaking results in artificial intelligence.",
		"Their findings demonstrate significant improvements in reasoning capabilities.",
		"The study involved over 1000 participants across 15 countries.",
		"Previous approaches had limited success in this domain.",
		"Many researchers have attempted similar methods without success.",
		"The key innovation was a novel training procedure.",
		"This procedure reduces computational costs by 40%.",
		"Furthermore, the accuracy improved by 25% over the baseline.",
		"The team plans to release the model weights in 2024.",
		"Community feedback has been overwhelmingly positive.",
	}

	// Pad to ensure >300 words
	var builder strings.Builder
	for i := 0; i < 5; i++ {
		for _, s := range sentences {
			builder.WriteString(s)
			builder.WriteString(" ")
		}
	}
	body := builder.String()

	wordCount := len(strings.Fields(body))
	if wordCount < 300 {
		t.Fatalf("test body has %d words, need >300", wordCount)
	}

	result := GenerateTLDR(body, nil)
	if result == "" {
		t.Error("expected non-empty TLDR for long post")
	}

	// Should be at most 2 sentences
	sentenceCount := len(splitSentences(result))
	if sentenceCount > 2 {
		t.Errorf("expected at most 2 sentences in TLDR, got %d", sentenceCount)
	}
}

func TestGenerateTLDR_EmptyBody(t *testing.T) {
	result := GenerateTLDR("", nil)
	if result != "" {
		t.Errorf("expected empty TLDR for empty body, got %q", result)
	}
}

func TestSplitSentences(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"Hello world. This is a test. Third sentence.", 3},
		{"One sentence only", 1},
		{"First. Second.\nThird.", 3},
		{"", 0},
	}

	for _, tc := range tests {
		sentences := splitSentences(tc.input)
		if len(sentences) != tc.expected {
			t.Errorf("splitSentences(%q) = %d sentences, want %d (got: %v)", tc.input, len(sentences), tc.expected, sentences)
		}
	}
}

func TestScoreSentence(t *testing.T) {
	// First sentence (index 0) should get +2
	score0 := scoreSentence("This is a simple test sentence for scoring.", 0)
	score5 := scoreSentence("This is a simple test sentence for scoring.", 5)
	if score0 <= score5 {
		t.Errorf("expected first sentence to score higher (got %d vs %d)", score0, score5)
	}

	// Sentence with proper nouns and numbers should score higher
	scorePlain := scoreSentence("This is a simple test of things.", 5)
	scoreRich := scoreSentence("OpenAI announced 100 new features in March.", 5)
	if scoreRich <= scorePlain {
		t.Errorf("expected rich sentence to score higher (got %d vs %d)", scoreRich, scorePlain)
	}

	// Very short sentence should get penalty
	scoreShort := scoreSentence("Too short.", 5)
	if scoreShort >= 0 {
		t.Errorf("expected short sentence penalty, got %d", scoreShort)
	}
}
