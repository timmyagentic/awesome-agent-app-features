package contract

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestV1PublicAPIMatchesSnapshot(t *testing.T) {
	root := repositoryRoot(t)
	packages := []string{"feedback", "feedback/httpclient", "updater", "updater/github"}
	actual := publicAPISnapshot(t, root, packages)
	if strings.Contains(actual, "<inferred>") {
		t.Fatal("exported variables must declare an explicit type for the v1 API contract")
	}
	snapshotPath := filepath.Join(root, "api", "v1.txt")
	if os.Getenv("UPDATE_API_SNAPSHOT") == "1" {
		if err := os.WriteFile(snapshotPath, []byte(actual), 0o644); err != nil {
			t.Fatalf("write API snapshot: %v", err)
		}
		return
	}
	want, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read API snapshot: %v", err)
	}
	if actual != string(want) {
		t.Fatalf("public API changed; update api/v1.txt only after a compatibility decision\n\n%s", actual)
	}
}

func publicAPISnapshot(t *testing.T, root string, packageDirectories []string) string {
	t.Helper()
	var snapshot strings.Builder
	for _, directory := range packageDirectories {
		absolute := filepath.Join(root, filepath.FromSlash(directory))
		fileSet := token.NewFileSet()
		entries, err := os.ReadDir(absolute)
		if err != nil {
			t.Fatalf("read public package %s: %v", directory, err)
		}
		var declarations []string
		packageNames := make(map[string]struct{})
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fileSet, filepath.Join(absolute, entry.Name()), nil, 0)
			if err != nil {
				t.Fatalf("parse %s/%s: %v", directory, entry.Name(), err)
			}
			packageNames[file.Name.Name] = struct{}{}
			for _, declaration := range file.Decls {
				declarations = append(declarations, exportedDeclaration(fileSet, declaration)...)
			}
		}
		if len(packageNames) != 1 {
			t.Fatalf("package count in %s = %d", directory, len(packageNames))
		}
		sort.Strings(declarations)
		fmt.Fprintf(&snapshot, "package github.com/timmyagentic/awesome-agent-app-features/%s\n", directory)
		for _, declaration := range declarations {
			snapshot.WriteString(declaration)
			snapshot.WriteByte('\n')
		}
		snapshot.WriteByte('\n')
	}
	return snapshot.String()
}

func exportedDeclaration(fileSet *token.FileSet, declaration ast.Decl) []string {
	switch value := declaration.(type) {
	case *ast.FuncDecl:
		if !value.Name.IsExported() {
			return nil
		}
		prefix := "func "
		if value.Recv != nil && len(value.Recv.List) == 1 {
			if !exportedReceiver(value.Recv.List[0].Type) {
				return nil
			}
			prefix += "(" + expression(fileSet, value.Recv.List[0].Type) + ") "
		}
		return []string{prefix + value.Name.Name + functionSignature(fileSet, value.Type)}
	case *ast.GenDecl:
		var result []string
		for _, spec := range value.Specs {
			switch item := spec.(type) {
			case *ast.TypeSpec:
				if !item.Name.IsExported() {
					continue
				}
				operator := " "
				if item.Assign.IsValid() {
					operator = " = "
				}
				result = append(result, "type "+item.Name.Name+operator+publicType(fileSet, item.Type))
			case *ast.ValueSpec:
				for index, name := range item.Names {
					if !name.IsExported() {
						continue
					}
					line := value.Tok.String() + " " + name.Name
					if item.Type != nil {
						line += " " + expression(fileSet, item.Type)
					}
					if value.Tok == token.VAR && item.Type == nil {
						// The exact initializer (especially errors.New text) is not
						// part of the compatibility contract. The external consumer
						// test still proves the inferred value can be used.
						line += " <inferred>"
					} else if value.Tok != token.VAR && len(item.Values) > 0 {
						valueIndex := index
						if valueIndex >= len(item.Values) {
							valueIndex = len(item.Values) - 1
						}
						line += " = " + expression(fileSet, item.Values[valueIndex])
					}
					result = append(result, line)
				}
			}
		}
		return result
	default:
		return nil
	}
}

func publicType(fileSet *token.FileSet, value ast.Expr) string {
	switch item := value.(type) {
	case *ast.StructType:
		var fields []string
		for _, field := range item.Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			for _, name := range field.Names {
				if !name.IsExported() {
					continue
				}
				line := name.Name + " " + publicType(fileSet, field.Type)
				if field.Tag != nil {
					line += " " + field.Tag.Value
				}
				fields = append(fields, line)
			}
		}
		if len(fields) == 0 {
			return "struct{opaque}"
		}
		return "struct{" + strings.Join(fields, "; ") + "}"
	case *ast.InterfaceType:
		var methods []string
		for _, field := range item.Methods.List {
			if len(field.Names) != 1 || !field.Names[0].IsExported() {
				continue
			}
			function, ok := field.Type.(*ast.FuncType)
			if !ok {
				continue
			}
			methods = append(methods, field.Names[0].Name+functionSignature(fileSet, function))
		}
		sort.Strings(methods)
		return "interface{" + strings.Join(methods, "; ") + "}"
	case *ast.FuncType:
		return "func" + functionSignature(fileSet, item)
	default:
		return expression(fileSet, value)
	}
}

func exportedReceiver(value ast.Expr) bool {
	switch receiver := value.(type) {
	case *ast.Ident:
		return receiver.IsExported()
	case *ast.StarExpr:
		return exportedReceiver(receiver.X)
	case *ast.IndexExpr:
		return exportedReceiver(receiver.X)
	case *ast.IndexListExpr:
		return exportedReceiver(receiver.X)
	default:
		return false
	}
}

func functionSignature(fileSet *token.FileSet, function *ast.FuncType) string {
	parameters := fieldTypes(fileSet, function.Params)
	results := fieldTypes(fileSet, function.Results)
	signature := "(" + strings.Join(parameters, ", ") + ")"
	if len(results) > 0 {
		signature += " (" + strings.Join(results, ", ") + ")"
	}
	return signature
}

func fieldTypes(fileSet *token.FileSet, fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	var result []string
	for _, field := range fields.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for index := 0; index < count; index++ {
			result = append(result, expression(fileSet, field.Type))
		}
	}
	return result
}

func expression(fileSet *token.FileSet, value any) string {
	var buffer bytes.Buffer
	if err := format.Node(&buffer, fileSet, value); err != nil {
		return "<invalid>"
	}
	return buffer.String()
}
