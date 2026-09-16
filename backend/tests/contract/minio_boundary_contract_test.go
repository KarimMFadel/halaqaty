//go:build contract

package contract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// TestMinIOSDKImportsAreConfinedToTheAdapter supplements real storage tests
// with ADR-023's bounded production dependency policy (internal and cmd/api).
func TestMinIOSDKImportsAreConfinedToTheAdapter(t *testing.T) {
	assertProviderImportsConfined(t, repositoryRoot(t), "github.com/minio/", filepath.Join("backend", "internal", "chat", "minio"))
}

// TestChatObjectStoreBoundaryExists is supplemental structural coverage for
// the approved neutral interface; policy and adapter suites exercise behavior.
func TestChatObjectStoreBoundaryExists(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(repositoryRoot(t), "backend", "internal", "chat", "media_store.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			typ, ok := spec.(*ast.TypeSpec)
			if ok && typ.Name.Name == "ObjectStore" {
				if _, ok := typ.Type.(*ast.InterfaceType); !ok {
					t.Fatal("ObjectStore must be an interface")
				}
				return
			}
		}
	}
	t.Fatal("chat-owned neutral ObjectStore interface is missing")
}
