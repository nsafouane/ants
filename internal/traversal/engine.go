// Package traversal provides high-performance concurrent file traversal capabilities
// for the Context Engine's Tier 1 structural analysis.
package traversal

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/pubsub"
)

// FileInfo represents a discovered file with its metadata.
type FileInfo struct {
	Path         string    `json:"path"`
	RelativePath string    `json:"relative_path"`
	Size         int64     `json:"size"`
	ModTime      time.Time `json:"mod_time"`
	Language     string    `json:"language"`
	Extension    string    `json:"extension"`
	IsCode       bool      `json:"is_code"`
}

// TraversalStats tracks statistics during file traversal.
type TraversalStats struct {
	FilesTotal     int64     `json:"files_total"`
	FilesProcessed int64     `json:"files_processed"`
	FilesSkipped   int64     `json:"files_skipped"`
	FilesFiltered  int64     `json:"files_filtered"`
	BytesTotal     int64     `json:"bytes_total"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	Duration       time.Duration `json:"duration"`
	FilesPerSecond float64   `json:"files_per_second"`
}

// Engine provides massively parallel file traversal capabilities.
type Engine struct {
	config      *config.FileTraversalConfig
	broker      *pubsub.Broker[map[string]interface{}]
	rootPath    string
	workerCount int
	bufferSize  int
	
	// File filtering rules
	includeExts    map[string]bool
	excludeExts    map[string]bool
	excludeDirs    map[string]bool
	
	// Statistics
	stats       *TraversalStats
	statsLock   sync.RWMutex
	
	logger *slog.Logger
}

// New creates a new file traversal engine.
func New(cfg *config.FileTraversalConfig, broker *pubsub.Broker[map[string]interface{}], rootPath string) *Engine {
	if cfg == nil {
		cfg = &config.FileTraversalConfig{}
	}
	
	// Set defaults
	workerCount := cfg.WorkerCount
	if workerCount <= 0 {
		workerCount = runtime.NumCPU() * 2 // Default to CPU count * 2
	}
	
	bufferSize := cfg.BufferSize
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	
	engine := &Engine{
		config:      cfg,
		broker:      broker,
		rootPath:    rootPath,
		workerCount: workerCount,
		bufferSize:  bufferSize,
		includeExts: make(map[string]bool),
		excludeExts: make(map[string]bool),
		excludeDirs: make(map[string]bool),
		stats:       &TraversalStats{},
		logger:      slog.With("component", "traversal_engine"),
	}
	
	// Build filter maps
	engine.buildFilterMaps()
	
	return engine
}

// buildFilterMaps builds the internal filter maps for fast lookups.
func (e *Engine) buildFilterMaps() {
	// Include extensions
	for _, ext := range e.config.IncludeExtensions {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		e.includeExts[strings.ToLower(ext)] = true
	}
	
	// Exclude extensions
	for _, ext := range e.config.ExcludeExtensions {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		e.excludeExts[strings.ToLower(ext)] = true
	}
	
	// Exclude directories
	for _, dir := range e.config.ExcludeDirectories {
		e.excludeDirs[strings.ToLower(dir)] = true
	}
	
	// Add default excludes if none specified
	if len(e.excludeDirs) == 0 {
		defaults := []string{
			"node_modules", "vendor", ".git", ".svn", ".hg",
			"build", "dist", "target", "bin", "obj",
			".vscode", ".idea", "__pycache__",
			".pytest_cache", ".mypy_cache",
		}
		for _, dir := range defaults {
			e.excludeDirs[strings.ToLower(dir)] = true
		}
	}
}

// Traverse performs parallel file traversal and sends discovered files to the output channel.
func (e *Engine) Traverse(ctx context.Context) (<-chan *FileInfo, <-chan error) {
	fileChan := make(chan *FileInfo, e.bufferSize)
	errorChan := make(chan error, 10)
	
	// Initialize stats
	e.statsLock.Lock()
	e.stats = &TraversalStats{
		StartTime: time.Now(),
	}
	e.statsLock.Unlock()
	
	// Publish traversal started event
	if e.broker != nil {
		e.broker.Publish(pubsub.TraversalStartedEvent, map[string]interface{}{
			"root_path":    e.rootPath,
			"worker_count": e.workerCount,
			"start_time":   time.Now(),
		})
	}
	
	go func() {
		defer close(fileChan)
		defer close(errorChan)
		
		e.logger.Info("Starting file traversal",
			"root_path", e.rootPath,
			"worker_count", e.workerCount,
			"buffer_size", e.bufferSize)
		
		// Create path channel for discovered file paths
		pathChan := make(chan string, e.bufferSize*2)
		
		// Start file discovery goroutine
		discoveryCtx, discoveryCancel := context.WithCancel(ctx)
		defer discoveryCancel()
		
		go func() {
			defer close(pathChan)
			if err := e.discoverFiles(discoveryCtx, pathChan); err != nil {
				select {
				case errorChan <- fmt.Errorf("file discovery failed: %w", err):
				case <-ctx.Done():
				}
			}
		}()
		
		// Start worker pool to process discovered files
		var wg sync.WaitGroup
		for i := 0; i < e.workerCount; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				e.processFiles(ctx, workerID, pathChan, fileChan, errorChan)
			}(i)
		}
		
		// Wait for all workers to complete
		wg.Wait()
		
		// Finalize stats
		e.statsLock.Lock()
		e.stats.EndTime = time.Now()
		e.stats.Duration = e.stats.EndTime.Sub(e.stats.StartTime)
		if e.stats.Duration > 0 {
			e.stats.FilesPerSecond = float64(e.stats.FilesProcessed) / e.stats.Duration.Seconds()
		}
		finalStats := *e.stats
		e.statsLock.Unlock()
		
		e.logger.Info("File traversal completed",
			"files_total", finalStats.FilesTotal,
			"files_processed", finalStats.FilesProcessed,
			"files_skipped", finalStats.FilesSkipped,
			"duration", finalStats.Duration,
			"files_per_second", finalStats.FilesPerSecond)
		
		// Publish traversal completed event
		if e.broker != nil {
			e.broker.Publish(pubsub.TraversalCompletedEvent, map[string]interface{}{
				"stats": finalStats,
			})
		}
	}()
	
	return fileChan, errorChan
}

// discoverFiles walks the directory tree and sends file paths to the path channel.
func (e *Engine) discoverFiles(ctx context.Context, pathChan chan<- string) error {
	return filepath.WalkDir(e.rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Log but continue on individual file errors
			e.logger.Warn("Error accessing path", "path", path, "error", err)
			return nil
		}
		
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		// Skip excluded directories
		if d.IsDir() {
			dirName := strings.ToLower(d.Name())
			if e.excludeDirs[dirName] {
				atomic.AddInt64(&e.stats.FilesSkipped, 1)
				return filepath.SkipDir
			}
			return nil
		}
		
		// Only process regular files
		if !d.Type().IsRegular() {
			return nil
		}
		
		// Apply file filtering
		if !e.shouldIncludeFile(path) {
			atomic.AddInt64(&e.stats.FilesFiltered, 1)
			return nil
		}
		
		atomic.AddInt64(&e.stats.FilesTotal, 1)
		
		// Send path to workers
		select {
		case pathChan <- path:
		case <-ctx.Done():
			return ctx.Err()
		}
		
		return nil
	})
}

// processFiles processes discovered file paths and creates FileInfo objects.
func (e *Engine) processFiles(ctx context.Context, workerID int, pathChan <-chan string, fileChan chan<- *FileInfo, errorChan chan<- error) {
	logger := e.logger.With("worker_id", workerID)
	
	for {
		select {
		case path, ok := <-pathChan:
			if !ok {
				return // Channel closed
			}
			
			fileInfo, err := e.processFile(path)
			if err != nil {
				logger.Warn("Error processing file", "path", path, "error", err)
				select {
				case errorChan <- err:
				case <-ctx.Done():
					return
				}
				continue
			}
			
			if fileInfo != nil {
				atomic.AddInt64(&e.stats.FilesProcessed, 1)
				atomic.AddInt64(&e.stats.BytesTotal, fileInfo.Size)
				
				select {
				case fileChan <- fileInfo:
				case <-ctx.Done():
					return
				}
			}
			
		case <-ctx.Done():
			return
		}
	}
}

// processFile creates a FileInfo object for a given file path.
func (e *Engine) processFile(path string) (*FileInfo, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", path, err)
	}
	
	// Calculate relative path
	relPath, err := filepath.Rel(e.rootPath, path)
	if err != nil {
		relPath = path
	}
	
	ext := strings.ToLower(filepath.Ext(path))
	language := detectLanguage(ext)
	isCode := isCodeFile(ext, language)
	
	return &FileInfo{
		Path:         path,
		RelativePath: relPath,
		Size:         stat.Size(),
		ModTime:      stat.ModTime(),
		Language:     language,
		Extension:    ext,
		IsCode:       isCode,
	}, nil
}

// shouldIncludeFile determines if a file should be included based on filtering rules.
func (e *Engine) shouldIncludeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	
	// Check exclude extensions first
	if e.excludeExts[ext] {
		return false
	}
	
	// If include extensions are specified, only include those
	if len(e.includeExts) > 0 {
		return e.includeExts[ext]
	}
	
	return true
}

// GetStats returns the current traversal statistics.
func (e *Engine) GetStats() TraversalStats {
	e.statsLock.RLock()
	defer e.statsLock.RUnlock()
	return *e.stats
}

// detectLanguage detects the programming language from file extension.
func detectLanguage(ext string) string {
	languageMap := map[string]string{
		".go":     "go",
		".py":     "python",
		".js":     "javascript",
		".ts":     "typescript",
		".jsx":    "javascript",
		".tsx":    "typescript",
		".java":   "java",
		".c":      "c",
		".cpp":    "cpp",
		".cc":     "cpp",
		".cxx":    "cpp",
		".h":      "c",
		".hpp":    "cpp",
		".hxx":    "cpp",
		".rs":     "rust",
		".rb":     "ruby",
		".php":    "php",
		".cs":     "csharp",
		".swift":  "swift",
		".kt":     "kotlin",
		".scala":  "scala",
		".sql":    "sql",
		".sh":     "shell",
		".bash":   "shell",
		".zsh":    "shell",
		".fish":   "shell",
		".ps1":    "powershell",
		".r":      "r",
		".R":      "r",
		".m":      "objective-c",
		".mm":     "objective-c",
		".pl":     "perl",
		".lua":    "lua",
		".dart":   "dart",
		".elm":    "elm",
		".ex":     "elixir",
		".exs":    "elixir",
		".erl":    "erlang",
		".hrl":    "erlang",
		".f":      "fortran",
		".f90":    "fortran",
		".f95":    "fortran",
		".hs":     "haskell",
		".lhs":    "haskell",
		".ml":     "ocaml",
		".mli":    "ocaml",
		".pas":    "pascal",
		".pp":     "pascal",
		".clj":    "clojure",
		".cljs":   "clojure",
		".cljc":   "clojure",
		".scm":    "scheme",
		".ss":     "scheme",
		".lisp":   "lisp",
		".lsp":    "lisp",
		".cl":     "lisp",
		".zig":    "zig",
		".nim":    "nim",
		".nims":   "nim",
		".v":      "vlang",
		".d":      "dlang",
		".cr":     "crystal",
		".jl":     "julia",
		".groovy": "groovy",
		".gradle": "groovy",
		".gvy":    "groovy",
		".gy":     "groovy",
		".gsh":    "groovy",
	}
	
	if lang, exists := languageMap[ext]; exists {
		return lang
	}
	
	return "unknown"
}

// isCodeFile determines if a file is a code file based on extension and language.
func isCodeFile(ext, language string) bool {
	// Code file extensions
	codeExtensions := map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".java": true, ".c": true, ".cpp": true, ".cc": true, ".cxx": true,
		".h": true, ".hpp": true, ".hxx": true, ".rs": true, ".rb": true,
		".php": true, ".cs": true, ".swift": true, ".kt": true, ".scala": true,
		".sql": true, ".sh": true, ".bash": true, ".zsh": true, ".fish": true,
		".ps1": true, ".r": true, ".m": true, ".mm": true, ".pl": true,
		".lua": true, ".dart": true, ".elm": true, ".ex": true, ".exs": true,
		".erl": true, ".hrl": true, ".f": true, ".f90": true, ".f95": true,
		".hs": true, ".lhs": true, ".ml": true, ".mli": true, ".pas": true,
		".pp": true, ".clj": true, ".cljs": true, ".cljc": true, ".scm": true,
		".ss": true, ".lisp": true, ".lsp": true, ".cl": true, ".zig": true,
		".nim": true, ".nims": true, ".v": true, ".d": true, ".cr": true,
		".jl": true, ".groovy": true, ".gradle": true, ".gvy": true, ".gy": true, ".gsh": true,
	}
	
	return codeExtensions[ext] || language != "unknown"
}