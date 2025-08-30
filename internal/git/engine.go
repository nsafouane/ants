// Package git provides Git repository analysis capabilities
// for the Context Engine's Tier 1 structural analysis.
package git

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// FileHotspot represents a file with high change frequency and potential risk.
type FileHotspot struct {
	Path                string    `json:"path"`
	ChangeFrequency     int       `json:"change_frequency"`
	AuthorCount         int       `json:"author_count"`
	LinesAdded          int       `json:"lines_added"`
	LinesDeleted        int       `json:"lines_deleted"`
	LastChange          time.Time `json:"last_change"`
	FirstChange         time.Time `json:"first_change"`
	HotspotScore        float64   `json:"hotspot_score"`
	ConcentrationRisk   bool      `json:"concentration_risk"`
	ConcentrationScore  float64   `json:"concentration_score"`
	ChangeCategory      string    `json:"change_category"` // hot, warm, cold
	RecentCommits       []CommitInfo `json:"recent_commits,omitempty"`
}

// CommitInfo represents basic information about a commit.
type CommitInfo struct {
	Hash            string    `json:"hash"`
	ShortHash       string    `json:"short_hash"`
	Author          string    `json:"author"`
	AuthorEmail     string    `json:"author_email"`
	Message         string    `json:"message"`
	ShortMessage    string    `json:"short_message"`
	Timestamp       time.Time `json:"timestamp"`
	FilesChanged    int       `json:"files_changed"`
	LinesAdded      int       `json:"lines_added"`
	LinesDeleted    int       `json:"lines_deleted"`
	IsMergeCommit   bool      `json:"is_merge_commit"`
}

// AuthorStats represents contribution statistics for an author.
type AuthorStats struct {
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	CommitCount  int       `json:"commit_count"`
	FilesChanged int       `json:"files_changed"`
	LinesAdded   int       `json:"lines_added"`
	LinesDeleted int       `json:"lines_deleted"`
	FirstCommit  time.Time `json:"first_commit"`
	LastCommit   time.Time `json:"last_commit"`
	IsActive     bool      `json:"is_active"` // Committed within last 30 days
}

// RepositoryStats represents overall repository statistics.
type RepositoryStats struct {
	TotalCommits      int                    `json:"total_commits"`
	TotalAuthors      int                    `json:"total_authors"`
	ActiveAuthors     int                    `json:"active_authors"`
	TotalFiles        int                    `json:"total_files"`
	HotspotFiles      int                    `json:"hotspot_files"`
	AnalysisWindow    time.Duration          `json:"analysis_window"`
	AnalysisStartDate time.Time              `json:"analysis_start_date"`
	AnalysisEndDate   time.Time              `json:"analysis_end_date"`
	TopAuthors        []*AuthorStats         `json:"top_authors"`
	TopHotspots       []*FileHotspot         `json:"top_hotspots"`
	ChangeDistribution map[string]int        `json:"change_distribution"` // hot/warm/cold counts
}

// AnalysisResult represents the complete Git history analysis result.
type AnalysisResult struct {
	RepositoryPath   string           `json:"repository_path"`
	Stats           RepositoryStats   `json:"stats"`
	Hotspots        []*FileHotspot    `json:"hotspots"`
	Authors         []*AuthorStats    `json:"authors"`
	RecentCommits   []*CommitInfo     `json:"recent_commits"`
	AnalysisTime    time.Duration     `json:"analysis_time"`
	ConfigUsed      *config.GitAnalysisConfig `json:"config_used"`
}

// HotspotData holds internal data for hotspot calculation.
type HotspotData struct {
	Path         string
	ChangeCount  int
	LinesAdded   int
	LinesDeleted int
	FirstChange  time.Time
	LastChange   time.Time
	Authors      map[string]int // email -> commit count
	Commits      []CommitInfo
}

// Engine provides Git repository analysis capabilities.
type Engine struct {
	config     *config.GitAnalysisConfig
	broker     *pubsub.Broker[map[string]interface{}]
	repository *git.Repository
	repoPath   string
	logger     *slog.Logger

	// Analysis state
	stats      *AnalysisStats
	statsLock  sync.RWMutex
}

