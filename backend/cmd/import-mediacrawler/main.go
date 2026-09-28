package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/example/cs-pulse/backend/internal/config"
	"github.com/example/cs-pulse/backend/internal/mediacrawler"
	"github.com/example/cs-pulse/backend/internal/repository"
	"github.com/joho/godotenv"
)

func main() {
	var xc, xm, tc, tm, keyword string
	flag.StringVar(&xc, "xhs-contents", "", "Xiaohongshu content JSONL")
	flag.StringVar(&xm, "xhs-comments", "", "Xiaohongshu comment JSONL")
	flag.StringVar(&tc, "tieba-contents", "", "Tieba content JSONL")
	flag.StringVar(&tm, "tieba-comments", "", "Tieba comment JSONL")
	flag.StringVar(&keyword, "keyword", "CS2", "Search keyword")
	flag.Parse()
	if xc == "" && tc == "" {
		fatal(fmt.Errorf("provide at least one --*-contents file"))
	}
	_ = godotenv.Load("../.env", ".env")
	db, err := repository.Open(config.Load().MySQLDSN)
	if err != nil {
		fatal(err)
	}
	repo := repository.New(db)
	if err := repo.MigrateAndSeed(); err != nil {
		fatal(err)
	}
	for _, input := range []struct{ source, contents, comments string }{{"xiaohongshu", xc, xm}, {"tieba", tc, tm}} {
		if input.contents == "" {
			continue
		}
		rows, comments, err := mediacrawler.ReadRows(input.contents, input.comments)
		if err != nil {
			fatal(err)
		}
		if err = mediacrawler.Import(repo, input.source, keyword, rows, comments); err != nil {
			fatal(err)
		}
		fmt.Printf("Imported %s: %d posts, %d comments\n", input.source, len(rows), len(comments))
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
