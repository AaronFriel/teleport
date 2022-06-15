package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

type EventData struct {
	Name    string
	Comment string
}

func main() {
	eventTypes := make(map[string]struct{})
	gofiles := []*ast.File{}
	eventData := []EventData{}

	s := token.NewFileSet()
	if err := filepath.Walk(path.Join("..", ".."), func(pth string, i fs.FileInfo, _ error) error {
		if !strings.HasSuffix(i.Name(), ".go") {
			return nil
		}
		f, err := parser.ParseFile(s, pth, nil, parser.ParseComments)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing Go source files: %v", err)
			os.Exit(1)
		}
		gofiles = append(gofiles, f)
		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "error walking gravitational/teleport: %v", err)
		os.Exit(1)
	}

	// First walk through the AST: collect types of audit events.
	// We identify audit event types by instances where a field named
	// "Metadata" is assigned to a composite literal with type
	// "Metadata". Further, that Metadata composite literal has a
	// field called "Type".
	for _, f := range gofiles {

		for _, d := range f.Decls {
			astutil.Apply(d, func(c *astutil.Cursor) bool {
				if kv, ok := c.Node().(*ast.KeyValueExpr); ok {

					if ki, ok := kv.Key.(*ast.Ident); !ok || ki.Name != "Metadata" {
						// This can't be the Metadata field of an audit
						// event, so keep looking
						return true
					}

					if vl, ok := kv.Value.(*ast.CompositeLit); ok {
						if vt, ok := vl.Type.(*ast.SelectorExpr); ok && vt.Sel.Name == "Metadata" {
							for _, el := range vl.Elts {
								elkv, ok := el.(*ast.KeyValueExpr)
								if !ok {
									continue
								}
								elkvk, ok := elkv.Key.(*ast.Ident)
								if !ok {
									continue
								}

								// We have an audit event type, so save it
								// for our second walk through the AST.
								if elkvk.Name == "Type" {
									elkvv, ok := elkv.Value.(*ast.SelectorExpr)
									if !ok {
										continue
									}
									eventTypes[elkvv.Sel.Name] = struct{}{}
								}
							}
						}
					}
				}
				return true
			}, nil)
		}

	}

	for _, f := range gofiles {
		// Second walk through the AST: find definitions of audit event
		// types by comparing them to the audit event types we collected
		// in the first walk.
		for _, d := range f.Decls {
			astutil.Apply(d, func(c *astutil.Cursor) bool {
				// Look through all declarations and find those that match
				// the identifiers we have collected.
				val, ok := c.Node().(*ast.ValueSpec)
				if !ok {
					return true
				}
				for _, n := range val.Names {
					if _, y := eventTypes[n.Name]; y {
						tx := val.Doc.Text()
						typ := strings.ReplaceAll(val.Values[0].(*ast.BasicLit).Value, "\"", "`")
						if strings.HasPrefix(tx, n.Name) {
							tx = strings.ReplaceAll(tx, n.Name, typ)
						}
						eventData = append(eventData, EventData{
							Name:    typ,
							Comment: tx,
						})
					}
				}
				return true
			}, nil)
		}
	}
	fmt.Println(eventData)
}
