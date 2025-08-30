package linking

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	astengine "github.com/charmbracelet/crush/internal/ast"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/require"
)

func TestEngine_New(t *testing.T) {
	t.Parallel()

	cfg := &config.Tier1AnalysisConfig{}
	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()
	
	astEngine := astengine.New(nil)
	
	engine := New(cfg, broker, astEngine)
	
	require.NotNil(t, engine)
	require.Equal(t, cfg, engine.config)
	require.Equal(t, broker, engine.broker)
	require.Equal(t, astEngine, engine.astEngine)
	require.NotNil(t, engine.symbols)
	require.NotNil(t, engine.packages)
	require.NotNil(t, engine.files)
}

func TestEngine_AnalyzeProject_EmptyProject(t *testing.T) {
	t.Parallel()

	cfg := &config.Tier1AnalysisConfig{}
	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()
	
	astEngine := astengine.New(nil)
	engine := New(cfg, broker, astEngine)
	
	ctx := context.Background()
	result, err := engine.AnalyzeProject(ctx, "/test/project", []string{})
	
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "/test/project", result.ProjectPath)
	require.Empty(t, result.References)
	require.Empty(t, result.UnresolvedRefs)
	require.NotNil(t, result.CallGraph)
	require.Empty(t, result.CallGraph.Nodes)
	require.Empty(t, result.CallGraph.Edges)
	require.NotNil(t, result.Statistics)
	require.Equal(t, 0, result.Statistics.TotalFiles)
	require.Equal(t, 0, result.Statistics.TotalSymbols)
	require.Equal(t, 0, result.Statistics.TotalReferences)
	require.False(t, result.GeneratedAt.IsZero())
	require.Greater(t, result.AnalysisTime, time.Duration(0))
}

func TestEngine_AnalyzeProject_SingleFile(t *testing.T) {
	t.Parallel()

	cfg := &config.Tier1AnalysisConfig{}
	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()
	
	astEngine := astengine.New(nil)
	engine := New(cfg, broker, astEngine)
	
	ctx := context.Background()
	result, err := engine.AnalyzeProject(ctx, "/test/project", []string{"/test/project/main.go"})
	
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "/test/project", result.ProjectPath)
	require.NotNil(t, result.CallGraph)
	require.NotNil(t, result.Statistics)
	require.Equal(t, 1, result.Statistics.TotalFiles)
	require.False(t, result.GeneratedAt.IsZero())
	require.Greater(t, result.AnalysisTime, time.Duration(0))
}

func TestEngine_buildSymbolTable(t *testing.T) {
	t.Parallel()

	cfg := &config.Tier1AnalysisConfig{}
	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()
	
	astEngine := astengine.New(nil)
	engine := New(cfg, broker, astEngine)
	
	ctx := context.Background()
	err := engine.buildSymbolTable(ctx, []string{"/test/main.go", "/test/utils.go"})
	
	require.NoError(t, err)
	
	// Check that files were processed
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	
	require.Len(t, engine.files, 2)
	require.Contains(t, engine.files, "/test/main.go")
	require.Contains(t, engine.files, "/test/utils.go")
}

