package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// polishChars: characters that must not appear in string literals or comments
// in cmd/ and internal/ (the code base is English-only). Written as escapes so
// this file passes its own check.
const polishChars = "\u0105\u0107\u0119\u0142\u0144\u00f3\u015b\u017a\u017c" +
	"\u0104\u0106\u0118\u0141\u0143\u00d3\u015a\u0179\u017b\u201e\u201d"

// cFiles: the C sources of the macFUSE bridge, checked as plain text.
var cFiles = []string{
	"internal/fusemount/ops_darwin.c",
	"internal/fusemount/bridge.h",
}

// TestSourcesAreEnglish walks every .go file (tests included) under cmd/ and
// internal/ and rejects Polish characters in string literals and comments.
// It also scans the C bridge files as plain text.
func TestSourcesAreEnglish(t *testing.T) {
	root := filepath.Join("..", "..")
	checked := 0
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") {
				return nil
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			checked++
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if ok && lit.Kind == token.STRING && strings.ContainsAny(lit.Value, polishChars) {
					t.Errorf("%s: Polish text in string literal: %s", fset.Position(lit.Pos()), lit.Value)
				}
				return true
			})
			for _, cg := range f.Comments {
				for _, c := range cg.List {
					if strings.ContainsAny(c.Text, polishChars) {
						t.Errorf("%s: Polish text in comment: %s", fset.Position(c.Pos()), c.Text)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked == 0 {
		t.Fatal("no .go files found to check")
	}
	for _, rel := range cFiles {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.ContainsAny(line, polishChars) {
				t.Errorf("%s:%d: Polish text: %s", rel, i+1, line)
			}
		}
	}
}
