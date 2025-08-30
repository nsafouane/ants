package traversal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/require"
)

func TestEngine_New(t *testing.T) {
	t.Parallel()

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	cfg := &config.FileTraversalConfig{
		WorkerCount: 4,
		BufferSize:  500,
		IncludeExtensions: []string{".go", ".py"},
		ExcludeExtensions: []string{".exe", ".bin"},
		ExcludeDirectories: []string{"vendor", "node_modules"},
	}

	engine := New(cfg, broker, "/test/path")

	require.NotNil(t, engine)
	require.Equal(t, 4, engine.workerCount)
	require.Equal(t, 500, engine.bufferSize)
	require.Equal(t, "/test/path", engine.rootPath)
	
	// Check filter maps are built correctly
	require.True(t, engine.includeExts[".go"])
	require.True(t, engine.includeExts[".py"])
	require.True(t, engine.excludeExts[".exe"])
	require.True(t, engine.excludeExts[".bin"])
	require.True(t, engine.excludeDirs["vendor"])
	require.True(t, engine.excludeDirs["node_modules"])
}

func TestEngine_New_WithDefaults(t *testing.T) {
	t.Parallel()

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine := New(nil, broker, "/test/path")

	require.NotNil(t, engine)
	require.Greater(t, engine.workerCount, 0) // Should use CPU count * 2
	require.Equal(t, 1000, engine.bufferSize) // Default buffer size
	
	// Check default excludes are applied
	require.True(t, engine.excludeDirs["node_modules"])
	require.True(t, engine.excludeDirs["vendor"])
	require.True(t, engine.excludeDirs[".git"])
}

