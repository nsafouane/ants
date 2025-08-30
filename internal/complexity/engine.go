// Package complexity provides code complexity analysis capabilities
// for the Context Engine's Tier 1 structural analysis.
package complexity

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/config"
)

// MetricType represents different complexity metrics.
type MetricType string

const (
	MetricCyclomatic MetricType = "cyclomatic"
	MetricCognitive  MetricType = "cognitive"
	MetricHalstead   MetricType = "halstead"
)

// ComplexityMetrics holds various complexity measurements for a code element.
type ComplexityMetrics struct {
	// Basic metrics
	CyclomaticComplexity int `json:"cyclomatic_complexity"`
	CognitiveComplexity  int `json:"cognitive_complexity"`
	LinesOfCode          int `json:"lines_of_code"`
	LogicalLinesOfCode   int `json:"logical_lines_of_code"`

	// Halstead metrics
	HalsteadDifficulty float64 `json:"halstead_difficulty"`
	HalsteadVolume     float64 `json:"halstead_volume"`
	HalsteadEffort     float64 `json:"halstead_effort"`

	// Structural metrics
	NestingDepth int `json:"nesting_depth"`
	FanOut       int `json:"fan_out"`
	FanIn        int `json:"fan_in"`

	// Quality indicators
	HasDocumentation bool `json:"has_documentation"`
	HasTests         bool `json:"has_tests"`
	TodoCount        int  `json:"todo_count"`
	HackCount        int  `json:"hack_count"`

	// Warning flags based on thresholds
	IsComplexityWarning bool `json:"is_complexity_warning"`
	IsCognitiveWarning  bool `json:"is_cognitive_warning"`
	IsLengthWarning     bool `json:"is_length_warning"`
}

// FunctionComplexity represents complexity analysis for a function.
type FunctionComplexity struct {
	Name            string            `json:"name"`
	Package         string            `json:"package"`
	StartLine       int               `json:"start_line"`
	EndLine         int               `json:"end_line"`
	Signature       string            `json:"signature"`
	IsExported      bool              `json:"is_exported"`
	IsMethod        bool              `json:"is_method"`
	ReceiverType    string            `json:"receiver_type,omitempty"`
	Metrics         ComplexityMetrics `json:"metrics"`
	CalculationTime time.Duration     `json:"calculation_time"`
}

// FileComplexity represents complexity analysis for an entire file.
type FileComplexity struct {
	FilePath       string                `json:"file_path"`
	Package        string                `json:"package"`
	Language       string                `json:"language"`
	Functions      []*FunctionComplexity `json:"functions"`
	OverallMetrics ComplexityMetrics     `json:"overall_metrics"`
	AnalysisTime   time.Duration         `json:"analysis_time"`
	Warnings       []string              `json:"warnings"`
}

// HalsteadOperators contains the operands and operators for Halstead metrics.
type HalsteadOperators struct {
	Operators       map[string]int `json:"operators"`        // Unique operators and their counts
	Operands        map[string]int `json:"operands"`         // Unique operands and their counts
	TotalOperators  int            `json:"total_operators"`  // Total operator occurrences
	TotalOperands   int            `json:"total_operands"`   // Total operand occurrences
	UniqueOperators int            `json:"unique_operators"` // Number of unique operators
	UniqueOperands  int            `json:"unique_operands"`  // Number of unique operands
}

// Engine provides code complexity analysis capabilities.
type Engine struct {
	config         *config.ComplexityAnalysisConfig
	enabledMetrics map[MetricType]bool
	thresholds     *config.ComplexityThresholds
	logger         *slog.Logger

	// Statistics
	stats     *AnalysisStats
	statsLock sync.RWMutex
}

// AnalysisStats tracks complexity analysis statistics.
type AnalysisStats struct {
	FunctionsAnalyzed      int64         `json:"functions_analyzed"`
	FilesAnalyzed          int64         `json:"files_analyzed"`
	TotalTime              time.Duration `json:"total_time"`
	AverageTimePerFunction time.Duration `json:"average_time_per_function"`
	HighComplexityCount    int64         `json:"high_complexity_count"`
	WarningsGenerated      int64         `json:"warnings_generated"`
}

