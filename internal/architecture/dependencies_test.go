package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type dependencyRule struct {
	name              string
	packagePrefixes   []string
	excludePrefixes   []string
	disallowedImports []string
}

type packageImports struct {
	importPath string
	files      map[string][]string
}

func TestArchitectureDependencies(t *testing.T) {
	modulePath, packages := loadPackageImports(t)

	rules := []dependencyRule{
		{
			name: "application core is independent from concrete infrastructure",
			packagePrefixes: []string{
				modulePath + "/internal/app/subscriptions",
				modulePath + "/internal/app/release_tracking",
				modulePath + "/internal/app/notifications",
				modulePath + "/internal/notification/notifications",
			},
			excludePrefixes: []string{
				modulePath + "/internal/app/subscriptions/repository",
				modulePath + "/internal/app/release_tracking/repository",
				modulePath + "/internal/notification/notifications/repository",
			},
			disallowedImports: []string{
				"database/sql",
				"github.com/gin-gonic/gin",
				"github.com/rabbitmq/amqp091-go",
				modulePath + "/internal/app/platform/http",
				modulePath + "/internal/app/platform/github",
				modulePath + "/internal/app/platform/messaging/rabbitmq",
				modulePath + "/internal/app/subscriptions/repository",
				modulePath + "/internal/app/release_tracking/repository",
				modulePath + "/internal/notification/notifications/repository",
				modulePath + "/internal/notification/platform/mail",
				modulePath + "/internal/notification/platform/messaging",
				modulePath + "/internal/platform/config",
				modulePath + "/internal/platform/db",
			},
		},
		{
			name: "repositories do not depend on delivery adapters",
			packagePrefixes: []string{
				modulePath + "/internal/app/subscriptions/repository",
				modulePath + "/internal/app/release_tracking/repository",
				modulePath + "/internal/notification/notifications/repository",
			},
			disallowedImports: []string{
				"github.com/gin-gonic/gin",
				"github.com/rabbitmq/amqp091-go",
				modulePath + "/cmd",
				modulePath + "/internal/app/platform/http",
				modulePath + "/internal/app/platform/github",
				modulePath + "/internal/app/platform/messaging/rabbitmq",
				modulePath + "/internal/notification/platform/mail",
				modulePath + "/internal/notification/platform/messaging",
				modulePath + "/internal/platform/config",
			},
		},
		{
			name: "shared package stays independent",
			packagePrefixes: []string{
				modulePath + "/internal/shared",
			},
			disallowedImports: []string{
				modulePath + "/cmd",
				modulePath + "/internal/app",
				modulePath + "/internal/notification",
				modulePath + "/internal/platform",
			},
		},
		{
			name: "contracts stay independent from internal packages",
			packagePrefixes: []string{
				modulePath + "/pkg/contracts",
			},
			disallowedImports: []string{
				modulePath + "/cmd",
				modulePath + "/internal",
			},
		},
		{
			name: "app service does not depend on notification service internals",
			packagePrefixes: []string{
				modulePath + "/internal/app",
			},
			disallowedImports: []string{
				modulePath + "/internal/notification",
			},
		},
		{
			name: "notification service does not depend on app service internals",
			packagePrefixes: []string{
				modulePath + "/internal/notification",
			},
			disallowedImports: []string{
				modulePath + "/internal/app",
			},
		},
	}

	for _, rule := range rules {
		t.Run(rule.name, func(t *testing.T) {
			for _, pkg := range packages {
				if !matchesAnyPrefix(pkg.importPath, rule.packagePrefixes) {
					continue
				}
				if matchesAnyPrefix(pkg.importPath, rule.excludePrefixes) {
					continue
				}

				for file, imports := range pkg.files {
					for _, imported := range imports {
						if matchesAnyPrefix(imported, rule.disallowedImports) {
							t.Errorf("%s imports %s in %s", pkg.importPath, imported, file)
						}
					}
				}
			}
		})
	}
}

func loadPackageImports(t *testing.T) (string, []packageImports) {
	t.Helper()

	root := findRepositoryRoot(t)
	modulePath := readModulePath(t, filepath.Join(root, "go.mod"))

	packagesByImportPath := make(map[string]*packageImports)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		imports := parseImports(t, path)
		if len(imports) == 0 {
			return nil
		}

		relDir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		importPath := modulePath
		if relDir != "." {
			importPath += "/" + filepath.ToSlash(relDir)
		}

		pkg := packagesByImportPath[importPath]
		if pkg == nil {
			pkg = &packageImports{importPath: importPath, files: map[string][]string{}}
			packagesByImportPath[importPath] = pkg
		}

		relFile, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		pkg.files[filepath.ToSlash(relFile)] = imports
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}

	packages := make([]packageImports, 0, len(packagesByImportPath))
	for _, pkg := range packagesByImportPath {
		packages = append(packages, *pkg)
	}

	return modulePath, packages
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func readModulePath(t *testing.T, goModPath string) string {
	t.Helper()

	data, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}

	t.Fatal("module path not found in go.mod")
	return ""
}

func parseImports(t *testing.T, path string) []string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports in %s: %v", path, err)
	}

	imports := make([]string, 0, len(file.Imports))
	for _, imported := range file.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatalf("unquote import in %s: %v", path, err)
		}
		imports = append(imports, value)
	}

	return imports
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".idea", ".agents", "docs", "documentation", "migrations", "observability":
		return true
	default:
		return false
	}
}

func matchesAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if value == prefix || strings.HasPrefix(value, prefix+"/") {
			return true
		}
	}
	return false
}
