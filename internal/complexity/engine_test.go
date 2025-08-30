package complexity

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestEngine_New(t *testing.T) {
	t.Parallel()

	cfg := &config.ComplexityAnalysisConfig{
		Enabled: true,
		Metrics: []string{"cyclomatic", "cognitive", "halstead"},
		WarningThresholds: &config.ComplexityThresholds{
			CyclomaticComplexity: 10,
			CognitiveComplexity:  15,
			MaxFunctionLength:    50,
		},
	}

	engine := New(cfg)

	require.NotNil(t, engine)
	require.Equal(t, cfg, engine.config)
	require.True(t, engine.enabledMetrics[MetricCyclomatic])
	require.True(t, engine.enabledMetrics[MetricCognitive])
	require.True(t, engine.enabledMetrics[MetricHalstead])
	require.Equal(t, 10, engine.thresholds.CyclomaticComplexity)
}

func TestEngine_New_WithDefaults(t *testing.T) {
	t.Parallel()

	engine := New(nil)

	require.NotNil(t, engine)
	require.True(t, engine.config.Enabled)
	require.Contains(t, engine.config.Metrics, "cyclomatic")
	require.Contains(t, engine.config.Metrics, "cognitive")
	require.Contains(t, engine.config.Metrics, "halstead")
	require.Equal(t, 10, engine.thresholds.CyclomaticComplexity)
	require.Equal(t, 15, engine.thresholds.CognitiveComplexity)
	require.Equal(t, 50, engine.thresholds.MaxFunctionLength)
}

func TestEngine_AnalyzeAST_SimpleFunction(t *testing.T) {
	t.Parallel()

	engine := New(nil)

	goCode := `package main

// Hello prints a greeting message
func Hello(name string) string {
	if name == "" {
		return "Hello, World!"
	}
	return "Hello, " + name + "!"
}`

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "test.go", goCode, parser.ParseComments)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := engine.AnalyzeAST(ctx, fileSet, file, "test.go")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "test.go", result.FilePath)
	require.Equal(t, "main", result.Package)
	require.Equal(t, "go", result.Language)
	require.Len(t, result.Functions, 1)

	// Check function complexity
	helloFunc := result.Functions[0]
	require.Equal(t, "Hello", helloFunc.Name)
	require.True(t, helloFunc.IsExported)
	require.False(t, helloFunc.IsMethod)
	require.Equal(t, 2, helloFunc.Metrics.CyclomaticComplexity) // Base 1 + if statement
	require.Greater(t, helloFunc.Metrics.LinesOfCode, 0)
	require.True(t, helloFunc.Metrics.HasDocumentation)
	require.False(t, helloFunc.Metrics.IsComplexityWarning) // Should be under threshold
}

func TestEngine_AnalyzeAST_ComplexFunction(t *testing.T) {
	t.Parallel()

	engine := New(&config.ComplexityAnalysisConfig{
		Enabled: true,
		Metrics: []string{"cyclomatic", "cognitive"},
		WarningThresholds: &config.ComplexityThresholds{
			CyclomaticComplexity: 3, // Low threshold for testing
			CognitiveComplexity:  3,
			MaxFunctionLength:    10,
		},
	})

	goCode := `package main

func ComplexFunction(x, y, z int) int {
	result := 0
	
	if x > 0 {
		for i := 0; i < x; i++ {
			if i%2 == 0 {
				switch y {
				case 1:
					result += 1
				case 2:
					result += 2
				default:
					result += 0
				}
			} else {
				result += i
			}
		}
	} else if x < 0 {
		for j := x; j < 0; j++ {
			result -= j
		}
	}
	
	return result
}`

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "test.go", goCode, parser.ParseComments)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := engine.AnalyzeAST(ctx, fileSet, file, "test.go")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Functions, 1)

	complexFunc := result.Functions[0]
	require.Equal(t, "ComplexFunction", complexFunc.Name)
	require.Greater(t, complexFunc.Metrics.CyclomaticComplexity, 3) // Should exceed threshold
	require.Greater(t, complexFunc.Metrics.CognitiveComplexity, 3)  // Should exceed threshold
	require.True(t, complexFunc.Metrics.IsComplexityWarning)
	require.True(t, complexFunc.Metrics.IsCognitiveWarning)
	require.Greater(t, complexFunc.Metrics.NestingDepth, 1)

	// Should have warnings
	require.NotEmpty(t, result.Warnings)
	require.Contains(t, result.Warnings[0], "high cyclomatic complexity")
}