// New creates a new complexity analysis engine.
func New(cfg *config.ComplexityAnalysisConfig) *Engine {
	if cfg == nil {
		cfg = &config.ComplexityAnalysisConfig{
			Enabled: true,
			Metrics: []string{"cyclomatic", "cognitive", "halstead"},
			WarningThresholds: &config.ComplexityThresholds{
				CyclomaticComplexity: 10,
				CognitiveComplexity:  15,
				MaxFunctionLength:    50,
			},
		}
	}

	if cfg.WarningThresholds == nil {
		cfg.WarningThresholds = &config.ComplexityThresholds{
			CyclomaticComplexity: 10,
			CognitiveComplexity:  15,
			MaxFunctionLength:    50,
		}
	}

	engine := &Engine{
		config:         cfg,
		enabledMetrics: make(map[MetricType]bool),
		thresholds:     cfg.WarningThresholds,
		logger:         slog.With("component", "complexity_engine"),
		stats:          &AnalysisStats{},
	}

	// Build enabled metrics map
	for _, metric := range cfg.Metrics {
		engine.enabledMetrics[MetricType(metric)] = true
	}

	return engine
}

// AnalyzeAST performs complexity analysis on a parsed AST.
func (e *Engine) AnalyzeAST(ctx context.Context, fileSet *token.FileSet, file *ast.File, filePath string) (*FileComplexity, error) {
	if !e.config.Enabled {
		return nil, fmt.Errorf("complexity analysis is disabled")
	}

	startTime := time.Now()

	e.logger.Debug("Starting complexity analysis", "file", filePath)

	result := &FileComplexity{
		FilePath:  filePath,
		Package:   file.Name.Name,
		Language:  "go",
		Functions: make([]*FunctionComplexity, 0),
		Warnings:  make([]string, 0),
	}

	// Analyze all functions in the file
	for _, decl := range file.Decls {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		switch d := decl.(type) {
		case *ast.FuncDecl:
			funcComplexity, err := e.analyzeFunctionDecl(fileSet, d, file.Name.Name)
			if err != nil {
				e.logger.Warn("Failed to analyze function", "function", d.Name.Name, "error", err)
				continue
			}

			result.Functions = append(result.Functions, funcComplexity)

			// Check for warnings
			if funcComplexity.Metrics.IsComplexityWarning {
				warning := fmt.Sprintf("Function %s has high cyclomatic complexity: %d",
					funcComplexity.Name, funcComplexity.Metrics.CyclomaticComplexity)
				result.Warnings = append(result.Warnings, warning)
			}

			if funcComplexity.Metrics.IsCognitiveWarning {
				warning := fmt.Sprintf("Function %s has high cognitive complexity: %d",
					funcComplexity.Name, funcComplexity.Metrics.CognitiveComplexity)
				result.Warnings = append(result.Warnings, warning)
			}

			if funcComplexity.Metrics.IsLengthWarning {
				warning := fmt.Sprintf("Function %s exceeds maximum length: %d lines",
					funcComplexity.Name, funcComplexity.Metrics.LinesOfCode)
				result.Warnings = append(result.Warnings, warning)
			}
		}
	}

	// Calculate overall file metrics
	result.OverallMetrics = e.calculateFileMetrics(result.Functions)
	result.AnalysisTime = time.Since(startTime)

	// Update statistics
	e.updateStats(result)

	e.logger.Debug("Complexity analysis completed",
		"file", filePath,
		"functions", len(result.Functions),
		"warnings", len(result.Warnings),
		"time", result.AnalysisTime)

	return result, nil
}

