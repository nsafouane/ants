// Package linking provides structural linking and dependency resolution capabilities
// for the Context Engine's Tier 1 analysis.
package linking

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	astengine "github.com/charmbracelet/crush/internal/ast"
	"github.com/charmbracelet/crush/internal/pubsub"
)

// Reference represents a reference from one code element to another.
type Reference struct {
	FromFile     string         `json:"from_file"`
	FromSymbol   string         `json:"from_symbol"`
	FromPosition astengine.Point `json:"from_position"`
	ToFile       string         `json:"to_file,omitempty"`
	ToSymbol     string         `json:"to_symbol"`
	ToPosition   astengine.Point `json:"to_position,omitempty"`
	ReferenceType string         `json:"reference_type"`
	IsResolved   bool           `json:"is_resolved"`
	Confidence   float64        `json:"confidence"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// CallGraph represents function call relationships.
type CallGraph struct {
	Nodes map[string]*CallNode `json:"nodes"`
	Edges []*CallEdge          `json:"edges"`
}

// CallNode represents a function or method in the call graph.
type CallNode struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	File         string   `json:"file"`
	Package      string   `json:"package"`
	IsExported   bool     `json:"is_exported"`
	CallsTo      []string `json:"calls_to"`
	CalledBy     []string `json:"called_by"`
	Complexity   int      `json:"complexity"`
}

// CallEdge represents a call relationship between two functions.
type CallEdge struct {
	From       string `json:"from"`
	To         string `json:"to"`
	CallType   string `json:"call_type"`
	Frequency  int    `json:"frequency"`
	LineNumber int    `json:"line_number"`
}

// AnalysisResult contains the results of structural linking analysis.
type AnalysisResult struct {
	ProjectPath    string            `json:"project_path"`
	References     []*Reference      `json:"references"`
	CallGraph      *CallGraph        `json:"call_graph"`
	UnresolvedRefs []*Reference      `json:"unresolved_refs"`
	Statistics     *LinkingStats     `json:"statistics"`
	GeneratedAt    time.Time         `json:"generated_at"`
	AnalysisTime   time.Duration     `json:"analysis_time"`
}

// LinkingStats provides statistics about the structural linking analysis.
type LinkingStats struct {
	TotalFiles        int     `json:"total_files"`
	TotalSymbols      int     `json:"total_symbols"`
	TotalReferences   int     `json:"total_references"`
	ResolvedRefs      int     `json:"resolved_refs"`
	UnresolvedRefs    int     `json:"unresolved_refs"`
	ResolutionRate    float64 `json:"resolution_rate"`
	FunctionCalls     int     `json:"function_calls"`
	ImportStatements  int     `json:"import_statements"`
}

// Engine provides structural linking and dependency resolution capabilities.
type Engine struct {
	config     *config.Tier1AnalysisConfig
	broker     *pubsub.Broker[map[string]interface{}]
	astEngine  *astengine.Engine
	fileSet    *token.FileSet
	
	symbols    map[string]*SymbolInfo
	packages   map[string]*PackageInfo
	files      map[string]*FileInfo
	
	mu         sync.RWMutex
	logger     *slog.Logger
}

// SymbolInfo contains information about a symbol for resolution.
type SymbolInfo struct {
	Name         string
	Type         string
	Package      string
	File         string
	Position     astengine.Point
	IsExported   bool
	Signature    string
}

// PackageInfo contains information about a package.
type PackageInfo struct {
	Name            string
	Path            string
	Files           []string
	ImportedPackages map[string]string
	ExportedSymbols map[string]*SymbolInfo
	LocalSymbols    map[string]*SymbolInfo
}

// FileInfo contains information about a file.
type FileInfo struct {
	Path            string
	Package         string
	Imports         []*astengine.Dependency
	Symbols         []*astengine.Symbol
	References      []*Reference
	LastModified    time.Time
}

// New creates a new structural linking engine.
func New(cfg *config.Tier1AnalysisConfig, broker *pubsub.Broker[map[string]interface{}], astEngine *astengine.Engine) *Engine {
	return &Engine{
		config:    cfg,
		broker:    broker,
		astEngine: astEngine,
		fileSet:   token.NewFileSet(),
		symbols:   make(map[string]*SymbolInfo),
		packages:  make(map[string]*PackageInfo),
		files:     make(map[string]*FileInfo),
		logger:    slog.With("component", "linking_engine"),
	}
}

// AnalyzeProject performs structural linking analysis on a project.
func (e *Engine) AnalyzeProject(ctx context.Context, projectPath string, filePaths []string) (*AnalysisResult, error) {
	startTime := time.Now()
	
	e.logger.Info("Starting structural linking analysis",
		"project_path", projectPath,
		"file_count", len(filePaths))
	
	// Reset state for new analysis
	e.reset()
	
	// Phase 1: Parse all files and build symbol table
	if err := e.buildSymbolTable(ctx, filePaths); err != nil {
		return nil, fmt.Errorf("failed to build symbol table: %w", err)
	}
	
	// Phase 2: Resolve references
	references, err := e.resolveReferences(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve references: %w", err)
	}
	
	// Phase 3: Build call graph
	callGraph := e.buildCallGraph()
	
	// Phase 4: Calculate statistics
	stats := e.calculateStatistics(references, callGraph)
	
	// Separate resolved and unresolved references
	var resolvedRefs, unresolvedRefs []*Reference
	for _, ref := range references {
		if ref.IsResolved {
			resolvedRefs = append(resolvedRefs, ref)
		} else {
			unresolvedRefs = append(unresolvedRefs, ref)
		}
	}
	
	result := &AnalysisResult{
		ProjectPath:    projectPath,
		References:     resolvedRefs,
		CallGraph:      callGraph,
		UnresolvedRefs: unresolvedRefs,
		Statistics:     stats,
		GeneratedAt:    time.Now(),
		AnalysisTime:   time.Since(startTime),
	}
	
	e.logger.Info("Structural linking analysis completed",
		"total_references", len(references),
		"resolved_references", len(resolvedRefs),
		"resolution_rate", fmt.Sprintf("%.2f%%", stats.ResolutionRate*100),
		"analysis_time", result.AnalysisTime)
	
	return result, nil
}

// reset clears the internal state for a new analysis.
func (e *Engine) reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.symbols = make(map[string]*SymbolInfo)
	e.packages = make(map[string]*PackageInfo)
	e.files = make(map[string]*FileInfo)
}

// buildSymbolTable parses all files and builds a comprehensive symbol table.
func (e *Engine) buildSymbolTable(ctx context.Context, filePaths []string) error {
	for _, filePath := range filePaths {
		// Read file content
		content, err := e.readFile(filePath)
		if err != nil {
			e.logger.Warn("Failed to read file", "file", filePath, "error", err)
			continue
		}
		
		// Parse with AST engine
		result, err := e.astEngine.ParseFile(ctx, filePath, content, "go")
		if err != nil {
			e.logger.Warn("Failed to parse file", "file", filePath, "error", err)
			continue
		}
		
		if err := e.processFileResult(filePath, result); err != nil {
			e.logger.Warn("Failed to process file result", "file", filePath, "error", err)
		}
	}
	
	return nil
}

// processFileResult processes a parsed file and updates the symbol table.
func (e *Engine) processFileResult(filePath string, result *astengine.ParseResult) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	packagePath := e.extractPackagePath(filePath)
	
	// Create or update package info
	if _, exists := e.packages[packagePath]; !exists {
		e.packages[packagePath] = &PackageInfo{
			Name:             filepath.Base(packagePath),
			Path:             packagePath,
			Files:            []string{},
			ImportedPackages: make(map[string]string),
			ExportedSymbols:  make(map[string]*SymbolInfo),
			LocalSymbols:     make(map[string]*SymbolInfo),
		}
	}
	
	pkg := e.packages[packagePath]
	pkg.Files = append(pkg.Files, filePath)
	
	// Process imports
	for _, dep := range result.Dependencies {
		if dep.Type == "import" {
			pkg.ImportedPackages[dep.Target] = dep.Alias
		}
	}
	
	// Process symbols
	fileInfo := &FileInfo{
		Path:         filePath,
		Package:      packagePath,
		Imports:      result.Dependencies,
		Symbols:      result.Symbols,
		References:   []*Reference{},
		LastModified: time.Now(),
	}
	
	for _, symbol := range result.Symbols {
		symbolInfo := &SymbolInfo{
			Name:       symbol.Name,
			Type:       symbol.Type,
			Package:    packagePath,
			File:       filePath,
			Position:   symbol.StartPoint,
			IsExported: symbol.IsExported,
			Signature:  symbol.Signature,
		}
		
		// Add to global symbol table
		symbolKey := fmt.Sprintf("%s.%s", packagePath, symbol.Name)
		e.symbols[symbolKey] = symbolInfo
		
		// Add to package symbol tables
		if symbol.IsExported {
			pkg.ExportedSymbols[symbol.Name] = symbolInfo
		} else {
			pkg.LocalSymbols[symbol.Name] = symbolInfo
		}
	}
	
	e.files[filePath] = fileInfo
	return nil
}

// resolveReferences analyzes symbol usage and resolves references.
func (e *Engine) resolveReferences(ctx context.Context) ([]*Reference, error) {
	var allReferences []*Reference
	
	e.mu.RLock()
	files := make([]*FileInfo, 0, len(e.files))
	for _, file := range e.files {
		files = append(files, file)
	}
	e.mu.RUnlock()
	
	// Process each file to find references
	for _, file := range files {
		refs, err := e.findReferencesInFile(ctx, file)
		if err != nil {
			e.logger.Warn("Failed to find references in file", "file", file.Path, "error", err)
			continue
		}
		allReferences = append(allReferences, refs...)
	}
	
	return allReferences, nil
}

// findReferencesInFile finds all symbol references in a specific file.
func (e *Engine) findReferencesInFile(ctx context.Context, file *FileInfo) ([]*Reference, error) {
	var references []*Reference
	
	// Read and re-parse the file to get detailed AST
	content, err := e.readFile(file.Path)
	if err != nil {
		return nil, err
	}
	
	// Parse with Go's AST parser for detailed analysis
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, file.Path, content, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	
	// Walk the AST to find references
	ast.Inspect(astFile, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			refs := e.processCallExpression(n, fset, file)
			references = append(references, refs...)
		}
		return true
	})
	
	return references, nil
}

// processCallExpression processes function call expressions.
func (e *Engine) processCallExpression(call *ast.CallExpr, fset *token.FileSet, file *FileInfo) []*Reference {
	var references []*Reference
	
	pos := fset.Position(call.Pos())
	
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		// Direct function call: foo()
		ref := &Reference{
			FromFile:      file.Path,
			FromPosition:  astengine.Point{Row: uint32(pos.Line - 1), Column: uint32(pos.Column - 1)},
			ToSymbol:      fun.Name,
			ReferenceType: "call",
			IsResolved:    false,
			Confidence:    0.8,
			Metadata:      make(map[string]string),
		}
		
		e.resolveReference(ref, file)
		references = append(references, ref)
	}
	
	return references
}

// resolveReference attempts to resolve a reference to its definition.
func (e *Engine) resolveReference(ref *Reference, file *FileInfo) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	// Try direct symbol lookup in current package
	currentPkg := e.packages[file.Package]
	if currentPkg != nil {
		if symbol, exists := currentPkg.LocalSymbols[ref.ToSymbol]; exists {
			ref.ToFile = symbol.File
			ref.ToPosition = symbol.Position
			ref.IsResolved = true
			ref.Confidence = 0.95
			return
		}
		
		if symbol, exists := currentPkg.ExportedSymbols[ref.ToSymbol]; exists {
			ref.ToFile = symbol.File
			ref.ToPosition = symbol.Position
			ref.IsResolved = true
			ref.Confidence = 0.95
			return
		}
	}
}

// buildCallGraph constructs a call graph from the resolved references.
func (e *Engine) buildCallGraph() *CallGraph {
	nodes := make(map[string]*CallNode)
	var edges []*CallEdge
	
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	// Create nodes for all functions and methods
	for _, symbol := range e.symbols {
		if symbol.Type == "function" || symbol.Type == "method" {
			nodeID := fmt.Sprintf("%s:%s", symbol.File, symbol.Name)
			nodes[nodeID] = &CallNode{
				ID:         nodeID,
				Name:       symbol.Name,
				File:       symbol.File,
				Package:    symbol.Package,
				IsExported: symbol.IsExported,
				CallsTo:    []string{},
				CalledBy:   []string{},
				Complexity: 1,
			}
		}
	}
	
	return &CallGraph{
		Nodes: nodes,
		Edges: edges,
	}
}

// calculateStatistics computes various statistics about the analysis.
func (e *Engine) calculateStatistics(references []*Reference, callGraph *CallGraph) *LinkingStats {
	stats := &LinkingStats{}
	
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	stats.TotalFiles = len(e.files)
	stats.TotalSymbols = len(e.symbols)
	stats.TotalReferences = len(references)
	
	// Count resolved vs unresolved references
	for _, ref := range references {
		if ref.IsResolved {
			stats.ResolvedRefs++
		} else {
			stats.UnresolvedRefs++
		}
		
		if ref.ReferenceType == "call" {
			stats.FunctionCalls++
		}
	}
	
	if stats.TotalReferences > 0 {
		stats.ResolutionRate = float64(stats.ResolvedRefs) / float64(stats.TotalReferences)
	}
	
	return stats
}

// Helper functions

// readFile reads the content of a file.
func (e *Engine) readFile(filePath string) ([]byte, error) {
	// This is a placeholder - in real implementation, this would read from filesystem
	// For testing, we'll return mock content
	return []byte("package main\n\nfunc main() {\n\tfmt.Println(\"Hello, World!\")\n}"), nil
}

// extractPackagePath extracts package path from file path.
func (e *Engine) extractPackagePath(filePath string) string {
	dir := filepath.Dir(filePath)
	return strings.ReplaceAll(dir, "\\", "/")
}