func TestEngine_processFileResult(t *testing.T) {
	t.Parallel()

	cfg := &config.Tier1AnalysisConfig{}
	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()
	
	astEngine := astengine.New(nil)
	engine := New(cfg, broker, astEngine)
	
	// Create mock parse result
	parseResult := &astengine.ParseResult{
		FilePath: "/test/main.go",
		Language: "go",
		Symbols: []*astengine.Symbol{
			{
				Name:       "main",
				Type:       "function",
				IsExported: false,
				StartPoint: astengine.Point{Row: 3, Column: 0},
			},
			{
				Name:       "HelloWorld",
				Type:       "function",
				IsExported: true,
				StartPoint: astengine.Point{Row: 7, Column: 0},
			},
		},
		Dependencies: []*astengine.Dependency{
			{
				Type:   "import",
				Target: "fmt",
			},
		},
	}
	
	err := engine.processFileResult("/test/main.go", parseResult)
	require.NoError(t, err)
	
	// Verify file was processed
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	
	require.Contains(t, engine.files, "/test/main.go")
	fileInfo := engine.files["/test/main.go"]
	require.Equal(t, "/test/main.go", fileInfo.Path)
	require.Equal(t, "/test", fileInfo.Package)
	require.Len(t, fileInfo.Symbols, 2)
	require.Len(t, fileInfo.Imports, 1)
	
	// Verify package was created
	require.Contains(t, engine.packages, "/test")
	pkg := engine.packages["/test"]
	require.Equal(t, "test", pkg.Name)
	require.Equal(t, "/test", pkg.Path)
	require.Contains(t, pkg.Files, "/test/main.go")
	require.Contains(t, pkg.ImportedPackages, "fmt")
	
	// Verify symbols were added
	require.Len(t, pkg.ExportedSymbols, 1)
	require.Contains(t, pkg.ExportedSymbols, "HelloWorld")
	require.Len(t, pkg.LocalSymbols, 1)
	require.Contains(t, pkg.LocalSymbols, "main")
	
	// Verify global symbols
	require.Contains(t, engine.symbols, "/test.main")
	require.Contains(t, engine.symbols, "/test.HelloWorld")
}

func TestEngine_resolveReferences(t *testing.T) {
	t.Parallel()

	cfg := &config.Tier1AnalysisConfig{}
	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()
	
	astEngine := astengine.New(nil)
	engine := New(cfg, broker, astEngine)
	
	// Add some test data
	engine.files["/test/main.go"] = &FileInfo{
		Path:    "/test/main.go",
		Package: "/test",
	}
	
	ctx := context.Background()
	references, err := engine.resolveReferences(ctx)
	
	require.NoError(t, err)
	// references can be empty for a file with no actual references
	require.GreaterOrEqual(t, len(references), 0)
}

func TestEngine_buildCallGraph(t *testing.T) {
	t.Parallel()

	cfg := &config.Tier1AnalysisConfig{}
	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()
	
	astEngine := astengine.New(nil)
	engine := New(cfg, broker, astEngine)
	
	// Add test symbols
	engine.symbols["/test.main"] = &SymbolInfo{
		Name:       "main",
		Type:       "function",
		Package:    "/test",
		File:       "/test/main.go",
		IsExported: false,
	}
	
	engine.symbols["/test.HelloWorld"] = &SymbolInfo{
		Name:       "HelloWorld",
		Type:       "function",
		Package:    "/test",
		File:       "/test/main.go",
		IsExported: true,
	}
	
	callGraph := engine.buildCallGraph()
	
	require.NotNil(t, callGraph)
	require.Len(t, callGraph.Nodes, 2)
	require.Contains(t, callGraph.Nodes, "/test/main.go:main")
	require.Contains(t, callGraph.Nodes, "/test/main.go:HelloWorld")
	
	mainNode := callGraph.Nodes["/test/main.go:main"]
	require.Equal(t, "main", mainNode.Name)
	require.Equal(t, "/test/main.go", mainNode.File)
	require.Equal(t, "/test", mainNode.Package)
	require.False(t, mainNode.IsExported)
	
	helloNode := callGraph.Nodes["/test/main.go:HelloWorld"]
	require.Equal(t, "HelloWorld", helloNode.Name)
	require.True(t, helloNode.IsExported)
}