// analyzeFunctionDecl analyzes complexity metrics for a single function declaration.
func (e *Engine) analyzeFunctionDecl(fileSet *token.FileSet, decl *ast.FuncDecl, packageName string) (*FunctionComplexity, error) {
	startTime := time.Now()

	if decl.Body == nil {
		// Function declaration without body (interface method)
		return nil, fmt.Errorf("function %s has no body", decl.Name.Name)
	}

	startPos := fileSet.Position(decl.Pos())
	endPos := fileSet.Position(decl.End())

	result := &FunctionComplexity{
		Name:       decl.Name.Name,
		Package:    packageName,
		StartLine:  startPos.Line,
		EndLine:    endPos.Line,
		IsExported: ast.IsExported(decl.Name.Name),
		IsMethod:   decl.Recv != nil,
	}

	// Extract receiver type for methods
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		if field := decl.Recv.List[0]; field.Type != nil {
			result.ReceiverType = e.extractTypeName(field.Type)
		}
	}

	// Build function signature
	result.Signature = e.buildFunctionSignature(decl)

	// Calculate metrics
	result.Metrics = e.calculateFunctionMetrics(fileSet, decl)
	result.CalculationTime = time.Since(startTime)

	return result, nil
}

// calculateFunctionMetrics calculates all enabled metrics for a function.
func (e *Engine) calculateFunctionMetrics(fileSet *token.FileSet, decl *ast.FuncDecl) ComplexityMetrics {
	metrics := ComplexityMetrics{}

	// Basic line counting
	startPos := fileSet.Position(decl.Pos())
	endPos := fileSet.Position(decl.End())
	metrics.LinesOfCode = endPos.Line - startPos.Line + 1

	// Calculate cyclomatic complexity
	if e.enabledMetrics[MetricCyclomatic] {
		metrics.CyclomaticComplexity = e.calculateCyclomaticComplexity(decl.Body)
	}

	// Calculate cognitive complexity
	if e.enabledMetrics[MetricCognitive] {
		metrics.CognitiveComplexity = e.calculateCognitiveComplexity(decl.Body, 0)
	}

	// Calculate Halstead metrics
	if e.enabledMetrics[MetricHalstead] {
		halstead := e.calculateHalsteadMetrics(decl.Body)
		metrics.HalsteadDifficulty = halstead.Difficulty
		metrics.HalsteadVolume = halstead.Volume
		metrics.HalsteadEffort = halstead.Effort
	}

	// Calculate nesting depth
	metrics.NestingDepth = e.calculateNestingDepth(decl.Body)

	// Calculate logical lines of code (non-comment, non-blank lines)
	metrics.LogicalLinesOfCode = e.calculateLogicalLinesOfCode(decl.Body)

	// Check for documentation
	metrics.HasDocumentation = decl.Doc != nil && len(decl.Doc.List) > 0

	// Count TODOs and HACKs in comments
	if decl.Doc != nil {
		for _, comment := range decl.Doc.List {
			text := strings.ToUpper(comment.Text)
			metrics.TodoCount += strings.Count(text, "TODO")
			metrics.HackCount += strings.Count(text, "HACK")
		}
	}

	// Set warning flags based on thresholds
	metrics.IsComplexityWarning = metrics.CyclomaticComplexity > e.thresholds.CyclomaticComplexity
	metrics.IsCognitiveWarning = metrics.CognitiveComplexity > e.thresholds.CognitiveComplexity
	metrics.IsLengthWarning = metrics.LinesOfCode > e.thresholds.MaxFunctionLength

	return metrics
}

// calculateCyclomaticComplexity calculates the cyclomatic complexity of a function body.
func (e *Engine) calculateCyclomaticComplexity(body *ast.BlockStmt) int {
	complexity := 1 // Base complexity

	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			complexity++
		case *ast.ForStmt:
			complexity++
		case *ast.RangeStmt:
			complexity++
		case *ast.SwitchStmt:
			complexity++
		case *ast.TypeSwitchStmt:
			complexity++
		case *ast.CaseClause:
			// Each case adds complexity (except default)
			if node.List != nil {
				complexity++
			}
		case *ast.CommClause:
			// Each case in select statement
			if node.Comm != nil {
				complexity++
			}
		case *ast.FuncLit:
			// Function literals add complexity
			complexity++
		}
		return true
	})

	return complexity
}

