package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

const sessionPackagePath = "github.com/trigosec/coderoom/internal/session"

func TestArchitecture_UIHasNoDirectSessionExecuteCalls(t *testing.T) {
	fileSet := token.NewFileSet()
	packages := map[string][]*ast.File{}
	err := filepath.WalkDir(".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fileSet, name, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", name, err)
		}
		directory := filepath.Dir(name)
		packages[directory] = append(packages[directory], file)
		return nil
	})
	if err != nil {
		t.Fatalf("inspect UI source: %v", err)
	}

	for directory, files := range packages {
		for _, position := range directSessionExecutePositions(fileSet, directory, files) {
			t.Errorf("direct session.Execute call at %s; route session commands through the interpreter", position)
		}
	}
}

func directSessionExecutePositions(fileSet *token.FileSet, packagePath string, files []*ast.File) []token.Position {
	info := &types.Info{Selections: make(map[*ast.SelectorExpr]*types.Selection)}
	config := types.Config{
		Importer: newBoundaryImporter(),
		Error:    func(error) {}, // Unrelated imports are intentionally skeletal.
	}
	_, _ = config.Check(packagePath, fileSet, files, info)

	var positions []token.Position
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Execute" {
				return true
			}
			selection := info.Selections[selector]
			if selection == nil || selection.Obj().Pkg() == nil || selection.Obj().Pkg().Path() != sessionPackagePath {
				return true
			}
			positions = append(positions, fileSet.Position(selector.Pos()))
			return true
		})
	}
	return positions
}

type boundaryImporter struct {
	packages map[string]*types.Package
}

func newBoundaryImporter() *boundaryImporter {
	return &boundaryImporter{packages: make(map[string]*types.Package)}
}

func (i *boundaryImporter) Import(path string) (*types.Package, error) {
	if imported, ok := i.packages[path]; ok {
		return imported, nil
	}
	imported := types.NewPackage(path, filepath.Base(path))
	i.packages[path] = imported
	if path == sessionPackagePath {
		addSessionType(imported)
	}
	imported.MarkComplete()
	return imported, nil
}

func addSessionType(pkg *types.Package) {
	typeName := types.NewTypeName(token.NoPos, pkg, "Session", nil)
	sessionType := types.NewNamed(typeName, types.NewStruct(nil, nil), nil)
	pkg.Scope().Insert(typeName)
	receiver := types.NewVar(token.NoPos, pkg, "s", types.NewPointer(sessionType))
	command := types.NewVar(token.NoPos, pkg, "command", types.NewInterfaceType(nil, nil).Complete())
	result := types.NewVar(token.NoPos, pkg, "", types.Universe.Lookup("error").Type())
	signature := types.NewSignatureType(
		receiver,
		nil,
		nil,
		types.NewTuple(command),
		types.NewTuple(result),
		false,
	)
	sessionType.AddMethod(types.NewFunc(token.NoPos, pkg, "Execute", signature))
}

func TestDirectSessionExecutePositions_resolvesReceiverTypes(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "boundary.go", `package boundary
import "github.com/trigosec/coderoom/internal/session"
type executor struct{}
func (executor) Execute(any) error { return nil }
func direct(differentlyNamed *session.Session, other executor) {
	alias := differentlyNamed
	_ = alias.Execute(nil)
	_ = other.Execute(nil)
}`, 0)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	positions := directSessionExecutePositions(fileSet, "boundary", []*ast.File{file})
	if len(positions) != 1 {
		t.Fatalf("direct session.Execute calls = %d, want 1", len(positions))
	}
}
