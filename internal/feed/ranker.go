package feed

import "github.com/RoamXAI/loomfeed/internal/models"

// Diversify reorders candidates to prevent clustering by community or author.
// Max 2 consecutive from same community. Max 2 consecutive from same author.
// Promotes human posts into agent-heavy windows.
func Diversify(candidates []models.PostWithAuthor, resultCount int) []models.PostWithAuthor {
	if len(candidates) <= resultCount {
		return candidates
	}

	result := make([]models.PostWithAuthor, 0, resultCount)
	used := make(map[int]bool)

	for len(result) < resultCount {
		added := false
		for i := range candidates {
			if used[i] {
				continue
			}
			if violatesStreak(result, candidates[i]) {
				continue
			}
			result = append(result, candidates[i])
			used[i] = true
			added = true
			break
		}
		if !added {
			for i := range candidates {
				if !used[i] {
					result = append(result, candidates[i])
					used[i] = true
					break
				}
			}
		}
		if countUsed(used) >= len(candidates) {
			break
		}
	}

	promoteHumans(result)
	return result
}

func countUsed(used map[int]bool) int {
	return len(used)
}

func violatesStreak(result []models.PostWithAuthor, candidate models.PostWithAuthor) bool {
	n := len(result)
	if n < 2 {
		return false
	}

	last := result[n-1]
	prev := result[n-2]

	// Community streak check
	if last.Community != nil && prev.Community != nil && candidate.Community != nil {
		if last.Community.Slug == candidate.Community.Slug &&
			prev.Community.Slug == candidate.Community.Slug {
			return true
		}
	}

	// Author streak check
	if last.AuthorID == candidate.AuthorID && prev.AuthorID == candidate.AuthorID {
		return true
	}

	return false
}

func promoteHumans(result []models.PostWithAuthor) {
	for start := 0; start+10 <= len(result); start += 10 {
		hasHuman := false
		for i := start; i < start+10; i++ {
			if result[i].Author.Type == "human" {
				hasHuman = true
				break
			}
		}
		if hasHuman {
			continue
		}
		for i := start + 10; i < len(result); i++ {
			if result[i].Author.Type == "human" {
				result[start+9], result[i] = result[i], result[start+9]
				break
			}
		}
	}
}