// calculateCognitiveComplexity calculates cognitive complexity with nesting penalties.
func (e *Engine) calculateCognitiveComplexity(body *ast.BlockStmt, nestingLevel int) int {
	complexity := 0

	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			complexity += 1 + nestingLevel
			if node.Body != nil {
				complexity += e.calculateCognitiveComplexity(node.Body, nestingLevel+1)
			}
			if node.Else != nil {
				if elseIf, ok := node.Else.(*ast.IfStmt); ok {
					complexity += e.calculateCognitiveComplexityNode(elseIf, nestingLevel)
				} else if elseBlock, ok := node.Else.(*ast.BlockStmt); ok {
					complexity += e.calculateCognitiveComplexity(elseBlock, nestingLevel+1)
				}
			}
			return false // Don't traverse children as we handle them manually

		case *ast.ForStmt:
			complexity += 1 + nestingLevel
			if node.Body != nil {
				complexity += e.calculateCognitiveComplexity(node.Body, nestingLevel+1)
			}
			return false

		case *ast.RangeStmt:
			complexity += 1 + nestingLevel
			if node.Body != nil {
				complexity += e.calculateCognitiveComplexity(node.Body, nestingLevel+1)
			}
			return false

		case *ast.SwitchStmt:
			complexity += 1 + nestingLevel
			if node.Body != nil {
				complexity += e.calculateCognitiveComplexity(node.Body, nestingLevel+1)
			}
			return false

		case *ast.TypeSwitchStmt:
			complexity += 1 + nestingLevel
			if node.Body != nil {
				complexity += e.calculateCognitiveComplexity(node.Body, nestingLevel+1)
			}
			return false
		}
		return true
	})

	return complexity
}

// calculateCognitiveComplexityNode calculates cognitive complexity for a single node.
func (e *Engine) calculateCognitiveComplexityNode(node ast.Node, nestingLevel int) int {
	switch n := node.(type) {
	case *ast.IfStmt:
		complexity := 1 + nestingLevel
		if n.Body != nil {
			complexity += e.calculateCognitiveComplexity(n.Body, nestingLevel+1)
		}
		if n.Else != nil {
			if elseIf, ok := n.Else.(*ast.IfStmt); ok {
				complexity += e.calculateCognitiveComplexityNode(elseIf, nestingLevel)
			} else if elseBlock, ok := n.Else.(*ast.BlockStmt); ok {
				complexity += e.calculateCognitiveComplexity(elseBlock, nestingLevel+1)
			}
		}
		return complexity
	}
	return 0
}

// HalsteadMetrics represents Halstead complexity metrics.
type HalsteadMetrics struct {
	Difficulty float64 `json:"difficulty"`
	Volume     float64 `json:"volume"`
	Effort     float64 `json:"effort"`
}

// calculateHalsteadMetrics calculates Halstead complexity metrics.
func (e *Engine) calculateHalsteadMetrics(body *ast.BlockStmt) HalsteadMetrics {
	operators := e.extractHalsteadOperators(body)

	n1 := float64(operators.UniqueOperators) // Number of distinct operators
	n2 := float64(operators.UniqueOperands)  // Number of distinct operands
	N1 := float64(operators.TotalOperators)  // Total number of operators
	N2 := float64(operators.TotalOperands)   // Total number of operands

	// Avoid division by zero
	if n2 == 0 || N2 == 0 {
		return HalsteadMetrics{}
	}

	// Halstead metrics formulas
	vocabulary := n1 + n2                      // Program vocabulary
	length := N1 + N2                          // Program length
	difficulty := (n1 / 2) * (N2 / n2)         // Program difficulty
	volume := length * (math.Log2(vocabulary)) // Program volume
	effort := difficulty * volume              // Programming effort

	return HalsteadMetrics{
		Difficulty: difficulty,
		Volume:     volume,
		Effort:     effort,
	}
}

