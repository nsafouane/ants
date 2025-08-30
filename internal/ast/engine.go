// Package ast provides AST parsing and analysis capabilities
// for the Context Engine's Tier 1 structural analysis.
package ast

import (
	"context"
	"crypto/md5"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/config"
)

// Node represents a parsed AST node with metadata.
type Node struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Text         string            `json:"text,omitempty"`
	StartByte    uint32            `json:"start_byte"`
	EndByte      uint32            `json:"end_byte"`
	StartPoint   Point             `json:"start_point"`
	EndPoint     Point             `json:"end_point"`
	Children     []*Node           `json:"children,omitempty"`
	Parent       *Node             `json:"-"` // Avoid circular references in JSON
	Metadata     map[string]string `json:"metadata,omitempty"`
	Language     string            `json:"language"`
	IsNamed      bool              `json:"is_named"`
	IsMissing    bool              `json:"is_missing"`
	HasError     bool              `json:"has_error"`
	SymbolType   string            `json:"symbol_type,omitempty"`   // function, class, variable, etc.
	Visibility   string            `json:"visibility,omitempty"`   // public, private, protected
	IsExported   bool              `json:"is_exported,omitempty"`  // For languages with export concepts
}

// Point represents a position in the source code.
type Point struct {
	Row    uint32 `json:"row"`
	Column uint32 `json:"column"`
}

