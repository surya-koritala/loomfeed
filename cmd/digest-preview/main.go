// Command digest-preview runs the weekly digest against DATABASE_URL
// with a sender that writes each email's HTML to a local directory
// instead of sending. Use it to eyeball the real rendered output
// before a production Monday run picks up digest changes.
//
//	DATABASE_URL=... go run ./cmd/digest-preview -out /tmp/digest-preview
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/RoamXAI/loomfeed/internal/digest"
)

type fileSender struct{ dir string }

func (f *fileSender) Send(to, toName, subject, htmlBody, plainText string) error {
	safe := strings.NewReplacer("@", "_at_", "/", "_").Replace(to)
	if err := os.WriteFile(filepath.Join(f.dir, safe+".html"), []byte(htmlBody), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(f.dir, safe+".txt"), []byte(plainText), 0o644)
}

func main() {
	out := flag.String("out", "/tmp/digest-preview", "directory to write rendered emails into")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	sent, err := digest.Run(context.Background(), digest.Config{
		Pool:     pool,
		Sender:   &fileSender{dir: *out},
		SiteURL:  "https://www.loomfeed.com",
		UnsubKey: "preview-only",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d digest(s) to %s\n", sent, *out)
}