func TestEngine_AnalyzeAST_Method(t *testing.T) {
	t.Parallel()

	engine := New(nil)

	goCode := `package main

type Calculator struct {
	value int
}

// Add adds a number to the calculator value
func (c *Calculator) Add(x int) {
	c.value += x
}

// GetValue returns the current value
func (c Calculator) GetValue() int {
	return c.value
}`

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "test.go", goCode, parser.ParseComments)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := engine.AnalyzeAST(ctx, fileSet, file, "test.go")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Functions, 2)

	// Check Add method
	var addMethod *FunctionComplexity
	for _, fn := range result.Functions {
		if fn.Name == "Add" {
			addMethod = fn
			break
		}
	}
	require.NotNil(t, addMethod)
	require.True(t, addMethod.IsMethod)
	require.Equal(t, "*Calculator", addMethod.ReceiverType)
	require.Contains(t, addMethod.Signature, "func (c *Calculator) Add(x int)")

	// Check GetValue method
	var getValueMethod *FunctionComplexity
	for _, fn := range result.Functions {
		if fn.Name == "GetValue" {
			getValueMethod = fn
			break
		}
	}
	require.NotNil(t, getValueMethod)
	require.True(t, getValueMethod.IsMethod)
	require.Equal(t, "Calculator", getValueMethod.ReceiverType)
	require.Contains(t, getValueMethod.Signature, "func (c Calculator) GetValue() int")
}

func TestEngine_AnalyzeAST_WithTodos(t *testing.T) {
	t.Parallel()

	engine := New(nil)

	goCode := `package main

// ProcessData processes the input data
// TODO: Add validation
// HACK: This is a temporary workaround
func ProcessData(data []byte) error {
	// TODO: Implement proper error handling
	return nil
}`

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "test.go", goCode, parser.ParseComments)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := engine.AnalyzeAST(ctx, fileSet, file, "test.go")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Functions, 1)

	processFunc := result.Functions[0]
	require.Equal(t, "ProcessData", processFunc.Name)
	require.True(t, processFunc.Metrics.HasDocumentation)
	require.Equal(t, 1, processFunc.Metrics.TodoCount) // Should find 1 TODO in function doc
	require.Equal(t, 1, processFunc.Metrics.HackCount) // Should find 1 HACK
}

func TestEngine_CyclomaticComplexity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		code       string
		expectedCC int
	}{
		{
			name:       "simple function",
			code:       `func Simple() { return }`,
			expectedCC: 1,
		},
		{
			name: "function with if statement",
			code: `func WithIf(x int) {
				if x > 0 {
					return
				}
			}`,
			expectedCC: 2,
		},
		{
			name: "function with for loop",
			code: `func WithFor() {
				for i := 0; i < 10; i++ {
					// do something
				}
			}`,
			expectedCC: 2,
		},
		{
			name: "function with switch",
			code: `func WithSwitch(x int) {
				switch x {
				case 1:
					return
				case 2:
					return
				default:
					return
				}
			}`,
			expectedCC: 4, // Base 1 + switch 1 + 2 cases = 4
		},
		{
			name: "complex function",
			code: `func Complex(x int) {
				if x > 0 {
					for i := 0; i < x; i++ {
						if i%2 == 0 {
							return
						}
					}
				} else if x < 0 {
					return
				}
			}`,
			expectedCC: 5, // Base 1 + if + for + if + else if
		},
	}

	engine := New(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fullCode := "package main\n\n" + tt.code

			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, "test.go", fullCode, 0)
			require.NoError(t, err)

			ctx := context.Background()
			result, err := engine.AnalyzeAST(ctx, fileSet, file, "test.go")

			require.NoError(t, err)
			require.Len(t, result.Functions, 1)
			require.Equal(t, tt.expectedCC, result.Functions[0].Metrics.CyclomaticComplexity)
		})
	}
}

