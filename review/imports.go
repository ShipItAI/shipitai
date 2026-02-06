package review

import (
	"path/filepath"
	"regexp"
	"strings"
)

// DetectLanguage returns the programming language based on file extension.
func DetectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py":
		return "python"
	case ".rb":
		return "ruby"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".swift":
		return "swift"
	case ".rs":
		return "rust"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp", ".hxx":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".php":
		return "php"
	case ".scala":
		return "scala"
	case ".ex", ".exs":
		return "elixir"
	default:
		return ""
	}
}

// GetTestFilePath returns the expected test file path for a given source file.
// Returns empty string if no test convention is known for the language.
func GetTestFilePath(path string) string {
	lang := DetectLanguage(path)
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	switch lang {
	case "go":
		// foo.go -> foo_test.go
		// Skip if already a test file
		if strings.HasSuffix(name, "_test") {
			return ""
		}
		return filepath.Join(dir, name+"_test.go")

	case "typescript", "javascript":
		// foo.ts -> foo.test.ts or foo.spec.ts
		// Also handle .tsx -> .test.tsx
		if strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".spec") {
			return ""
		}
		// Return .test variant as primary
		return filepath.Join(dir, name+".test"+ext)

	case "python":
		// foo.py -> test_foo.py (pytest convention)
		// Also: foo.py -> tests/test_foo.py
		if strings.HasPrefix(name, "test_") {
			return ""
		}
		return filepath.Join(dir, "test_"+name+ext)

	case "ruby":
		// foo.rb -> foo_spec.rb (rspec) or foo_test.rb (minitest)
		if strings.HasSuffix(name, "_spec") || strings.HasSuffix(name, "_test") {
			return ""
		}
		return filepath.Join(dir, name+"_spec.rb")

	case "java", "kotlin":
		// src/main/java/com/foo/Bar.java -> src/test/java/com/foo/BarTest.java
		if strings.HasSuffix(name, "Test") {
			return ""
		}
		testPath := strings.Replace(path, "/main/", "/test/", 1)
		testDir := filepath.Dir(testPath)
		return filepath.Join(testDir, name+"Test"+ext)

	case "rust":
		// Rust typically uses inline #[cfg(test)] modules
		// but also has tests/integration_test.rs
		return ""

	default:
		return ""
	}
}

// ParseLocalImports extracts local import paths from file content.
// modulePath is the Go module path (e.g., "github.com/user/repo") for Go files.
// Returns paths relative to repository root where possible.
func ParseLocalImports(path, content, modulePath string) []string {
	lang := DetectLanguage(path)
	var imports []string

	switch lang {
	case "go":
		imports = parseGoImports(content, modulePath)
	case "typescript", "javascript":
		imports = parseTSImports(path, content)
	case "python":
		imports = parsePythonImports(path, content)
	}

	return imports
}

// parseGoImports extracts local package imports from Go source.
func parseGoImports(content, modulePath string) []string {
	if modulePath == "" {
		return nil
	}

	var imports []string

	// Match import statements: import "path" or import ( "path" )
	// Single import
	singleRe := regexp.MustCompile(`import\s+"([^"]+)"`)
	for _, match := range singleRe.FindAllStringSubmatch(content, -1) {
		if strings.HasPrefix(match[1], modulePath+"/") {
			// Convert module path to relative path
			relPath := strings.TrimPrefix(match[1], modulePath+"/")
			imports = append(imports, relPath)
		}
	}

	// Grouped imports
	groupRe := regexp.MustCompile(`import\s*\(([^)]+)\)`)
	for _, group := range groupRe.FindAllStringSubmatch(content, -1) {
		lines := strings.Split(group[1], "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Handle: "path" or alias "path"
			pathRe := regexp.MustCompile(`(?:\w+\s+)?"([^"]+)"`)
			if m := pathRe.FindStringSubmatch(line); m != nil {
				if strings.HasPrefix(m[1], modulePath+"/") {
					relPath := strings.TrimPrefix(m[1], modulePath+"/")
					imports = append(imports, relPath)
				}
			}
		}
	}

	return imports
}

// parseTSImports extracts relative imports from TypeScript/JavaScript source.
func parseTSImports(filePath, content string) []string {
	var imports []string
	fileDir := filepath.Dir(filePath)

	// Match: import ... from './path' or import ... from '../path'
	// Also: import './path' (side effect imports)
	// Also: require('./path')
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`import\s+.*?\s+from\s+['"](\.[^'"]+)['"]`),
		regexp.MustCompile(`import\s+['"](\.[^'"]+)['"]`),
		regexp.MustCompile(`require\s*\(\s*['"](\.[^'"]+)['"]\s*\)`),
	}

	for _, re := range patterns {
		for _, match := range re.FindAllStringSubmatch(content, -1) {
			importPath := match[1]
			// Resolve relative path
			resolved := resolveRelativeImport(fileDir, importPath)
			if resolved != "" {
				imports = append(imports, resolved)
			}
		}
	}

	// Also handle @/ style imports (common in Next.js, Vue)
	// These typically map to src/ or root
	atRe := regexp.MustCompile(`import\s+.*?\s+from\s+['"]@/([^'"]+)['"]`)
	for _, match := range atRe.FindAllStringSubmatch(content, -1) {
		// Assume @/ maps to src/ (most common)
		imports = append(imports, "src/"+match[1])
	}

	return imports
}

// parsePythonImports extracts relative imports from Python source.
func parsePythonImports(filePath, content string) []string {
	var imports []string
	fileDir := filepath.Dir(filePath)

	// Match: from . import foo or from .foo import bar or from ..foo import bar
	relativeRe := regexp.MustCompile(`from\s+(\.+)(\w*)\s+import`)
	for _, match := range relativeRe.FindAllStringSubmatch(content, -1) {
		dots := match[1]
		module := match[2]

		// Calculate parent directory based on dot count
		targetDir := fileDir
		for i := 1; i < len(dots); i++ {
			targetDir = filepath.Dir(targetDir)
		}

		if module != "" {
			// from .foo import bar -> foo.py or foo/__init__.py
			imports = append(imports, filepath.Join(targetDir, module+".py"))
		}
	}

	return imports
}

// resolveRelativeImport converts a relative import path to a repository-relative path.
func resolveRelativeImport(fileDir, importPath string) string {
	// Join the file's directory with the import path
	resolved := filepath.Join(fileDir, importPath)
	// Clean up the path
	resolved = filepath.Clean(resolved)

	// Handle paths that might need file extensions
	// Check common extensions if none present
	if filepath.Ext(resolved) == "" {
		// Return base path - caller should try with extensions
		return resolved
	}

	return resolved
}

// GetRelatedTestPaths returns all possible test file paths for a source file.
// This handles multiple conventions per language.
func GetRelatedTestPaths(path string) []string {
	var paths []string
	primary := GetTestFilePath(path)
	if primary != "" {
		paths = append(paths, primary)
	}

	lang := DetectLanguage(path)
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	switch lang {
	case "typescript", "javascript":
		// Also try .spec variant
		if primary != "" && strings.Contains(primary, ".test") {
			specPath := filepath.Join(dir, name+".spec"+ext)
			paths = append(paths, specPath)
		}
		// Try __tests__ directory
		paths = append(paths, filepath.Join(dir, "__tests__", base))
		paths = append(paths, filepath.Join(dir, "__tests__", name+".test"+ext))

	case "python":
		// Also try tests/ subdirectory
		paths = append(paths, filepath.Join(dir, "tests", "test_"+name+ext))
		// conftest.py for fixtures
		paths = append(paths, filepath.Join(dir, "conftest.py"))
	}

	return paths
}