// ParseResult contains the result of parsing a source file.
type ParseResult struct {
	FilePath     string            `json:"file_path"`
	Language     string            `json:"language"`
	Root         *Node             `json:"root"`
	Symbols      []*Symbol         `json:"symbols"`
	Dependencies []*Dependency     `json:"dependencies"`
	Errors       []ParseError      `json:"errors,omitempty"`
	ParseTime    time.Duration     `json:"parse_time"`
	ContentHash  string            `json:"content_hash"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Symbol represents a high-level symbol found in the code.
type Symbol struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`         // function, class, interface, variable, constant
	Kind         string            `json:"kind"`         // method, field, constructor, etc.
	StartByte    uint32            `json:"start_byte"`
	EndByte      uint32            `json:"end_byte"`
	StartPoint   Point             `json:"start_point"`
	EndPoint     Point             `json:"end_point"`
	Signature    string            `json:"signature,omitempty"`
	DocComment   string            `json:"doc_comment,omitempty"`
	Visibility   string            `json:"visibility,omitempty"`
	IsExported   bool              `json:"is_exported"`
	Package      string            `json:"package,omitempty"`
	Module       string            `json:"module,omitempty"`
	Parameters   []Parameter       `json:"parameters,omitempty"`
	ReturnType   string            `json:"return_type,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Parameter represents a function or method parameter.
type Parameter struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Dependency represents a code dependency (import, include, etc.).
type Dependency struct {
	Type     string `json:"type"`      // import, include, require, etc.
	Target   string `json:"target"`    // module/package name
	Alias    string `json:"alias,omitempty"`
	IsLocal  bool   `json:"is_local"`  // local vs external dependency
	Position Point  `json:"position"`
}

// ParseError represents an error that occurred during parsing.
type ParseError struct {
	Message   string `json:"message"`
	Position  Point  `json:"position"`
	Severity  string `json:"severity"` // error, warning, info
	Code      string `json:"code,omitempty"`
}

// CacheEntry represents a cached parse result.
type CacheEntry struct {
	Result    *ParseResult
	Timestamp time.Time
	Hash      string
}

// Engine provides AST parsing and analysis capabilities.
type Engine struct {
	config       *config.ASTProcessingConfig
	fileSet      *token.FileSet
	cache        map[string]*CacheEntry
	cacheMutex   sync.RWMutex
	maxCacheSize int64
	currentSize  int64
	logger       *slog.Logger
}

// New creates a new AST processing engine.
func New(cfg *config.ASTProcessingConfig) *Engine {
	if cfg == nil {
		cfg = &config.ASTProcessingConfig{
			EnableCaching:   true,
			CacheTTLSeconds: 3600,
			MaxCacheSizeMB:  128,
			SupportedLanguages: []string{"go", "python", "javascript", "typescript"},
		}
	}

	engine := &Engine{
		config:       cfg,
		fileSet:      token.NewFileSet(),
		cache:        make(map[string]*CacheEntry),
		maxCacheSize: int64(cfg.MaxCacheSizeMB) * 1024 * 1024,
		logger:       slog.With("component", "ast_engine"),
	}

	return engine
}

// isLanguageSupported checks if a language is in the supported list.
func (e *Engine) isLanguageSupported(language string) bool {
	if len(e.config.SupportedLanguages) == 0 {
		return true // Support all if none specified
	}
	
	for _, supported := range e.config.SupportedLanguages {
		if supported == language {
			return true
		}
	}
	return false
}

// ParseFile parses a source file and returns the AST structure.
func (e *Engine) ParseFile(ctx context.Context, filePath string, content []byte, language string) (*ParseResult, error) {
	startTime := time.Now()
	
	// Calculate content hash for caching
	contentHash := fmt.Sprintf("%x", md5.Sum(content))
	
	// Check cache if enabled
	if e.config.EnableCaching {
		if cached := e.getCachedResult(filePath, contentHash); cached != nil {
			e.logger.Debug("Using cached parse result", "file", filePath)
			return cached, nil
		}
	}
	
	// Get parser for the language
	if language != "go" {
		return nil, fmt.Errorf("unsupported language: %s (only Go is currently supported)", language)
	}
	
	// Parse using Go's built-in parser
	file, err := parser.ParseFile(e.fileSet, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file %s: %w", filePath, err)
	}
	
	// Convert to our Node structure
	root := e.convertGoASTNode(file, content, nil)
	
	// Extract symbols and dependencies
	symbols := e.extractGoSymbolsFromAST(file, content)
	dependencies := e.extractGoDependenciesFromAST(file, content)
	errors := []ParseError{} // Go parser errors are handled by the err return
	
	result := &ParseResult{
		FilePath:     filePath,
		Language:     language,
		Root:         root,
		Symbols:      symbols,
		Dependencies: dependencies,
		Errors:       errors,
		ParseTime:    time.Since(startTime),
		ContentHash:  contentHash,
		Metadata:     make(map[string]string),
	}
	
	// Cache the result if enabled
	if e.config.EnableCaching {
		e.cacheResult(filePath, contentHash, result)
	}
	
	e.logger.Debug("Parsed file successfully", 
		"file", filePath,
		"language", language,
		"symbols", len(symbols),
		"dependencies", len(dependencies),
		"parse_time", result.ParseTime)
	
	return result, nil
}

// convertGoASTNode converts a Go AST node to our Node structure.
func (e *Engine) convertGoASTNode(node ast.Node, content []byte, parent *Node) *Node {
	if node == nil {
		return nil
	}
	
	pos := e.fileSet.Position(node.Pos())
	end := e.fileSet.Position(node.End())
	
	nodeText := ""
	if int(node.Pos()) < len(content) && int(node.End()) <= len(content) {
		nodeText = string(content[node.Pos()-1 : node.End()-1])
	}
	
	astNode := &Node{
		ID:         fmt.Sprintf("%T_%d_%d", node, node.Pos(), node.End()),
		Type:       fmt.Sprintf("%T", node),
		Text:       nodeText,
		StartByte:  uint32(node.Pos() - 1),
		EndByte:    uint32(node.End() - 1),
		StartPoint: Point{Row: uint32(pos.Line - 1), Column: uint32(pos.Column - 1)},
		EndPoint:   Point{Row: uint32(end.Line - 1), Column: uint32(end.Column - 1)},
		Parent:     parent,
		Metadata:   make(map[string]string),
		Language:   "go",
		IsNamed:    true,
		IsMissing:  false,
		HasError:   false,
	}
	
	return astNode
}

// extractGoSymbolsFromAST extracts symbols from Go AST.
func (e *Engine) extractGoSymbolsFromAST(file *ast.File, content []byte) []*Symbol {
	var symbols []*Symbol
	
	// Walk the AST and extract symbols
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncDecl:
			symbol := e.extractGoFunctionFromAST(n, content)
			if symbol != nil {
				symbols = append(symbols, symbol)
			}
		case *ast.GenDecl:
			// Handle type, var, const declarations
			genSymbols := e.extractGoGenDeclFromAST(n, content)
			symbols = append(symbols, genSymbols...)
		}
		return true
	})
	
	return symbols
}

// extractGoFunctionFromAST extracts a Go function symbol from AST.
func (e *Engine) extractGoFunctionFromAST(fn *ast.FuncDecl, content []byte) *Symbol {
	if fn.Name == nil {
		return nil
	}
	
	pos := e.fileSet.Position(fn.Pos())
	end := e.fileSet.Position(fn.End())
	
	symbol := &Symbol{
		Name:       fn.Name.Name,
		Type:       "function",
		Kind:       "function",
		StartByte:  uint32(fn.Pos() - 1),
		EndByte:    uint32(fn.End() - 1),
		StartPoint: Point{Row: uint32(pos.Line - 1), Column: uint32(pos.Column - 1)},
		EndPoint:   Point{Row: uint32(end.Line - 1), Column: uint32(end.Column - 1)},
		IsExported: ast.IsExported(fn.Name.Name),
		Parameters: e.extractGoParametersFromAST(fn.Type),
		Metadata:   make(map[string]string),
	}
	
	// Check if it's a method
	if fn.Recv != nil {
		symbol.Kind = "method"
	}
	
	// Extract signature
	if int(fn.Pos()) < len(content) && int(fn.End()) <= len(content) {
		symbol.Signature = string(content[fn.Pos()-1 : fn.End()-1])
	}
	
	// Extract doc comment
	if fn.Doc != nil {
		symbol.DocComment = fn.Doc.Text()
	}
	
	return symbol
}

// extractGoGenDeclFromAST extracts symbols from general declarations (type, var, const).
func (e *Engine) extractGoGenDeclFromAST(gen *ast.GenDecl, content []byte) []*Symbol {
	var symbols []*Symbol
	
	for _, spec := range gen.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			symbol := e.extractGoTypeSpecFromAST(s, content)
			if symbol != nil {
				symbols = append(symbols, symbol)
			}
		case *ast.ValueSpec:
			varSymbols := e.extractGoValueSpecFromAST(s, gen.Tok, content)
			symbols = append(symbols, varSymbols...)
		}
	}
	
	return symbols
}

// extractGoTypeSpecFromAST extracts a Go type specification from AST.
func (e *Engine) extractGoTypeSpecFromAST(spec *ast.TypeSpec, content []byte) *Symbol {
	if spec.Name == nil {
		return nil
	}
	
	pos := e.fileSet.Position(spec.Pos())
	end := e.fileSet.Position(spec.End())
	
	typeKind := "type"
	switch spec.Type.(type) {
	case *ast.StructType:
		typeKind = "struct"
	case *ast.InterfaceType:
		typeKind = "interface"
	case *ast.MapType:
		typeKind = "map"
	case *ast.ChanType:
		typeKind = "channel"
	case *ast.FuncType:
		typeKind = "function_type"
	}
	
	return &Symbol{
		Name:       spec.Name.Name,
		Type:       "type",
		Kind:       typeKind,
		StartByte:  uint32(spec.Pos() - 1),
		EndByte:    uint32(spec.End() - 1),
		StartPoint: Point{Row: uint32(pos.Line - 1), Column: uint32(pos.Column - 1)},
		EndPoint:   Point{Row: uint32(end.Line - 1), Column: uint32(end.Column - 1)},
		IsExported: ast.IsExported(spec.Name.Name),
		Metadata:   make(map[string]string),
	}
}

// extractGoValueSpecFromAST extracts Go variable/constant symbols from AST.
func (e *Engine) extractGoValueSpecFromAST(spec *ast.ValueSpec, tok token.Token, content []byte) []*Symbol {
	var symbols []*Symbol
	
	symbolType := "variable"
	if tok == token.CONST {
		symbolType = "constant"
	}
	
	for _, name := range spec.Names {
		if name == nil {
			continue
		}
		
		pos := e.fileSet.Position(name.Pos())
		end := e.fileSet.Position(name.End())
		
		symbols = append(symbols, &Symbol{
			Name:       name.Name,
			Type:       symbolType,
			Kind:       symbolType,
			StartByte:  uint32(name.Pos() - 1),
			EndByte:    uint32(name.End() - 1),
			StartPoint: Point{Row: uint32(pos.Line - 1), Column: uint32(pos.Column - 1)},
			EndPoint:   Point{Row: uint32(end.Line - 1), Column: uint32(end.Column - 1)},
			IsExported: ast.IsExported(name.Name),
			Metadata:   make(map[string]string),
		})
	}
	
	return symbols
}

// extractGoParametersFromAST extracts function parameters from AST.
func (e *Engine) extractGoParametersFromAST(fnType *ast.FuncType) []Parameter {
	var params []Parameter
	
	if fnType == nil || fnType.Params == nil {
		return params
	}
	
	for _, param := range fnType.Params.List {
		var paramType string
		if param.Type != nil {
			paramType = e.typeToString(param.Type)
		}
		
		if len(param.Names) == 0 {
			// Anonymous parameter
			params = append(params, Parameter{
				Name: "",
				Type: paramType,
			})
		} else {
			// Named parameters
			for _, name := range param.Names {
				params = append(params, Parameter{
					Name: name.Name,
					Type: paramType,
				})
			}
		}
	}
	
	return params
}

// typeToString converts an AST type to a string representation.
func (e *Engine) typeToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + e.typeToString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + e.typeToString(t.Elt)
		}
		return "[...]" + e.typeToString(t.Elt)
	case *ast.MapType:
		return "map[" + e.typeToString(t.Key) + "]" + e.typeToString(t.Value)
	case *ast.ChanType:
		return "chan " + e.typeToString(t.Value)
	case *ast.FuncType:
		return "func"
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{}"
	case *ast.SelectorExpr:
		return e.typeToString(t.X) + "." + t.Sel.Name
	default:
		return "unknown"
	}
}

// extractGoDependenciesFromAST extracts Go import statements from AST.
func (e *Engine) extractGoDependenciesFromAST(file *ast.File, content []byte) []*Dependency {
	var dependencies []*Dependency
	
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		
		pos := e.fileSet.Position(imp.Pos())
		
		// Remove quotes from import path
		target := imp.Path.Value
		if len(target) >= 2 && target[0] == '"' && target[len(target)-1] == '"' {
			target = target[1 : len(target)-1]
		}
		
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		
		dependencies = append(dependencies, &Dependency{
			Type:     "import",
			Target:   target,
			Alias:    alias,
			IsLocal:  isLocalImport(target),
			Position: Point{Row: uint32(pos.Line - 1), Column: uint32(pos.Column - 1)},
		})
	}
	
	return dependencies
}



// getCachedResult retrieves a cached parse result.
func (e *Engine) getCachedResult(filePath, contentHash string) *ParseResult {
	e.cacheMutex.RLock()
	defer e.cacheMutex.RUnlock()
	
	entry, exists := e.cache[filePath]
	if !exists || entry.Hash != contentHash {
		return nil
	}
	
	// Check TTL
	if time.Since(entry.Timestamp) > time.Duration(e.config.CacheTTLSeconds)*time.Second {
		return nil
	}
	
	return entry.Result
}

// cacheResult stores a parse result in the cache.
func (e *Engine) cacheResult(filePath, contentHash string, result *ParseResult) {
	e.cacheMutex.Lock()
	defer e.cacheMutex.Unlock()
	
	// Simple size estimation (not exact but good enough)
	estimatedSize := int64(len(result.FilePath) + len(result.ContentHash) + 1024) // Base overhead
	
	// Remove old entry if exists
	if _, exists := e.cache[filePath]; exists {
		e.currentSize -= estimatedSize // Rough estimation
		delete(e.cache, filePath)
	}
	
	// Check cache size limit
	if e.currentSize+estimatedSize > e.maxCacheSize {
		e.evictLRU()
	}
	
	e.cache[filePath] = &CacheEntry{
		Result:    result,
		Timestamp: time.Now(),
		Hash:      contentHash,
	}
	e.currentSize += estimatedSize
}

// evictLRU evicts least recently used cache entries.
func (e *Engine) evictLRU() {
	// Simple LRU implementation - remove oldest entries
	oldestTime := time.Now()
	var oldestKey string
	
	for key, entry := range e.cache {
		if entry.Timestamp.Before(oldestTime) {
			oldestTime = entry.Timestamp
			oldestKey = key
		}
	}
	
	if oldestKey != "" {
		delete(e.cache, oldestKey)
		e.currentSize -= 1024 // Rough estimation
	}
}

// ClearCache clears the entire cache.
func (e *Engine) ClearCache() {
	e.cacheMutex.Lock()
	defer e.cacheMutex.Unlock()
	
	e.cache = make(map[string]*CacheEntry)
	e.currentSize = 0
}

// GetCacheStats returns cache statistics.
func (e *Engine) GetCacheStats() map[string]interface{} {
	e.cacheMutex.RLock()
	defer e.cacheMutex.RUnlock()
	
	return map[string]interface{}{
		"entries":     len(e.cache),
		"size_bytes":  e.currentSize,
		"max_size":    e.maxCacheSize,
		"utilization": float64(e.currentSize) / float64(e.maxCacheSize),
	}
}

// Utility functions



// isLocalImport checks if an import path is local.
func isLocalImport(importPath string) bool {
	if len(importPath) == 0 {
		return false
	}
	
	// Relative imports (start with . or ..)
	if importPath[0] == '.' {
		return true
	}
	
	// Absolute paths
	if importPath[0] == '/' {
		return true
	}
	
	// Standard library packages (no dots, single word)
	// Common standard library packages
	standardLibs := map[string]bool{
		"fmt": true, "os": true, "io": true, "net": true, "http": true,
		"time": true, "context": true, "sync": true, "strings": true,
		"strconv": true, "regexp": true, "encoding": true, "crypto": true,
		"database": true, "log": true, "math": true, "sort": true,
		"bytes": true, "bufio": true, "path": true, "filepath": true,
		"runtime": true, "reflect": true, "errors": true, "flag": true,
		"json": true, "xml": true, "html": true, "url": true, "unicode": true,
	}
	
	// Check if it's a known standard library package
	if standardLibs[importPath] {
		return false
	}
	
	// If it contains a dot, it's likely an external package
	if containsDot(importPath) {
		return false
	}
	
	// Single word packages that are not standard library are considered local
	return true
}

// containsDot checks if a string contains a dot.
func containsDot(s string) bool {
	for _, r := range s {
		if r == '.' {
			return true
		}
	}
	return false
}