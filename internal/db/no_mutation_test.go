package db_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// forbiddenMutationPattern matches UPDATE/DELETE statements targeting
// evidence_records anywhere in the repo's queries or generated code. This is
// the app-layer half of the write-once guarantee (issue #3 Decision 3): no
// code path may issue a mutating statement against evidence_records, so the
// database trigger is the only thing standing between a bug and tampering.
var forbiddenMutationPattern = regexp.MustCompile(`(?i)(UPDATE\s+evidence_records|DELETE\s+FROM\s+evidence_records)`)

// scannedRoots are the directories that could plausibly contain a query
// targeting evidence_records: the hand-written sqlc query files and the
// generated internal/db package.
var scannedRoots = []string{
	filepath.Join("..", "..", "db", "queries"),
	".",
}

func TestNoMutationQueriesAgainstEvidenceRecords(t *testing.T) {
	for _, root := range scannedRoots {
		root := root
		t.Run(root, func(t *testing.T) {
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatalf("read dir %s: %v", root, err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if !strings.HasSuffix(name, ".sql") && !strings.HasSuffix(name, ".sql.go") {
					continue
				}
				path := filepath.Join(root, name)
				contents, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read %s: %v", path, err)
				}
				if forbiddenMutationPattern.Match(contents) {
					t.Fatalf("%s contains a forbidden UPDATE/DELETE against evidence_records: write-once guarantee violated", path)
				}
			}
		})
	}
}
