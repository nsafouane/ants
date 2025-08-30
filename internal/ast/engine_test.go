package ast

import (
	"context"
	"go/ast"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestEngine_New(t *testing.T) {
	t.Parallel()

	cfg := &config.ASTProcessingConfig{
		EnableCaching:      true,
		CacheTTLSeconds:    1800,
		MaxCacheSizeMB:     64,
		SupportedLanguages: []string{"go", "python"},
	}

	engine := New(cfg)

	require.NotNil(t, engine)
	require.Equal(t, cfg, engine.config)
	require.NotEmpty(t, engine.fileSet)
	require.NotNil(t, engine.cache)
}

func TestEngine_New_WithDefaults(t *testing.T) {
	t.Parallel()

	engine := New(nil)

	require.NotNil(t, engine)
	require.NotNil(t, engine.config)
	require.True(t, engine.config.EnableCaching)
	require.Equal(t, 3600, engine.config.CacheTTLSeconds)
	require.Equal(t, 128, engine.config.MaxCacheSizeMB)
}

func TestEngine_ParseGoFile_SimpleFunction(t *testing.T) {
	t.Parallel()

	engine := New(nil)
	
	goCode := `package main

import "fmt"

// Hello prints a greeting message
func Hello(name string) string {
	return fmt.Sprintf("Hello, %s!", name)
}

func main() {
	fmt.Println(Hello("World"))
}`

	ctx := context.Background()
	result, err := engine.ParseFile(ctx, "test.go", []byte(goCode), "go")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "test.go", result.FilePath)
	require.Equal(t, "go", result.Language)
	require.NotNil(t, result.Root)
	require.GreaterOrEqual(t, result.ParseTime, time.Duration(0))
	require.NotEmpty(t, result.ContentHash)

	// Should have found symbols
	require.NotEmpty(t, result.Symbols)
	
	// Check for Hello function
	var helloFunc *Symbol
	for _, symbol := range result.Symbols {
		if symbol.Name == "Hello" && symbol.Type == "function" {
			helloFunc = symbol
			break
		}
	}
	require.NotNil(t, helloFunc, "Should find Hello function")
	require.True(t, helloFunc.IsExported)
	require.Equal(t, "function", helloFunc.Kind)
	require.Len(t, helloFunc.Parameters, 1)
	require.Equal(t, "name", helloFunc.Parameters[0].Name)
	require.Equal(t, "string", helloFunc.Parameters[0].Type)

	// Check for main function
	var mainFunc *Symbol
	for _, symbol := range result.Symbols {
		if symbol.Name == "main" && symbol.Type == "function" {
			mainFunc = symbol
			break
		}
	}
	require.NotNil(t, mainFunc, "Should find main function")
	require.False(t, mainFunc.IsExported)

	// Should have found import
	require.NotEmpty(t, result.Dependencies)
	
	var fmtImport *Dependency
	for _, dep := range result.Dependencies {
		if dep.Target == "fmt" {
			fmtImport = dep
			break
		}
	}
	require.NotNil(t, fmtImport, "Should find fmt import")
	require.Equal(t, "import", fmtImport.Type)
	require.False(t, fmtImport.IsLocal)
}

func TestEngine_ParseGoFile_Struct(t *testing.T) {
	t.Parallel()

	engine := New(nil)
	
	goCode := `package models

// User represents a user in the system
type User struct {
	ID       int64  ` + "`json:\"id\"`" + `
	Name     string ` + "`json:\"name\"`" + `
	Email    string ` + "`json:\"email\"`" + `
	IsActive bool   ` + "`json:\"is_active\"`" + `
}

// GetDisplayName returns the user's display name
func (u *User) GetDisplayName() string {
	return u.Name
}

// Validate checks if the user data is valid
func (u *User) Validate() error {
	if u.Name == "" {
		return errors.New("name is required")
	}
	return nil
}`

	ctx := context.Background()
	result, err := engine.ParseFile(ctx, "user.go", []byte(goCode), "go")

	require.NoError(t, err)
	require.NotNil(t, result)

	// Should find the User type
	var userType *Symbol
	for _, symbol := range result.Symbols {
		if symbol.Name == "User" && symbol.Type == "type" {
			userType = symbol
			break
		}
	}
	require.NotNil(t, userType, "Should find User type")
	require.True(t, userType.IsExported)
	require.Equal(t, "struct", userType.Kind)

	// Should find the methods
	var getDisplayNameMethod, validateMethod *Symbol
	for _, symbol := range result.Symbols {
		if symbol.Name == "GetDisplayName" && symbol.Kind == "method" {
			getDisplayNameMethod = symbol
		} else if symbol.Name == "Validate" && symbol.Kind == "method" {
			validateMethod = symbol
		}
	}
	require.NotNil(t, getDisplayNameMethod, "Should find GetDisplayName method")
	require.True(t, getDisplayNameMethod.IsExported)
	
	require.NotNil(t, validateMethod, "Should find Validate method")
	require.True(t, validateMethod.IsExported)
}