func TestEngine_CognitiveComplexity(t *testing.T) {
	t.Parallel()

	// Cognitive complexity includes nesting penalties
	goCode := `package main

func CognitiveExample(x int) {
	if x > 0 {           // +1
		for i := 0; i < x; i++ {  // +2 (1 + 1 nesting)
			if i%2 == 0 {         // +3 (1 + 2 nesting)
				return
			}
		}
	}
}`

	engine := New(nil)

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "test.go", goCode, 0)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := engine.AnalyzeAST(ctx, fileSet, file, "test.go")

	require.NoError(t, err)
	require.Len(t, result.Functions, 1)

	cognitiveComplexity := result.Functions[0].Metrics.CognitiveComplexity
	require.Greater(t, cognitiveComplexity, 3) // Should be higher due to nesting
}

func TestEngine_HalsteadMetrics(t *testing.T) {
	t.Parallel()

	goCode := `package main

func HalsteadExample(x, y int) int {
	result := x + y
	if result > 10 {
		result = result * 2
	}
	return result
}`

	engine := New(nil)

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "test.go", goCode, 0)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := engine.AnalyzeAST(ctx, fileSet, file, "test.go")

	require.NoError(t, err)
	require.Len(t, result.Functions, 1)

	metrics := result.Functions[0].Metrics
	require.Greater(t, metrics.HalsteadDifficulty, 0.0)
	require.Greater(t, metrics.HalsteadVolume, 0.0)
	require.Greater(t, metrics.HalsteadEffort, 0.0)
}

func TestEngine_NestingDepth(t *testing.T) {
	t.Parallel()

	goCode := `package main

func DeepNesting() {
	if true {                    // depth 1
		for i := 0; i < 10; i++ {    // depth 2
			if i%2 == 0 {                // depth 3
				switch i {                   // depth 4
				case 2:
					if i == 2 {                  // depth 5
						return
					}
				}
			}
		}
	}
}`

	engine := New(nil)

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "test.go", goCode, 0)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := engine.AnalyzeAST(ctx, fileSet, file, "test.go")

	require.NoError(t, err)
	require.Len(t, result.Functions, 1)

	nestingDepth := result.Functions[0].Metrics.NestingDepth
	require.GreaterOrEqual(t, nestingDepth, 4) // Should detect deep nesting
}

func TestEngine_FileMetrics(t *testing.T) {
	t.Parallel()

	goCode := `package main

// Function1 is documented
func Function1() {
	if true {
		return
	}
}

func Function2() {
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			continue
		}
	}
}

// Function3 has documentation
func Function3() {
	// Simple function
}`

	engine := New(nil)

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "test.go", goCode, parser.ParseComments)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := engine.AnalyzeAST(ctx, fileSet, file, "test.go")

	require.NoError(t, err)
	require.Len(t, result.Functions, 3)

	// Check overall file metrics
	overall := result.OverallMetrics
	require.Greater(t, overall.CyclomaticComplexity, 0) // Average complexity
	require.Greater(t, overall.LinesOfCode, 0)          // Total LOC
	require.True(t, overall.HasDocumentation)           // Majority have docs
}

func TestEngine_DisabledAnalysis(t *testing.T) {
	t.Parallel()

	cfg := &config.ComplexityAnalysisConfig{
		Enabled: false,
	}
	engine := New(cfg)

	goCode := `package main
func Test() {}`

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "test.go", goCode, 0)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := engine.AnalyzeAST(ctx, fileSet, file, "test.go")

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "complexity analysis is disabled")
}