// extractHalsteadOperators extracts operators and operands for Halstead metrics.
func (e *Engine) extractHalsteadOperators(body *ast.BlockStmt) HalsteadOperators {
	operators := make(map[string]int)
	operands := make(map[string]int)
	totalOperators := 0
	totalOperands := 0

	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			// Binary operators
			op := node.Op.String()
			operators[op]++
			totalOperators++

		case *ast.UnaryExpr:
			// Unary operators
			op := node.Op.String()
			operators[op]++
			totalOperators++

		case *ast.AssignStmt:
			// Assignment operators
			op := node.Tok.String()
			operators[op]++
			totalOperators++

		case *ast.Ident:
			// Identifiers as operands
			if node.Name != "_" {
				operands[node.Name]++
				totalOperands++
			}

		case *ast.BasicLit:
			// Literals as operands
			operands[node.Value]++
			totalOperands++
		}
		return true
	})

	return HalsteadOperators{
		Operators:       operators,
		Operands:        operands,
		TotalOperators:  totalOperators,
		TotalOperands:   totalOperands,
		UniqueOperators: len(operators),
		UniqueOperands:  len(operands),
	}
}

// calculateNestingDepth calculates the maximum nesting depth.
func (e *Engine) calculateNestingDepth(body *ast.BlockStmt) int {
	maxDepth := 0

	var visit func(ast.Node, int) int
	visit = func(n ast.Node, depth int) int {
		currentMax := depth

		switch node := n.(type) {
		case *ast.BlockStmt:
			for _, stmt := range node.List {
				childMax := visit(stmt, depth)
				if childMax > currentMax {
					currentMax = childMax
				}
			}
		case *ast.IfStmt:
			currentMax = visit(node.Body, depth+1)
			if node.Else != nil {
				elseMax := visit(node.Else, depth+1)
				if elseMax > currentMax {
					currentMax = elseMax
				}
			}
		case *ast.ForStmt:
			if node.Body != nil {
				currentMax = visit(node.Body, depth+1)
			}
		case *ast.RangeStmt:
			if node.Body != nil {
				currentMax = visit(node.Body, depth+1)
			}
		case *ast.SwitchStmt:
			if node.Body != nil {
				currentMax = visit(node.Body, depth+1)
			}
		case *ast.TypeSwitchStmt:
			if node.Body != nil {
				currentMax = visit(node.Body, depth+1)
			}
		}

		return currentMax
	}

	maxDepth = visit(body, 0)
	return maxDepth
}

// calculateLogicalLinesOfCode counts logical lines of code (excluding comments and blank lines).
func (e *Engine) calculateLogicalLinesOfCode(body *ast.BlockStmt) int {
	logicalLines := 0

	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.ExprStmt, *ast.AssignStmt, *ast.DeclStmt, *ast.ReturnStmt,
			*ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt,
			*ast.TypeSwitchStmt, *ast.SelectStmt, *ast.GoStmt, *ast.DeferStmt:
			logicalLines++
		}
		return true
	})

	return logicalLines
}

// calculateFileMetrics aggregates function metrics into file-level metrics.
func (e *Engine) calculateFileMetrics(functions []*FunctionComplexity) ComplexityMetrics {
	if len(functions) == 0 {
		return ComplexityMetrics{}
	}

	totalCyclomatic := 0
	totalCognitive := 0
	totalLOC := 0
	totalLogicalLOC := 0
	maxNesting := 0
	totalTodos := 0
	totalHacks := 0
	functionsWithDocs := 0

	for _, fn := range functions {
		totalCyclomatic += fn.Metrics.CyclomaticComplexity
		totalCognitive += fn.Metrics.CognitiveComplexity
		totalLOC += fn.Metrics.LinesOfCode
		totalLogicalLOC += fn.Metrics.LogicalLinesOfCode
		totalTodos += fn.Metrics.TodoCount
		totalHacks += fn.Metrics.HackCount

		if fn.Metrics.NestingDepth > maxNesting {
			maxNesting = fn.Metrics.NestingDepth
		}

		if fn.Metrics.HasDocumentation {
			functionsWithDocs++
		}
	}

	return ComplexityMetrics{
		CyclomaticComplexity: totalCyclomatic / len(functions), // Average
		CognitiveComplexity:  totalCognitive / len(functions),  // Average
		LinesOfCode:          totalLOC,
		LogicalLinesOfCode:   totalLogicalLOC,
		NestingDepth:         maxNesting,
		TodoCount:            totalTodos,
		HackCount:            totalHacks,
		HasDocumentation:     functionsWithDocs > len(functions)/2, // Majority have docs
	}
}

