package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/daviddwlee84/sshbbs/internal/markdown"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

// runImport handles `sshbbs import --file FILE --board NAME [--author USER]`.
// It opens the SQLite store, parses the markdown file, looks up the target
// board and author, and inserts a new article. Pushes in the markdown file
// are deliberately ignored — round-tripping pushes requires verifying each
// push's author exists in the users table, which is left for a future
// iteration to avoid spoofing risk.
//
// Returns the process exit code (0 on success).
func runImport(args []string) int {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: sshbbs import --file PATH --board NAME [--author USER] [--db PATH]")
		fs.PrintDefaults()
	}
	file := fs.String("file", "", "path to the markdown file (use '-' for stdin)")
	board := fs.String("board", "", "target board name (overrides frontmatter `board:`)")
	author := fs.String("author", "admin", "author user_id to attribute the import to")
	dbPath := fs.String("db", "data/bbs.db", "SQLite database path")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *file == "" {
		fmt.Fprintln(os.Stderr, "error: --file is required")
		fs.Usage()
		return 2
	}

	body, err := readFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *file, err)
		return 1
	}
	parsed, err := markdown.Parse(string(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse markdown: %v\n", err)
		return 1
	}
	if parsed.Title == "" {
		fmt.Fprintln(os.Stderr, "error: markdown has no `title:` frontmatter")
		return 1
	}
	targetBoard := *board
	if targetBoard == "" {
		targetBoard = parsed.BoardName
	}
	if targetBoard == "" {
		fmt.Fprintln(os.Stderr, "error: target board not given (use --board or `board:` frontmatter)")
		return 1
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store %s: %v\n", *dbPath, err)
		return 1
	}
	defer st.Close()

	ctx := context.Background()
	user, err := st.Users().GetByUserID(ctx, *author)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lookup author %q: %v\n", *author, err)
		return 1
	}
	b, err := st.Boards().GetByName(ctx, targetBoard)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lookup board %q: %v\n", targetBoard, err)
		return 1
	}
	art, err := st.Articles().Create(ctx, b.ID, user.ID, user.UserID, parsed.Title, parsed.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create article: %v\n", err)
		return 1
	}
	fmt.Printf("imported article %d into board %q (author=%s, %d pushes ignored)\n",
		art.ID, b.Name, user.UserID, len(parsed.Pushes))
	return 0
}

// readFile reads the named file, or all of stdin when path is "-".
func readFile(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "./") && !strings.HasPrefix(path, "../") {
		// Defensive: clarify relative paths are interpreted from cwd.
		path = "./" + path
	}
	return os.ReadFile(path)
}