func TestEngine_shouldIncludeFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   *config.FileTraversalConfig
		filePath string
		expected bool
	}{
		{
			name: "include when no filters specified",
			config: &config.FileTraversalConfig{},
			filePath: "/test/file.go",
			expected: true,
		},
		{
			name: "include when extension matches include list",
			config: &config.FileTraversalConfig{
				IncludeExtensions: []string{".go", ".py"},
			},
			filePath: "/test/file.go",
			expected: true,
		},
		{
			name: "exclude when extension not in include list",
			config: &config.FileTraversalConfig{
				IncludeExtensions: []string{".go", ".py"},
			},
			filePath: "/test/file.js",
			expected: false,
		},
		{
			name: "exclude when extension in exclude list",
			config: &config.FileTraversalConfig{
				ExcludeExtensions: []string{".exe", ".bin"},
			},
			filePath: "/test/file.exe",
			expected: false,
		},
		{
			name: "include when not in exclude list",
			config: &config.FileTraversalConfig{
				ExcludeExtensions: []string{".exe", ".bin"},
			},
			filePath: "/test/file.go",
			expected: true,
		},
		{
			name: "exclude takes precedence over include",
			config: &config.FileTraversalConfig{
				IncludeExtensions: []string{".go"},
				ExcludeExtensions: []string{".go"},
			},
			filePath: "/test/file.go",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			broker := pubsub.NewBroker[map[string]interface{}]()
			defer broker.Shutdown()

			engine := New(tt.config, broker, "/test")
			result := engine.shouldIncludeFile(tt.filePath)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestEngine_detectLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		extension string
		expected  string
	}{
		{".go", "go"},
		{".py", "python"},
		{".js", "javascript"},
		{".ts", "typescript"},
		{".java", "java"},
		{".cpp", "cpp"},
		{".rs", "rust"},
		{".rb", "ruby"},
		{".php", "php"},
		{".unknown", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.extension, func(t *testing.T) {
			t.Parallel()

			result := detectLanguage(tt.extension)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestEngine_isCodeFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		extension string
		language  string
		expected  bool
	}{
		{".go", "go", true},
		{".py", "python", true},
		{".js", "javascript", true},
		{".txt", "unknown", false},
		{".md", "unknown", false},
		{".json", "unknown", false},
		{".unknown", "go", true}, // Language overrides extension
	}

	for _, tt := range tests {
		t.Run(tt.extension+"_"+tt.language, func(t *testing.T) {
			t.Parallel()

			result := isCodeFile(tt.extension, tt.language)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestEngine_TraverseWithRealFiles(t *testing.T) {
	t.Parallel()

	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()
	
	// Create test files
	testFiles := []struct {
		path    string
		content string
	}{
		{"main.go", "package main\n\nfunc main() {}\n"},
		{"utils.py", "def hello():\n    pass\n"},
		{"README.md", "# Test Project\n"},
		{"vendor/lib.go", "package lib\n"},
		{"node_modules/package.json", `{"name": "test"}`},
		{"subdir/test.js", "console.log('hello');\n"},
	}

	for _, tf := range testFiles {
		fullPath := filepath.Join(tmpDir, tf.path)
		dir := filepath.Dir(fullPath)
		
		err := os.MkdirAll(dir, 0o755)
		require.NoError(t, err)
		
		err = os.WriteFile(fullPath, []byte(tf.content), 0o644)
		require.NoError(t, err)
	}

	// Test configuration that includes code files but excludes vendor/node_modules
	cfg := &config.FileTraversalConfig{
		WorkerCount:       2,
		BufferSize:        10,
		IncludeExtensions: []string{".go", ".py", ".js"},
		ExcludeDirectories: []string{"vendor", "node_modules"},
	}

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine := New(cfg, broker, tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fileChan, errorChan := engine.Traverse(ctx)

	var files []*FileInfo
	var errors []error

	// Collect results
	for {
		select {
		case file, ok := <-fileChan:
			if !ok {
				fileChan = nil
			} else {
				files = append(files, file)
			}
		case err, ok := <-errorChan:
			if !ok {
				errorChan = nil
			} else {
				errors = append(errors, err)
			}
		case <-ctx.Done():
			t.Fatal("Test timed out")
		}

		if fileChan == nil && errorChan == nil {
			break
		}
	}

	// Verify results
	require.Empty(t, errors, "Should not have any errors")
	require.Len(t, files, 3, "Should find exactly 3 files (main.go, utils.py, subdir/test.js)")

	// Check that vendor and node_modules files were excluded
	for _, file := range files {
		require.NotContains(t, file.RelativePath, "vendor")
		require.NotContains(t, file.RelativePath, "node_modules")
	}

	// Verify file properties
	fileMap := make(map[string]*FileInfo)
	for _, file := range files {
		fileMap[file.RelativePath] = file
	}

	// Check main.go
	goFile := fileMap["main.go"]
	require.NotNil(t, goFile)
	require.Equal(t, "go", goFile.Language)
	require.Equal(t, ".go", goFile.Extension)
	require.True(t, goFile.IsCode)
	require.Greater(t, goFile.Size, int64(0))

	// Check utils.py
	pyFile := fileMap["utils.py"]
	require.NotNil(t, pyFile)
	require.Equal(t, "python", pyFile.Language)
	require.Equal(t, ".py", pyFile.Extension)
	require.True(t, pyFile.IsCode)

	// Check subdir/test.js
	jsFile := fileMap[filepath.Join("subdir", "test.js")]
	require.NotNil(t, jsFile)
	require.Equal(t, "javascript", jsFile.Language)
	require.Equal(t, ".js", jsFile.Extension)
	require.True(t, jsFile.IsCode)

	// Verify stats
	stats := engine.GetStats()
	require.Equal(t, int64(3), stats.FilesProcessed)
	require.Greater(t, stats.FilesTotal, int64(0))
	require.Greater(t, stats.FilesPerSecond, float64(0))
	require.Greater(t, stats.BytesTotal, int64(0))
	require.False(t, stats.StartTime.IsZero())
	require.False(t, stats.EndTime.IsZero())
	require.Greater(t, stats.Duration, time.Duration(0))
}

func TestEngine_TraverseWithCancellation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	
	// Create a single test file
	err := os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package main"), 0o644)
	require.NoError(t, err)

	cfg := &config.FileTraversalConfig{
		WorkerCount: 1,
		BufferSize:  1,
	}

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine := New(cfg, broker, tmpDir)

	ctx, cancel := context.WithCancel(context.Background())
	
	fileChan, errorChan := engine.Traverse(ctx)

	// Cancel immediately
	cancel()

	// Collect results with timeout
	timeout := time.After(2 * time.Second)
	
	for {
		select {
		case _, ok := <-fileChan:
			if !ok {
				fileChan = nil
			}
		case _, ok := <-errorChan:
			if !ok {
				errorChan = nil
			}
		case <-timeout:
			t.Fatal("Traversal did not stop after cancellation")
		}

		if fileChan == nil && errorChan == nil {
			break
		}
	}
}

func TestEngine_GetStats(t *testing.T) {
	t.Parallel()

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine := New(nil, broker, "/test")

	// Initially, stats should be zero
	stats := engine.GetStats()
	require.Zero(t, stats.FilesTotal)
	require.Zero(t, stats.FilesProcessed)
	require.Zero(t, stats.BytesTotal)
}

func BenchmarkEngine_shouldIncludeFile(b *testing.B) {
	cfg := &config.FileTraversalConfig{
		IncludeExtensions: []string{".go", ".py", ".js", ".ts"},
		ExcludeExtensions: []string{".exe", ".bin", ".dll"},
	}

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine := New(cfg, broker, "/test")

	testPaths := []string{
		"/test/main.go",
		"/test/utils.py",
		"/test/app.js",
		"/test/types.ts",
		"/test/app.exe",
		"/test/lib.so",
		"/test/README.md",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := testPaths[i%len(testPaths)]
		engine.shouldIncludeFile(path)
	}
}

func BenchmarkDetectLanguage(b *testing.B) {
	extensions := []string{
		".go", ".py", ".js", ".ts", ".java", ".cpp", ".rs", ".rb", ".php",
		".c", ".h", ".hpp", ".swift", ".kt", ".scala", ".unknown",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ext := extensions[i%len(extensions)]
		detectLanguage(ext)
	}
}