func TestEngine_ParseGoFile_Variables(t *testing.T) {
	t.Parallel()

	engine := New(nil)
	
	goCode := `package config

import "time"

const (
	DefaultTimeout = 30 * time.Second
	MaxRetries     = 3
)

var (
	Debug   = false
	Version = "1.0.0"
)`

	ctx := context.Background()
	result, err := engine.ParseFile(ctx, "config.go", []byte(goCode), "go")

	require.NoError(t, err)
	require.NotNil(t, result)

	// Count constants and variables
	var constants, variables int
	for _, symbol := range result.Symbols {
		switch symbol.Type {
		case "constant":
			constants++
			require.True(t, symbol.IsExported)
		case "variable":
			variables++
			require.True(t, symbol.IsExported)
		}
	}

	require.Greater(t, constants, 0, "Should find constants")
	require.Greater(t, variables, 0, "Should find variables")
}

func TestEngine_ParseGoFile_Interface(t *testing.T) {
	t.Parallel()

	engine := New(nil)
	
	goCode := `package interfaces

// Writer defines the interface for writing data
type Writer interface {
	Write(data []byte) (int, error)
	Close() error
}

// ReadWriter combines Reader and Writer interfaces
type ReadWriter interface {
	Reader
	Writer
}`

	ctx := context.Background()
	result, err := engine.ParseFile(ctx, "interfaces.go", []byte(goCode), "go")

	require.NoError(t, err)
	require.NotNil(t, result)

	// Should find the interfaces
	var writerInterface, readWriterInterface *Symbol
	for _, symbol := range result.Symbols {
		if symbol.Name == "Writer" && symbol.Type == "type" {
			writerInterface = symbol
		} else if symbol.Name == "ReadWriter" && symbol.Type == "type" {
			readWriterInterface = symbol
		}
	}
	
	require.NotNil(t, writerInterface, "Should find Writer interface")
	require.Equal(t, "interface", writerInterface.Kind)
	require.True(t, writerInterface.IsExported)
	
	require.NotNil(t, readWriterInterface, "Should find ReadWriter interface")
	require.Equal(t, "interface", readWriterInterface.Kind)
	require.True(t, readWriterInterface.IsExported)
}

func TestEngine_ParseGoFile_LocalImports(t *testing.T) {
	t.Parallel()

	engine := New(nil)
	
	goCode := `package main

import (
	"fmt"
	"context"
	
	"github.com/example/project/internal/models"
	"github.com/example/project/pkg/utils"
	
	"./local"
	"../relative"
)`

	ctx := context.Background()
	result, err := engine.ParseFile(ctx, "main.go", []byte(goCode), "go")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Dependencies)

	// Categorize imports
	var standardLib, external, local int
	for _, dep := range result.Dependencies {
		switch dep.Target {
		case "fmt", "context":
			standardLib++
			require.False(t, dep.IsLocal)
		case "github.com/example/project/internal/models", "github.com/example/project/pkg/utils":
			external++
			require.False(t, dep.IsLocal)
		case "./local", "../relative":
			local++
			require.True(t, dep.IsLocal)
		}
	}

	require.Equal(t, 2, standardLib)
	require.Equal(t, 2, external)
	require.Equal(t, 2, local)
}

func TestEngine_Caching(t *testing.T) {
	t.Parallel()

	cfg := &config.ASTProcessingConfig{
		EnableCaching:   true,
		CacheTTLSeconds: 1,
		MaxCacheSizeMB:  1,
	}
	engine := New(cfg)
	
	goCode := `package main
func main() {}`

	ctx := context.Background()
	
	// First parse
	result1, err := engine.ParseFile(ctx, "test.go", []byte(goCode), "go")
	require.NoError(t, err)
	require.NotNil(t, result1)

	// Second parse should use cache (same content)
	result2, err := engine.ParseFile(ctx, "test.go", []byte(goCode), "go")
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.Equal(t, result1.ContentHash, result2.ContentHash)

	// Different content should not use cache
	differentCode := `package main
func Hello() {}`
	result3, err := engine.ParseFile(ctx, "test.go", []byte(differentCode), "go")
	require.NoError(t, err)
	require.NotNil(t, result3)
	require.NotEqual(t, result1.ContentHash, result3.ContentHash)

	// Wait for cache to expire
	time.Sleep(1100 * time.Millisecond)
	
	// Should reparse after TTL expiry
	result4, err := engine.ParseFile(ctx, "test.go", []byte(goCode), "go")
	require.NoError(t, err)
	require.NotNil(t, result4)
	require.Equal(t, result1.ContentHash, result4.ContentHash)
}