// AnalysisStats tracks Git analysis statistics.
type AnalysisStats struct {
	RepositoriesAnalyzed int64         `json:"repositories_analyzed"`
	CommitsProcessed     int64         `json:"commits_processed"`
	HotspotsDetected     int64         `json:"hotspots_detected"`
	TotalAnalysisTime    time.Duration `json:"total_analysis_time"`
	AverageAnalysisTime  time.Duration `json:"average_analysis_time"`
	LastAnalysisTime     time.Time     `json:"last_analysis_time"`
}

// New creates a new Git analysis engine.
func New(cfg *config.GitAnalysisConfig, broker *pubsub.Broker[map[string]interface{}], repoPath string) (*Engine, error) {
	if cfg == nil {
		cfg = &config.GitAnalysisConfig{
			Enabled:           true,
			MaxCommits:        1000,
			HotspotWindowDays: 90,
			MinHotspotChanges: 5,
		}
	}

	engine := &Engine{
		config:   cfg,
		broker:   broker,
		repoPath: repoPath,
		logger:   slog.With("component", "git_engine"),
		stats:    &AnalysisStats{},
	}

	// Open the repository
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository at %s: %w", repoPath, err)
	}

	engine.repository = repo
	
	engine.logger.Info("Git analysis engine initialized", 
		"repo_path", repoPath,
		"max_commits", cfg.MaxCommits,
		"hotspot_window_days", cfg.HotspotWindowDays)

	return engine, nil
}

// AnalyzeRepository performs comprehensive Git history analysis.
func (e *Engine) AnalyzeRepository(ctx context.Context) (*AnalysisResult, error) {
	if !e.config.Enabled {
		return nil, fmt.Errorf("git analysis is disabled")
	}

	startTime := time.Now()
	
	e.logger.Info("Starting Git repository analysis", "repo_path", e.repoPath)

	result := &AnalysisResult{
		RepositoryPath: e.repoPath,
		ConfigUsed:     e.config,
	}

	// Analyze commits within the configured time window
	cutoffDate := time.Now().AddDate(0, 0, -e.config.HotspotWindowDays)
	
	commits, err := e.getCommitsInWindow(ctx, cutoffDate, e.config.MaxCommits)
	if err != nil {
		return nil, fmt.Errorf("failed to get commits: %w", err)
	}

	e.logger.Debug("Retrieved commits for analysis", "commit_count", len(commits))

	// Analyze hotspots
	hotspots, err := e.analyzeHotspots(ctx, commits, cutoffDate)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze hotspots: %w", err)
	}

	// Analyze authors
	authors := e.analyzeAuthors(commits, cutoffDate)

	// Build repository statistics
	stats := e.buildRepositoryStats(commits, hotspots, authors, cutoffDate)

	// Fill result
	result.Stats = stats
	result.Hotspots = hotspots
	result.Authors = authors
	result.RecentCommits = commits[:min(len(commits), 20)] // Top 20 recent commits
	result.AnalysisTime = time.Since(startTime)

	// Update engine statistics
	e.updateStats(result)

	e.logger.Info("Git repository analysis completed",
		"analysis_time", result.AnalysisTime,
		"commits_analyzed", len(commits),
		"hotspots_found", len(hotspots),
		"authors_found", len(authors))

	// Publish analysis completed event
	if e.broker != nil {
		e.broker.Publish(pubsub.GitAnalysisCompletedEvent, map[string]interface{}{
			"repo_path":      e.repoPath,
			"analysis_time":  result.AnalysisTime,
			"hotspots_count": len(hotspots),
			"commits_count":  len(commits),
		})
	}

	return result, nil
}

