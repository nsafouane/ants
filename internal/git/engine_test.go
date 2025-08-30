package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"
)

func TestEngine_New(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	
	// Initialize a git repository
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)
	require.NotNil(t, repo)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	cfg := &config.GitAnalysisConfig{
		Enabled:           true,
		MaxCommits:        100,
		HotspotWindowDays: 30,
		MinHotspotChanges: 2,
	}

	engine, err := New(cfg, broker, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, engine)
	require.Equal(t, cfg, engine.config)
	require.Equal(t, tmpDir, engine.repoPath)
	require.True(t, engine.IsGitRepository())
}

func TestEngine_New_WithDefaults(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	
	// Initialize a git repository
	_, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine, err := New(nil, broker, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, engine)
	require.True(t, engine.config.Enabled)
	require.Equal(t, 1000, engine.config.MaxCommits)
	require.Equal(t, 90, engine.config.HotspotWindowDays)
	require.Equal(t, 5, engine.config.MinHotspotChanges)
}

func TestEngine_New_NonGitRepository(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	// Don't initialize as git repository

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine, err := New(nil, broker, tmpDir)
	require.Error(t, err)
	require.Nil(t, engine)
	require.Contains(t, err.Error(), "failed to open git repository")
}

func TestEngine_AnalyzeRepository_EmptyRepo(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	
	// Initialize empty git repository
	_, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine, err := New(nil, broker, tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := engine.AnalyzeRepository(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, tmpDir, result.RepositoryPath)
	require.Equal(t, 0, result.Stats.TotalCommits)
	require.Equal(t, 0, len(result.Hotspots))
	require.Equal(t, 0, len(result.Authors))
	require.Greater(t, result.AnalysisTime, time.Duration(0))
}

func TestEngine_AnalyzeRepository_WithCommits(t *testing.T) {
	tmpDir := createTestRepositoryWithCommits(t)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	cfg := &config.GitAnalysisConfig{
		Enabled:           true,
		MaxCommits:        50,
		HotspotWindowDays: 365, // Include all commits
		MinHotspotChanges: 1,   // Include all changed files
	}

	engine, err := New(cfg, broker, tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := engine.AnalyzeRepository(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Greater(t, result.Stats.TotalCommits, 0)
	require.Greater(t, len(result.Authors), 0)
	require.Greater(t, len(result.Hotspots), 0)

	// Check that we have hotspots
	for _, hotspot := range result.Hotspots {
		require.NotEmpty(t, hotspot.Path)
		require.Greater(t, hotspot.ChangeFrequency, 0)
		require.Contains(t, []string{"hot", "warm", "cold"}, hotspot.ChangeCategory)
		require.GreaterOrEqual(t, hotspot.HotspotScore, 0.0)
	}

	// Check that hotspots are sorted by score (descending)
	for i := 1; i < len(result.Hotspots); i++ {
		require.GreaterOrEqual(t, result.Hotspots[i-1].HotspotScore, result.Hotspots[i].HotspotScore)
	}

	// Check authors
	require.Greater(t, len(result.Authors), 0)
	author := result.Authors[0]
	require.NotEmpty(t, author.Name)
	require.NotEmpty(t, author.Email)
	require.Greater(t, author.CommitCount, 0)
}

func TestEngine_GetCommitContext(t *testing.T) {
	tmpDir := createTestRepositoryWithCommits(t)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine, err := New(nil, broker, tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	
	// Get commits for a specific file
	commits, err := engine.GetCommitContext(ctx, "main.go", 5)
	require.NoError(t, err)
	require.LessOrEqual(t, len(commits), 5)

	for _, commit := range commits {
		require.NotEmpty(t, commit.Hash)
		require.NotEmpty(t, commit.ShortHash)
		require.Len(t, commit.ShortHash, 8)
		require.NotEmpty(t, commit.Author)
		require.NotEmpty(t, commit.AuthorEmail)
		require.NotEmpty(t, commit.Message)
		require.Equal(t, 1, commit.FilesChanged) // We're querying for one specific file
	}
}

func TestEngine_GetHotspotFiles(t *testing.T) {
	tmpDir := createTestRepositoryWithCommits(t)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	cfg := &config.GitAnalysisConfig{
		Enabled:           true,
		MaxCommits:        50,
		HotspotWindowDays: 365,
		MinHotspotChanges: 1,
	}

	engine, err := New(cfg, broker, tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	
	// Get top 3 hotspot files
	hotspots, err := engine.GetHotspotFiles(ctx, 3)
	require.NoError(t, err)
	require.LessOrEqual(t, len(hotspots), 3)

	// Should be sorted by hotspot score
	for i := 1; i < len(hotspots); i++ {
		require.GreaterOrEqual(t, hotspots[i-1].HotspotScore, hotspots[i].HotspotScore)
	}
}

func TestEngine_DisabledAnalysis(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	_, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	cfg := &config.GitAnalysisConfig{
		Enabled: false,
	}

	engine, err := New(cfg, broker, tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := engine.AnalyzeRepository(ctx)

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "git analysis is disabled")
}

func TestEngine_CalculateAuthorConcentration(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	_, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine, err := New(nil, broker, tmpDir)
	require.NoError(t, err)

	tests := []struct {
		name        string
		authors     map[string]int
		expectedMin float64
		expectedMax float64
	}{
		{
			name:        "empty authors",
			authors:     map[string]int{},
			expectedMin: 0.0,
			expectedMax: 0.0,
		},
		{
			name:        "single author",
			authors:     map[string]int{"alice@example.com": 10},
			expectedMin: 1.0,
			expectedMax: 1.0,
		},
		{
			name: "balanced authors",
			authors: map[string]int{
				"alice@example.com": 10,
				"bob@example.com":   10,
			},
			expectedMin: 0.5,
			expectedMax: 0.5,
		},
		{
			name: "concentrated changes",
			authors: map[string]int{
				"alice@example.com": 8,
				"bob@example.com":   2,
			},
			expectedMin: 0.8,
			expectedMax: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := engine.calculateAuthorConcentration(tt.authors)
			require.GreaterOrEqual(t, score, tt.expectedMin)
			require.LessOrEqual(t, score, tt.expectedMax)
		})
	}
}

func TestEngine_CategorizeChangeFrequency(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	_, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine, err := New(nil, broker, tmpDir)
	require.NoError(t, err)

	tests := []struct {
		name          string
		changeCount   int
		totalCommits  int
		expected      string
	}{
		{
			name:         "hot file",
			changeCount:  15,
			totalCommits: 100,
			expected:     "hot", // 15/100 = 0.15 > 0.1
		},
		{
			name:         "warm file",
			changeCount:  7,
			totalCommits: 100,
			expected:     "warm", // 7/100 = 0.07 > 0.05 but < 0.1
		},
		{
			name:         "cold file",
			changeCount:  2,
			totalCommits: 100,
			expected:     "cold", // 2/100 = 0.02 < 0.05
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := engine.categorizeChangeFrequency(tt.changeCount, tt.totalCommits)
			require.Equal(t, tt.expected, category)
		})
	}
}

func TestEngine_GetShortMessage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	_, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine, err := New(nil, broker, tmpDir)
	require.NoError(t, err)

	tests := []struct {
		name        string
		fullMessage string
		expected    string
	}{
		{
			name:        "single line message",
			fullMessage: "Add new feature",
			expected:    "Add new feature",
		},
		{
			name:        "multi-line message",
			fullMessage: "Add new feature\n\nThis is a detailed description\nof the changes made.",
			expected:    "Add new feature",
		},
		{
			name:        "empty message",
			fullMessage: "",
			expected:    "",
		},
		{
			name:        "whitespace only",
			fullMessage: "   \n   \n   ",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.getShortMessage(tt.fullMessage)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestEngine_GetStats(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	_, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine, err := New(nil, broker, tmpDir)
	require.NoError(t, err)

	// Initially empty stats
	stats := engine.GetStats()
	require.Equal(t, int64(0), stats.RepositoriesAnalyzed)
	require.Equal(t, int64(0), stats.CommitsProcessed)
	require.Equal(t, int64(0), stats.HotspotsDetected)
}

func TestEngine_GetConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	_, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	cfg := &config.GitAnalysisConfig{
		Enabled:           true,
		MaxCommits:        500,
		HotspotWindowDays: 60,
		MinHotspotChanges: 3,
	}

	engine, err := New(cfg, broker, tmpDir)
	require.NoError(t, err)

	returnedCfg := engine.GetConfig()
	require.Equal(t, cfg, returnedCfg)
}

func TestEngine_IsEnabled(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	_, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	// Enabled engine
	enabledCfg := &config.GitAnalysisConfig{Enabled: true}
	enabledEngine, err := New(enabledCfg, broker, tmpDir)
	require.NoError(t, err)
	require.True(t, enabledEngine.IsEnabled())

	// Disabled engine
	disabledCfg := &config.GitAnalysisConfig{Enabled: false}
	disabledEngine, err := New(disabledCfg, broker, tmpDir)
	require.NoError(t, err)
	require.False(t, disabledEngine.IsEnabled())
}

func TestEngine_GetRepositoryPath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	_, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	engine, err := New(nil, broker, tmpDir)
	require.NoError(t, err)

	require.Equal(t, tmpDir, engine.GetRepositoryPath())
}

// Helper functions

// createTestRepositoryWithCommits creates a test repository with some commits
func createTestRepositoryWithCommits(t *testing.T) string {
	tmpDir := t.TempDir()

	// Initialize git repository
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	// Create and commit some files
	files := []struct {
		name    string
		content string
	}{
		{"main.go", "package main\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}"},
		{"utils.go", "package main\n\nfunc helper() {\n\t// utility function\n}"},
		{"README.md", "# Test Project\n\nThis is a test project."},
	}

	for i, file := range files {
		// Write file
		filePath := filepath.Join(tmpDir, file.name)
		err = os.WriteFile(filePath, []byte(file.content), 0o644)
		require.NoError(t, err)

		// Add file to git
		_, err = worktree.Add(file.name)
		require.NoError(t, err)

		// Commit
		commitMsg := "Add " + file.name
		if i > 0 {
			commitMsg = "Update " + file.name
		}

		_, err = worktree.Commit(commitMsg, &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test Author",
				Email: "test@example.com",
				When:  time.Now().Add(-time.Duration(len(files)-i) * time.Hour),
			},
		})
		require.NoError(t, err)

		// Make additional changes to main.go to create a hotspot
		if file.name == "main.go" && i == 0 {
			// Make several more commits to main.go
			for j := 1; j <= 3; j++ {
				newContent := file.content + "\n// Additional change " + string(rune('0'+j))
				err = os.WriteFile(filePath, []byte(newContent), 0o644)
				require.NoError(t, err)

				_, err = worktree.Add(file.name)
				require.NoError(t, err)

				_, err = worktree.Commit("Update main.go - change "+string(rune('0'+j)), &git.CommitOptions{
					Author: &object.Signature{
						Name:  "Test Author",
						Email: "test@example.com",
						When:  time.Now().Add(-time.Duration(j) * time.Minute),
					},
				})
				require.NoError(t, err)
			}
		}
	}

	return tmpDir
}

func BenchmarkEngine_AnalyzeRepository(b *testing.B) {
	tmpDir := createBenchmarkRepository(b)

	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	cfg := &config.GitAnalysisConfig{
		Enabled:           true,
		MaxCommits:        100,
		HotspotWindowDays: 30,
		MinHotspotChanges: 1,
	}

	engine, err := New(cfg, broker, tmpDir)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.AnalyzeRepository(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// createBenchmarkRepository creates a repository with more commits for benchmarking
func createBenchmarkRepository(b *testing.B) string {
	tmpDir := b.TempDir()

	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		b.Fatal(err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		b.Fatal(err)
	}

	// Create initial file
	filePath := filepath.Join(tmpDir, "main.go")
	content := "package main\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}"
	err = os.WriteFile(filePath, []byte(content), 0o644)
	if err != nil {
		b.Fatal(err)
	}

	// Make many commits
	for i := 0; i < 50; i++ {
		// Update content
		newContent := content + "\n// Change " + string(rune('0'+i%10))
		err = os.WriteFile(filePath, []byte(newContent), 0o644)
		if err != nil {
			b.Fatal(err)
		}

		_, err = worktree.Add("main.go")
		if err != nil {
			b.Fatal(err)
		}

		_, err = worktree.Commit("Commit "+string(rune('0'+i%10)), &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Benchmark Author",
				Email: "bench@example.com",
				When:  time.Now().Add(-time.Duration(50-i) * time.Minute),
			},
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	return tmpDir
}