func TestEngine_GetStats(t *testing.T) {
	t.Parallel()

	engine := New(nil)

	// Initially empty
	stats := engine.GetStats()
	require.Equal(t, int64(0), stats.FilesAnalyzed)
	require.Equal(t, int64(0), stats.FunctionsAnalyzed)

	// Analyze a file
	goCode := `package main
func Test1() {}
func Test2() {}`

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "test.go", goCode, 0)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = engine.AnalyzeAST(ctx, fileSet, file, "test.go")
	require.NoError(t, err)

	// Check updated stats
	stats = engine.GetStats()
	require.Equal(t, int64(1), stats.FilesAnalyzed)
	require.Equal(t, int64(2), stats.FunctionsAnalyzed)
	require.GreaterOrEqual(t, stats.TotalTime, time.Duration(0))
}

func TestEngine_GetEnabledMetrics(t *testing.T) {
	t.Parallel()

	cfg := &config.ComplexityAnalysisConfig{
		Enabled: true,
		Metrics: []string{"cyclomatic", "cognitive"},
	}
	engine := New(cfg)

	metrics := engine.GetEnabledMetrics()
	require.Len(t, metrics, 2)
	require.Contains(t, metrics, MetricCyclomatic)
	require.Contains(t, metrics, MetricCognitive)
}

func TestEngine_IsEnabled(t *testing.T) {
	t.Parallel()

	enabledEngine := New(&config.ComplexityAnalysisConfig{Enabled: true})
	require.True(t, enabledEngine.IsEnabled())

	disabledEngine := New(&config.ComplexityAnalysisConfig{Enabled: false})
	require.False(t, disabledEngine.IsEnabled())
}

func TestEngine_GetThresholds(t *testing.T) {
	t.Parallel()

	cfg := &config.ComplexityAnalysisConfig{
		Enabled: true,
		WarningThresholds: &config.ComplexityThresholds{
			CyclomaticComplexity: 5,
			CognitiveComplexity:  8,
			MaxFunctionLength:    25,
		},
	}
	engine := New(cfg)

	thresholds := engine.GetThresholds()
	require.Equal(t, 5, thresholds.CyclomaticComplexity)
	require.Equal(t, 8, thresholds.CognitiveComplexity)
	require.Equal(t, 25, thresholds.MaxFunctionLength)
}

func BenchmarkEngine_AnalyzeAST(b *testing.B) {
	engine := New(nil)

	goCode := `package main

import "fmt"

func ComplexFunction(x, y, z int) int {
	result := 0
	
	if x > 0 {
		for i := 0; i < x; i++ {
			if i%2 == 0 {
				switch y {
				case 1:
					result += 1
				case 2:
					result += 2
				case 3:
					result += 3
				default:
					result += 0
				}
			} else {
				result += i
			}
		}
	} else if x < 0 {
		for j := x; j < 0; j++ {
			result -= j
		}
	}
	
	for k := 0; k < z; k++ {
		result *= 2
	}
	
	return result
}

func SimpleFunction() {
	fmt.Println("Hello, World!")
}

func AnotherFunction(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + " " + b
}`

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "bench.go", goCode, parser.ParseComments)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.AnalyzeAST(ctx, fileSet, file, "bench.go")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngine_CyclomaticComplexity(b *testing.B) {
	engine := New(nil)

	goCode := `package main

func BenchmarkFunction(x int) int {
	result := 0
	
	if x > 100 {
		for i := 0; i < x; i++ {
			if i%2 == 0 {
				if i%4 == 0 {
					result += i * 2
				} else {
					result += i
				}
			} else {
				switch i % 3 {
				case 0:
					result -= 1
				case 1:
					result += 1
				case 2:
					result *= 2
				}
			}
		}
	} else if x > 50 {
		for j := 0; j < x; j += 2 {
			result += j
		}
	} else {
		result = x * x
	}
	
	return result
}`

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "bench.go", goCode, 0)
	if err != nil {
		b.Fatal(err)
	}

	// Extract the function body for benchmarking
	var funcBody *ast.BlockStmt
	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Name.Name == "BenchmarkFunction" {
			funcBody = funcDecl.Body
			break
		}
	}

	if funcBody == nil {
		b.Fatal("Function not found")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.calculateCyclomaticComplexity(funcBody)
	}
}