// getCommitsInWindow retrieves commits within the specified time window.
func (e *Engine) getCommitsInWindow(ctx context.Context, cutoffDate time.Time, maxCommits int) ([]*CommitInfo, error) {
	commitIter, err := e.repository.Log(&git.LogOptions{
		Order: git.LogOrderCommitterTime,
		All:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log: %w", err)
	}
	defer commitIter.Close()

	var commits []*CommitInfo
	count := 0

	err = commitIter.ForEach(func(commit *object.Commit) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if commit is within time window
		if commit.Committer.When.Before(cutoffDate) {
			return fmt.Errorf("reached cutoff date") // Stop iteration
		}

		// Check max commits limit
		if count >= maxCommits {
			return fmt.Errorf("reached max commits limit") // Stop iteration
		}

		// Get commit stats
		stats, err := commit.Stats()
		if err != nil {
			e.logger.Warn("Failed to get commit stats", "commit_hash", commit.Hash, "error", err)
			stats = object.FileStats{} // Empty stats
		}

		// Calculate totals
		var filesChanged, linesAdded, linesDeleted int
		for _, stat := range stats {
			filesChanged++
			linesAdded += stat.Addition
			linesDeleted += stat.Deletion
		}

		commitInfo := &CommitInfo{
			Hash:          commit.Hash.String(),
			ShortHash:     commit.Hash.String()[:8],
			Author:        commit.Author.Name,
			AuthorEmail:   commit.Author.Email,
			Message:       commit.Message,
			ShortMessage:  e.getShortMessage(commit.Message),
			Timestamp:     commit.Committer.When,
			FilesChanged:  filesChanged,
			LinesAdded:    linesAdded,
			LinesDeleted:  linesDeleted,
			IsMergeCommit: len(commit.ParentHashes) > 1,
		}

		commits = append(commits, commitInfo)
		count++
		return nil
	})

	// Handle expected errors (reached limits)
	if err != nil && err.Error() != "reached cutoff date" && err.Error() != "reached max commits limit" && err != ctx.Err() {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return commits, nil
}

// analyzeHotspots identifies files with high change frequency.
func (e *Engine) analyzeHotspots(ctx context.Context, commits []*CommitInfo, cutoffDate time.Time) ([]*FileHotspot, error) {
	fileData := make(map[string]*HotspotData)

	// Process each commit to gather file change data
	for _, commitInfo := range commits {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Get the actual commit object
		hash := plumbing.NewHash(commitInfo.Hash)
		commit, err := e.repository.CommitObject(hash)
		if err != nil {
			e.logger.Warn("Failed to get commit object", "hash", commitInfo.Hash, "error", err)
			continue
		}

		// Get commit stats
		stats, err := commit.Stats()
		if err != nil {
			e.logger.Warn("Failed to get commit stats", "hash", commitInfo.Hash, "error", err)
			continue
		}

		// Process each file in the commit
		for _, fileStat := range stats {
			data := fileData[fileStat.Name]
			if data == nil {
				data = &HotspotData{
					Path:        fileStat.Name,
					FirstChange: commitInfo.Timestamp,
					LastChange:  commitInfo.Timestamp,
					Authors:     make(map[string]int),
					Commits:     make([]CommitInfo, 0),
				}
				fileData[fileStat.Name] = data
			}

			// Update change data
			data.ChangeCount++
			data.LinesAdded += fileStat.Addition
			data.LinesDeleted += fileStat.Deletion

			// Update time bounds
			if commitInfo.Timestamp.After(data.LastChange) {
				data.LastChange = commitInfo.Timestamp
			}
			if commitInfo.Timestamp.Before(data.FirstChange) {
				data.FirstChange = commitInfo.Timestamp
			}

			// Track author contributions
			data.Authors[commitInfo.AuthorEmail]++

			// Add commit info (limit to recent commits)
			if len(data.Commits) < 10 {
				data.Commits = append(data.Commits, *commitInfo)
			}
		}
	}

	// Convert to hotspots and calculate scores
	var hotspots []*FileHotspot
	for path, data := range fileData {
		// Skip files with insufficient changes
		if data.ChangeCount < e.config.MinHotspotChanges {
			continue
		}

		hotspot := &FileHotspot{
			Path:            path,
			ChangeFrequency: data.ChangeCount,
			AuthorCount:     len(data.Authors),
			LinesAdded:      data.LinesAdded,
			LinesDeleted:    data.LinesDeleted,
			FirstChange:     data.FirstChange,
			LastChange:      data.LastChange,
			RecentCommits:   data.Commits,
		}

		// Calculate concentration score
		hotspot.ConcentrationScore = e.calculateAuthorConcentration(data.Authors)
		hotspot.ConcentrationRisk = hotspot.ConcentrationScore > 0.8

		// Calculate hotspot score
		hotspot.HotspotScore = e.calculateHotspotScore(data, hotspot.ConcentrationScore, cutoffDate)

		// Categorize change frequency
		hotspot.ChangeCategory = e.categorizeChangeFrequency(data.ChangeCount, len(commits))

		hotspots = append(hotspots, hotspot)
	}

	// Sort by hotspot score (descending)
	sort.Slice(hotspots, func(i, j int) bool {
		return hotspots[i].HotspotScore > hotspots[j].HotspotScore
	})

	return hotspots, nil
}

// analyzeAuthors analyzes author contribution patterns.
func (e *Engine) analyzeAuthors(commits []*CommitInfo, cutoffDate time.Time) []*AuthorStats {
	authorData := make(map[string]*AuthorStats)

	// Process commits to gather author data
	for _, commit := range commits {
		author := authorData[commit.AuthorEmail]
		if author == nil {
			author = &AuthorStats{
				Name:        commit.Author,
				Email:       commit.AuthorEmail,
				FirstCommit: commit.Timestamp,
				LastCommit:  commit.Timestamp,
			}
			authorData[commit.AuthorEmail] = author
		}

		// Update stats
		author.CommitCount++
		author.FilesChanged += commit.FilesChanged
		author.LinesAdded += commit.LinesAdded
		author.LinesDeleted += commit.LinesDeleted

		// Update time bounds
		if commit.Timestamp.After(author.LastCommit) {
			author.LastCommit = commit.Timestamp
		}
		if commit.Timestamp.Before(author.FirstCommit) {
			author.FirstCommit = commit.Timestamp
		}

		// Check if author is active (committed within last 30 days)
		if time.Since(commit.Timestamp) <= 30*24*time.Hour {
			author.IsActive = true
		}
	}

	// Convert to slice and sort by commit count
	authors := make([]*AuthorStats, 0, len(authorData))
	for _, author := range authorData {
		authors = append(authors, author)
	}

	sort.Slice(authors, func(i, j int) bool {
		return authors[i].CommitCount > authors[j].CommitCount
	})

	return authors
}

// buildRepositoryStats builds comprehensive repository statistics.
func (e *Engine) buildRepositoryStats(commits []*CommitInfo, hotspots []*FileHotspot, authors []*AuthorStats, cutoffDate time.Time) RepositoryStats {
	stats := RepositoryStats{
		TotalCommits:      len(commits),
		TotalAuthors:      len(authors),
		TotalFiles:        0, // Will be calculated from hotspots
		HotspotFiles:      len(hotspots),
		AnalysisWindow:    time.Since(cutoffDate),
		AnalysisStartDate: cutoffDate,
		AnalysisEndDate:   time.Now(),
		ChangeDistribution: make(map[string]int),
	}

	// Count active authors
	for _, author := range authors {
		if author.IsActive {
			stats.ActiveAuthors++
		}
	}

	// Get top authors (up to 10)
	topAuthorCount := min(len(authors), 10)
	stats.TopAuthors = authors[:topAuthorCount]

	// Get top hotspots (up to 20)
	topHotspotCount := min(len(hotspots), 20)
	stats.TopHotspots = hotspots[:topHotspotCount]

	// Calculate change distribution
	for _, hotspot := range hotspots {
		stats.ChangeDistribution[hotspot.ChangeCategory]++
	}

	// Calculate total files from unique file paths in hotspots
	fileSet := make(map[string]bool)
	for _, hotspot := range hotspots {
		fileSet[hotspot.Path] = true
	}
	stats.TotalFiles = len(fileSet)

	return stats
}

// calculateAuthorConcentration calculates how concentrated changes are among authors.
func (e *Engine) calculateAuthorConcentration(authors map[string]int) float64 {
	if len(authors) == 0 {
		return 0.0
	}

	totalChanges := 0
	maxChanges := 0

	for _, changes := range authors {
		totalChanges += changes
		if changes > maxChanges {
			maxChanges = changes
		}
	}

	if totalChanges == 0 {
		return 0.0
	}

	return float64(maxChanges) / float64(totalChanges)
}

// calculateHotspotScore calculates a composite hotspot score.
func (e *Engine) calculateHotspotScore(data *HotspotData, concentrationScore float64, cutoffDate time.Time) float64 {
	// Base score from change frequency
	changeScore := float64(data.ChangeCount)

	// Recency factor (more recent changes = higher score)
	daysSinceLastChange := time.Since(data.LastChange).Hours() / 24
	recencyFactor := 1.0
	if daysSinceLastChange < 7 {
		recencyFactor = 2.0
	} else if daysSinceLastChange < 30 {
		recencyFactor = 1.5
	}

	// Author concentration factor (high concentration = higher risk)
	concentrationFactor := 1.0 + concentrationScore

	// Code churn factor (high churn = higher risk)
	churnFactor := 1.0
	if data.LinesAdded+data.LinesDeleted > 1000 {
		churnFactor = 1.5
	}

	return changeScore * recencyFactor * concentrationFactor * churnFactor
}

// categorizeChangeFrequency categorizes files by change frequency.
func (e *Engine) categorizeChangeFrequency(changeCount, totalCommits int) string {
	changeRatio := float64(changeCount) / float64(totalCommits)

	if changeRatio > 0.1 {
		return "hot"
	} else if changeRatio > 0.05 {
		return "warm"
	}
	return "cold"
}

// getShortMessage extracts the first line of a commit message.
func (e *Engine) getShortMessage(fullMessage string) string {
	lines := strings.Split(strings.TrimSpace(fullMessage), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}

// GetCommitContext retrieves recent commits affecting a specific file.
func (e *Engine) GetCommitContext(ctx context.Context, filePath string, limit int) ([]*CommitInfo, error) {
	if limit <= 0 {
		limit = 10
	}

	commitIter, err := e.repository.Log(&git.LogOptions{
		FileName: &filePath,
		Order:    git.LogOrderCommitterTime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log for file %s: %w", filePath, err)
	}
	defer commitIter.Close()

	var commits []*CommitInfo
	count := 0

	err = commitIter.ForEach(func(commit *object.Commit) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if count >= limit {
			return fmt.Errorf("reached limit") // Stop iteration
		}

		// Get commit stats for the specific file
		stats, err := commit.Stats()
		if err != nil {
			e.logger.Warn("Failed to get commit stats", "commit_hash", commit.Hash, "error", err)
			stats = object.FileStats{} // Empty stats
		}

		// Find stats for the specific file
		var linesAdded, linesDeleted int
		for _, stat := range stats {
			if stat.Name == filePath {
				linesAdded = stat.Addition
				linesDeleted = stat.Deletion
				break
			}
		}

		commitInfo := &CommitInfo{
			Hash:          commit.Hash.String(),
			ShortHash:     commit.Hash.String()[:8],
			Author:        commit.Author.Name,
			AuthorEmail:   commit.Author.Email,
			Message:       commit.Message,
			ShortMessage:  e.getShortMessage(commit.Message),
			Timestamp:     commit.Committer.When,
			FilesChanged:  1, // We're looking at one specific file
			LinesAdded:    linesAdded,
			LinesDeleted:  linesDeleted,
			IsMergeCommit: len(commit.ParentHashes) > 1,
		}

		commits = append(commits, commitInfo)
		count++
		return nil
	})

	// Handle expected error (reached limit)
	if err != nil && err.Error() != "reached limit" && err != ctx.Err() {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return commits, nil
}

// GetHotspotFiles returns the top N hotspot files.
func (e *Engine) GetHotspotFiles(ctx context.Context, limit int) ([]*FileHotspot, error) {
	result, err := e.AnalyzeRepository(ctx)
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > len(result.Hotspots) {
		return result.Hotspots, nil
	}

	return result.Hotspots[:limit], nil
}

// IsGitRepository checks if the given path is a Git repository.
func (e *Engine) IsGitRepository() bool {
	_, err := git.PlainOpen(e.repoPath)
	return err == nil
}

// updateStats updates the engine's analysis statistics.
func (e *Engine) updateStats(result *AnalysisResult) {
	e.statsLock.Lock()
	defer e.statsLock.Unlock()

	e.stats.RepositoriesAnalyzed++
	e.stats.CommitsProcessed += int64(len(result.RecentCommits))
	e.stats.HotspotsDetected += int64(len(result.Hotspots))
	e.stats.TotalAnalysisTime += result.AnalysisTime
	e.stats.LastAnalysisTime = time.Now()

	// Calculate average analysis time
	if e.stats.RepositoriesAnalyzed > 0 {
		e.stats.AverageAnalysisTime = e.stats.TotalAnalysisTime / time.Duration(e.stats.RepositoriesAnalyzed)
	}
}

// GetStats returns the current analysis statistics.
func (e *Engine) GetStats() AnalysisStats {
	e.statsLock.RLock()
	defer e.statsLock.RUnlock()
	return *e.stats
}

// IsEnabled returns true if Git analysis is enabled.
func (e *Engine) IsEnabled() bool {
	return e.config.Enabled
}

// GetConfig returns the engine's configuration.
func (e *Engine) GetConfig() *config.GitAnalysisConfig {
	return e.config
}

// GetRepositoryPath returns the repository path.
func (e *Engine) GetRepositoryPath() string {
	return e.repoPath
}

// Helper functions

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}