func TestEngine_CacheStats(t *testing.T) {
	t.Parallel()

	engine := New(nil)
	
	// Initially empty
	stats := engine.GetCacheStats()
	require.Equal(t, 0, stats["entries"])
	require.Equal(t, int64(0), stats["size_bytes"])

	// Parse a file to add to cache
	goCode := `package main
func main() {}`
	
	ctx := context.Background()
	_, err := engine.ParseFile(ctx, "test.go", []byte(goCode), "go")
	require.NoError(t, err)

	// Should have cache entry
	stats = engine.GetCacheStats()
	require.Equal(t, 1, stats["entries"])
	require.Greater(t, stats["size_bytes"], int64(0))
	require.LessOrEqual(t, stats["utilization"].(float64), 1.0)

	// Clear cache
	engine.ClearCache()
	stats = engine.GetCacheStats()
	require.Equal(t, 0, stats["entries"])
	require.Equal(t, int64(0), stats["size_bytes"])
}

func TestEngine_UnsupportedLanguage(t *testing.T) {
	t.Parallel()

	engine := New(nil)
	
	code := `print("Hello, World!")`
	
	ctx := context.Background()
	result, err := engine.ParseFile(ctx, "test.py", []byte(code), "python")
	
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "only Go is currently supported")
}

func TestEngine_isLanguageSupported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		supportedLanguages []string
		testLanguage       string
		expected           bool
	}{
		{
			name:               "empty list supports all",
			supportedLanguages: []string{},
			testLanguage:       "go",
			expected:           true,
		},
		{
			name:               "language in list",
			supportedLanguages: []string{"go", "python", "javascript"},
			testLanguage:       "go",
			expected:           true,
		},
		{
			name:               "language not in list",
			supportedLanguages: []string{"python", "javascript"},
			testLanguage:       "go",
			expected:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.ASTProcessingConfig{
				SupportedLanguages: tt.supportedLanguages,
			}
			engine := New(cfg)
			
			result := engine.isLanguageSupported(tt.testLanguage)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestUtilityFunctions(t *testing.T) {
	t.Parallel()

	t.Run("isExportedName", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			expected bool
		}{
			{"exported function", "HelloWorld", true},
			{"unexported function", "helloWorld", false},
			{"exported constant", "MAX_VALUE", true},
			{"unexported variable", "debugMode", false},
			{"empty string", "", false},
			{"single uppercase", "A", true},
			{"single lowercase", "a", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := ast.IsExported(tt.input)
				require.Equal(t, tt.expected, result, "isExportedName(%q)", tt.input)
			})
		}
	})

	t.Run("isLocalImport", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			expected bool
		}{
			{"relative current", "./local", true},
			{"relative parent", "../parent", true},
			{"absolute path", "/usr/local/lib", true},
			{"external package", "github.com/user/repo", false},
			{"standard library", "fmt", false}, // Standard library import
			{"empty string", "", false},
			{"dot only", ".", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := isLocalImport(tt.input)
				require.Equal(t, tt.expected, result, "isLocalImport(%q)", tt.input)
			})
		}
	})

	t.Run("containsDot", func(t *testing.T) {
		tests := []struct {
			input    string
			expected bool
		}{
			{"github.com", true},
			{"simple", false},
			{"", false},
			{".", true},
			{"a.b.c", true},
		}

		for _, tt := range tests {
			result := containsDot(tt.input)
			require.Equal(t, tt.expected, result, "containsDot(%q)", tt.input)
		}
	})
}

func BenchmarkEngine_ParseGoFile(b *testing.B) {
	engine := New(nil)
	
	goCode := `package main

import (
	"fmt"
	"context"
	"time"
)

type User struct {
	ID   int64
	Name string
}

func (u *User) String() string {
	return fmt.Sprintf("User{ID: %d, Name: %s}", u.ID, u.Name)
}

func main() {
	ctx := context.Background()
	user := &User{ID: 1, Name: "John"}
	fmt.Println(user.String())
}`

	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.ParseFile(ctx, "test.go", []byte(goCode), "go")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngine_ParseGoFile_WithCache(b *testing.B) {
	cfg := &config.ASTProcessingConfig{
		EnableCaching:   true,
		CacheTTLSeconds: 3600,
		MaxCacheSizeMB:  64,
	}
	engine := New(cfg)
	
	goCode := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}`

	ctx := context.Background()
	
	// Warm up cache
	_, err := engine.ParseFile(ctx, "test.go", []byte(goCode), "go")
	if err != nil {
		b.Fatal(err)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.ParseFile(ctx, "test.go", []byte(goCode), "go")
		if err != nil {
			b.Fatal(err)
		}
	}
}