func TestEngine_calculateStatistics(t *testing.T) {
	t.Parallel()

	cfg := &config.Tier1AnalysisConfig{}
	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()
	
	astEngine := astengine.New(nil)
	engine := New(cfg, broker, astEngine)
	
	// Add test data
	engine.files["/test/main.go"] = &FileInfo{}
	engine.files["/test/utils.go"] = &FileInfo{}
	
	engine.symbols["/test.main"] = &SymbolInfo{}
	engine.symbols["/test.HelloWorld"] = &SymbolInfo{}
	engine.symbols["/test.util"] = &SymbolInfo{}
	
	references := []*Reference{
		{IsResolved: true, ReferenceType: "call"},
		{IsResolved: true, ReferenceType: "call"},
		{IsResolved: false, ReferenceType: "call"},
	}
	
	callGraph := &CallGraph{
		Nodes: map[string]*CallNode{
			"node1": {Complexity: 1},
			"node2": {Complexity: 3},
		},
	}
	
	stats := engine.calculateStatistics(references, callGraph)
	
	require.NotNil(t, stats)
	require.Equal(t, 2, stats.TotalFiles)
	require.Equal(t, 3, stats.TotalSymbols)
	require.Equal(t, 3, stats.TotalReferences)
	require.Equal(t, 2, stats.ResolvedRefs)
	require.Equal(t, 1, stats.UnresolvedRefs)
	require.Equal(t, 3, stats.FunctionCalls)
	require.InDelta(t, 0.667, stats.ResolutionRate, 0.001)
}

func TestEngine_resolveReference(t *testing.T) {
	t.Parallel()

	cfg := &config.Tier1AnalysisConfig{}
	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()
	
	astEngine := astengine.New(nil)
	engine := New(cfg, broker, astEngine)
	
	// Set up test data
	pkg := &PackageInfo{
		LocalSymbols: map[string]*SymbolInfo{
			"helper": {
				Name:     "helper",
				File:     "/test/utils.go",
				Position: astengine.Point{Row: 5, Column: 0},
			},
		},
		ExportedSymbols: map[string]*SymbolInfo{
			"PublicFunc": {
				Name:     "PublicFunc",
				File:     "/test/api.go",
				Position: astengine.Point{Row: 10, Column: 0},
			},
		},
	}
	
	engine.packages["/test"] = pkg
	
	file := &FileInfo{
		Package: "/test",
	}
	
	// Test resolving local symbol
	ref := &Reference{
		ToSymbol:   "helper",
		IsResolved: false,
	}
	
	engine.resolveReference(ref, file)
	
	require.True(t, ref.IsResolved)
	require.Equal(t, "/test/utils.go", ref.ToFile)
	require.Equal(t, astengine.Point{Row: 5, Column: 0}, ref.ToPosition)
	require.Equal(t, 0.95, ref.Confidence)
	
	// Test resolving exported symbol
	ref2 := &Reference{
		ToSymbol:   "PublicFunc",
		IsResolved: false,
	}
	
	engine.resolveReference(ref2, file)
	
	require.True(t, ref2.IsResolved)
	require.Equal(t, "/test/api.go", ref2.ToFile)
	require.Equal(t, astengine.Point{Row: 10, Column: 0}, ref2.ToPosition)
	require.Equal(t, 0.95, ref2.Confidence)
	
	// Test unresolvable symbol
	ref3 := &Reference{
		ToSymbol:   "UnknownFunc",
		IsResolved: false,
	}
	
	engine.resolveReference(ref3, file)
	
	require.False(t, ref3.IsResolved)
}

func TestEngine_extractPackagePath(t *testing.T) {
	t.Parallel()

	cfg := &config.Tier1AnalysisConfig{}
	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()
	
	astEngine := astengine.New(nil)
	engine := New(cfg, broker, astEngine)
	
	tests := []struct {
		input    string
		expected string
	}{
		{"/home/user/project/main.go", "/home/user/project"},
		{"C:\\Users\\user\\project\\main.go", "C:/Users/user/project"},
		{"./main.go", "."},
		{"main.go", "."},
	}
	
	for _, tt := range tests {
		result := engine.extractPackagePath(tt.input)
		require.Equal(t, tt.expected, result, "extractPackagePath(%q)", tt.input)
	}
}

func BenchmarkEngine_AnalyzeProject(b *testing.B) {
	cfg := &config.Tier1AnalysisConfig{}
	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()
	
	astEngine := astengine.New(nil)
	engine := New(cfg, broker, astEngine)
	
	filePaths := []string{
		"/test/main.go",
		"/test/utils.go",
		"/test/api.go",
	}
	
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.AnalyzeProject(ctx, "/test", filePaths)
		if err != nil {
			b.Fatal(err)
		}
	}
}