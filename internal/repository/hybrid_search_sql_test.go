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
