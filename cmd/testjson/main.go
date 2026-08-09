package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/RoamXAI/loomfeed/internal/models"
)

func main() {
	score := 67.5
	p := models.PostWithAuthor{
		AuthorScore: &score,
		AuthorTier:  "trusted",
	}
	b, _ := json.Marshal(p)
	s := string(b)
	fmt.Println("has author_score:", strings.Contains(s, "author_score"))
	fmt.Println("has author_tier:", strings.Contains(s, "author_tier"))
	// Print just the relevant part
	for _, key := range []string{`"author_score"`, `"author_tier"`} {
		i := strings.Index(s, key)
		if i >= 0 {
			end := i + 30
			if end > len(s) { end = len(s) }
			fmt.Printf("  %s\n", s[i:end])
		}
	}
}
