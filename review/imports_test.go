package review

import (
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		// Go
		{"main.go", "go"},
		{"internal/pkg/handler.go", "go"},

		// TypeScript/JavaScript
		{"src/app.ts", "typescript"},
		{"components/Button.tsx", "typescript"},
		{"index.js", "javascript"},
		{"App.jsx", "javascript"},
		{"config.mjs", "javascript"},
		{"setup.cjs", "javascript"},

		// Python
		{"main.py", "python"},
		{"tests/test_handler.py", "python"},

		// Other languages
		{"handler.rb", "ruby"},
		{"Main.java", "java"},
		{"App.kt", "kotlin"},
		{"main.swift", "swift"},
		{"lib.rs", "rust"},
		{"main.c", "c"},
		{"utils.cpp", "cpp"},
		{"Program.cs", "csharp"},
		{"index.php", "php"},
		{"Main.scala", "scala"},
		{"lib.ex", "elixir"},

		// Unknown
		{"README.md", ""},
		{"Makefile", ""},
		{".gitignore", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := DetectLanguage(tt.path)
			if got != tt.expected {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestGetTestFilePath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		// Go
		{"handler.go", "handler_test.go"},
		{"internal/pkg/client.go", "internal/pkg/client_test.go"},
		{"handler_test.go", ""}, // Already a test file

		// TypeScript
		{"Button.tsx", "Button.test.tsx"},
		{"utils.ts", "utils.test.ts"},
		{"app.test.ts", ""}, // Already a test file
		{"app.spec.ts", ""}, // Already a test file

		// Python
		{"handler.py", "test_handler.py"},
		{"models/user.py", "models/test_user.py"},
		{"test_handler.py", ""}, // Already a test file

		// Ruby
		{"handler.rb", "handler_spec.rb"},
		{"handler_spec.rb", ""}, // Already a test file

		// Java
		{"src/main/java/com/example/Service.java", "src/test/java/com/example/ServiceTest.java"},
		{"ServiceTest.java", ""}, // Already a test file

		// Rust (no convention)
		{"lib.rs", ""},

		// Unknown extension
		{"README.md", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := GetTestFilePath(tt.path)
			if got != tt.expected {
				t.Errorf("GetTestFilePath(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestGetRelatedTestPaths(t *testing.T) {
	tests := []struct {
		path     string
		minPaths int // Minimum expected paths (varies by language)
	}{
		// Go - just the _test.go variant
		{"handler.go", 1},

		// TypeScript - .test.ts, .spec.ts, __tests__/ variants
		{"Button.tsx", 4},

		// Python - test_foo.py, tests/test_foo.py, conftest.py
		{"handler.py", 3},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := GetRelatedTestPaths(tt.path)
			if len(got) < tt.minPaths {
				t.Errorf("GetRelatedTestPaths(%q) returned %d paths, want at least %d: %v", tt.path, len(got), tt.minPaths, got)
			}
		})
	}
}

func TestParseLocalImports_Go(t *testing.T) {
	modulePath := "github.com/example/myapp"

	content := `package main

import (
	"context"
	"fmt"

	"github.com/example/myapp/internal/handler"
	"github.com/example/myapp/pkg/utils"
	"github.com/other/library"
)

func main() {}
`

	imports := ParseLocalImports("main.go", content, modulePath)

	expected := []string{"internal/handler", "pkg/utils"}
	if len(imports) != len(expected) {
		t.Errorf("got %d imports, want %d", len(imports), len(expected))
	}

	for _, exp := range expected {
		found := false
		for _, imp := range imports {
			if imp == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected import %q not found in %v", exp, imports)
		}
	}
}

func TestParseLocalImports_GoSingleImport(t *testing.T) {
	modulePath := "github.com/example/myapp"

	content := `package main

import "github.com/example/myapp/internal/handler"

func main() {}
`

	imports := ParseLocalImports("main.go", content, modulePath)

	if len(imports) != 1 {
		t.Errorf("got %d imports, want 1", len(imports))
	}
	if len(imports) > 0 && imports[0] != "internal/handler" {
		t.Errorf("got import %q, want %q", imports[0], "internal/handler")
	}
}

func TestParseLocalImports_TypeScript(t *testing.T) {
	content := `import React from 'react';
import { Button } from './components/Button';
import { utils } from '../utils';
import type { User } from './types';
import { globalStyles } from '@/styles/global';
`

	imports := ParseLocalImports("src/App.tsx", content, "")

	// Should find relative imports and @/ imports
	if len(imports) < 3 {
		t.Errorf("got %d imports, want at least 3", len(imports))
	}

	// Check for expected paths
	foundRelative := false
	foundAt := false
	for _, imp := range imports {
		if imp == "src/components/Button" {
			foundRelative = true
		}
		if imp == "src/styles/global" {
			foundAt = true
		}
	}

	if !foundRelative {
		t.Errorf("relative import not found in %v", imports)
	}
	if !foundAt {
		t.Errorf("@/ import not found in %v", imports)
	}
}

func TestParseLocalImports_Python(t *testing.T) {
	content := `from . import utils
from .models import User
from ..shared import helpers
`

	imports := ParseLocalImports("myapp/handlers/main.py", content, "")

	// Should find relative imports
	if len(imports) < 2 {
		t.Errorf("got %d imports, want at least 2", len(imports))
	}
}

func TestParseLocalImports_EmptyModulePath(t *testing.T) {
	content := `package main

import "github.com/example/myapp/internal/handler"
`

	// Without module path, Go imports shouldn't be detected as local
	imports := ParseLocalImports("main.go", content, "")

	if len(imports) != 0 {
		t.Errorf("got %d imports with empty module path, want 0", len(imports))
	}
}

func TestParseLocalImports_UnknownLanguage(t *testing.T) {
	content := `some content`

	imports := ParseLocalImports("README.md", content, "")

	if len(imports) != 0 {
		t.Errorf("got %d imports for unknown language, want 0", len(imports))
	}
}