// extractTypeName extracts the type name from an AST expression.
func (e *Engine) extractTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + e.extractTypeName(t.X)
	case *ast.SelectorExpr:
		return e.extractTypeName(t.X) + "." + t.Sel.Name
	default:
		return "unknown"
	}
}

// buildFunctionSignature builds a string representation of the function signature.
func (e *Engine) buildFunctionSignature(decl *ast.FuncDecl) string {
	var sig strings.Builder

	sig.WriteString("func ")

	// Add receiver for methods
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		sig.WriteString("(")
		field := decl.Recv.List[0]
		if len(field.Names) > 0 {
			sig.WriteString(field.Names[0].Name + " ")
		}
		sig.WriteString(e.extractTypeName(field.Type))
		sig.WriteString(") ")
	}

	sig.WriteString(decl.Name.Name)
	sig.WriteString("(")

	// Add parameters
	if decl.Type.Params != nil {
		for i, param := range decl.Type.Params.List {
			if i > 0 {
				sig.WriteString(", ")
			}
			if len(param.Names) > 0 {
				for j, name := range param.Names {
					if j > 0 {
						sig.WriteString(", ")
					}
					sig.WriteString(name.Name)
				}
				sig.WriteString(" ")
			}
			sig.WriteString(e.extractTypeName(param.Type))
		}
	}

	sig.WriteString(")")

	// Add return types
	if decl.Type.Results != nil && len(decl.Type.Results.List) > 0 {
		sig.WriteString(" ")
		if len(decl.Type.Results.List) > 1 {
			sig.WriteString("(")
		}
		for i, result := range decl.Type.Results.List {
			if i > 0 {
				sig.WriteString(", ")
			}
			sig.WriteString(e.extractTypeName(result.Type))
		}
		if len(decl.Type.Results.List) > 1 {
			sig.WriteString(")")
		}
	}

	return sig.String()
}

// updateStats updates the engine's analysis statistics.
func (e *Engine) updateStats(result *FileComplexity) {
	e.statsLock.Lock()
	defer e.statsLock.Unlock()

	e.stats.FilesAnalyzed++
	e.stats.FunctionsAnalyzed += int64(len(result.Functions))
	e.stats.TotalTime += result.AnalysisTime
	e.stats.WarningsGenerated += int64(len(result.Warnings))

	// Count high complexity functions
	for _, fn := range result.Functions {
		if fn.Metrics.IsComplexityWarning || fn.Metrics.IsCognitiveWarning {
			e.stats.HighComplexityCount++
		}
	}

	// Calculate average time per function
	if e.stats.FunctionsAnalyzed > 0 {
		e.stats.AverageTimePerFunction = e.stats.TotalTime / time.Duration(e.stats.FunctionsAnalyzed)
	}
}

// GetStats returns the current analysis statistics.
func (e *Engine) GetStats() AnalysisStats {
	e.statsLock.RLock()
	defer e.statsLock.RUnlock()
	return *e.stats
}

// IsEnabled returns true if complexity analysis is enabled.
func (e *Engine) IsEnabled() bool {
	return e.config.Enabled
}

// GetEnabledMetrics returns the list of enabled complexity metrics.
func (e *Engine) GetEnabledMetrics() []MetricType {
	metrics := make([]MetricType, 0, len(e.enabledMetrics))
	for metric := range e.enabledMetrics {
		metrics = append(metrics, metric)
	}
	return metrics
}

// GetThresholds returns the configured complexity thresholds.
func (e *Engine) GetThresholds() *config.ComplexityThresholds {
	return e.thresholds
}
