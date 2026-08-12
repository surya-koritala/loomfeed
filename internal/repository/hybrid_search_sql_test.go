package repository

import (
	"strings"
	"testing"
)

// When pg_trgm's similarity() is not callable (extension row present but the
// function is off the connection's search_path — observed in production,
// SQLSTATE 42883), detectTrgm leaves hasTrgm false and the hybrid query MUST
// fall back to LIKE matching. If it emitted similarity() anyway, every hybrid
// search would 500. These are pure SQL-builder tests so they run without a DB.

func TestHybridSearchSQL_FallbackAvoidsSimilarity(t *testing.T) {
	r := &HybridSearchRepo{hasTrgm: false}
	sql, _ := r.hybridSearchSQL(SearchFilters{})
	if strings.Contains(sql, "similarity(") {
		t.Fatal("fallback SQL must not call similarity() — pg_trgm may be unavailable")
	}
	if !strings.Contains(sql, "LIKE") {
		t.Fatal("fallback SQL should use LIKE-based title matching")
	}
}

func TestHybridSearchSQL_TrgmPathUsesSimilarity(t *testing.T) {
	r := &HybridSearchRepo{hasTrgm: true}
	sql, _ := r.hybridSearchSQL(SearchFilters{})
	if !strings.Contains(sql, "similarity(") {
		t.Fatal("trgm path should use similarity() for fuzzy title matching")
	}
}

func TestSemanticHybridSearchSQL_AddsThirdRRFSignal(t *testing.T) {
	r := &HybridSearchRepo{hasTrgm: false}
	sql, _ := r.semanticHybridSearchSQL(SearchFilters{})

	for _, want := range []string{
		"semantic_candidates AS",
		"semantic_search AS",
		"embedding::halfvec(3072) <=> $4::halfvec(3072)",
		"LIMIT $5",
		"ss.semantic_pos",
		"60.0 + ss.semantic_pos",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("semantic hybrid SQL missing %q", want)
		}
	}
}

func TestSemanticHybridSearchSQL_AppliesFiltersBeforeANNLimit(t *testing.T) {
	r := &HybridSearchRepo{hasTrgm: false}
	sql, args := r.semanticHybridSearchSQL(SearchFilters{
		Community:  "science",
		AuthorType: "agent",
	})

	if len(args) != 2 || args[0] != "science" || args[1] != "agent" {
		t.Fatalf("unexpected filter args: %#v", args)
	}
	if strings.Count(sql, "c.slug = $6") != 1 {
		t.Error("expected the community slug filter in final results")
	}
	if strings.Count(sql, "p.community_id = (SELECT id FROM communities WHERE slug = $6)") != 1 {
		t.Error("expected an index-friendly community filter in semantic candidates")
	}
	if strings.Count(sql, "p.author_type = $7") != 2 {
		t.Error("expected author type filter in semantic candidates and final results")
	}
	if strings.Contains(sql, "_SEMANTIC_FILTER_CLAUSE_") ||
		strings.Contains(sql, "_FILTER_CLAUSE_") {
		t.Fatal("semantic hybrid SQL contains an unresolved filter placeholder")
	}
}
