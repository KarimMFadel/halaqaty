//go:build contract

package contract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func TestCircleHardDeleteSafety_NoRepositoryMethodOrSQL(t *testing.T) {
	fileSet := token.NewFileSet()
	repository, err := parser.ParseFile(fileSet, "../../internal/rbac/repository.go", nil, 0)
	if err != nil {
		t.Fatalf("parse repository: %v", err)
	}
	for _, declaration := range repository.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && strings.Contains(strings.ToLower(function.Name.Name), "deletecircle") {
			t.Fatalf("hard-delete repository method exists: %s", function.Name.Name)
		}
	}

	queries, err := parser.ParseFile(fileSet, "../../internal/rbac/queries.go", nil, 0)
	if err != nil {
		t.Fatalf("parse queries: %v", err)
	}
	ast.Inspect(queries, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if ok && strings.Contains(strings.ToLower(literal.Value), "delete from circles") {
			t.Errorf("hard-delete SQL exists in circle queries")
		}
		return true
	})
}

func TestCircleHardDeleteSafety_FeatureMigrationAddsNoCascade(t *testing.T) {
	for _, migration := range []string{
		"000013_create_circles.up.sql",
		"000014_circle_members_circle_fk.up.sql",
		"000015_circle_management.up.sql",
	} {
		contents, err := os.ReadFile("../../migrations/" + migration)
		if err != nil {
			t.Fatalf("read %s: %v", migration, err)
		}
		normalized := strings.ToLower(string(contents))
		if strings.Contains(normalized, "delete from circles") || strings.Contains(normalized, "on delete cascade") {
			t.Errorf("%s introduces destructive circle deletion", migration)
		}
	}
}

func TestCircleHardDeleteSafety_DeleteRouteArchivesWithoutResponseLeak(t *testing.T) {
	store := newCircleStoreStub()
	store.circles[contractCircleID] = rbac.Circle{
		ID: contractCircleID, Name: "Retained Circle", CreatedAt: time.Now().UTC(),
	}
	store.members[contractCircleID] = map[string]string{
		testLocalUserID:   rbac.RoleTeacher,
		contractStudentID: rbac.RoleStudent,
	}
	req := httptest.NewRequest(http.MethodDelete, "/circles/"+contractCircleID, nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()

	buildCircleRetirementRoute(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("archive response: status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !store.circles[contractCircleID].IsArchived || len(store.members[contractCircleID]) != 2 {
		t.Fatal("DELETE route must archive while retaining circle memberships")
	}
}
