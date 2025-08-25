package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextEngineConfig_DefaultValues(t *testing.T) {
	// Test that default values are set correctly
	cfg := &Config{}
	cfg.setDefaults("test-dir", "test-data")

	require.NotNil(t, cfg.Options)
	require.NotNil(t, cfg.Options.ContextEngine)

	// Verify default values
	assert.Equal(t, ContextEngineModePerformance, cfg.Options.ContextEngine.Mode)
	assert.Equal(t, 0, cfg.Options.ContextEngine.MaxGoroutines)
	assert.Equal(t, 0, cfg.Options.ContextEngine.MemoryBudgetMB)
	assert.Equal(t, 10, cfg.Options.ContextEngine.PrewarmTopN)
	assert.Nil(t, cfg.Options.ContextEngine.VectorDB)
}

func TestContextEngineConfig_RepositoryLevelOverride(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "context-engine-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create config directory
	configDir := filepath.Join(tempDir, "config")
	err = os.MkdirAll(configDir, 0755)
	require.NoError(t, err)

	// Create context_engine.json file
	contextEngineConfig := ContextEngineConfig{
		Mode:           ContextEngineModeEco,
		MaxGoroutines:  16,
		MemoryBudgetMB: 1024,
		PrewarmTopN:    20,
		VectorDB: &VectorDBConfig{
			Type:       "qdrant",
			URL:        "http://localhost:6333",
			Collection: "test_collection",
		},
	}

	configData, err := json.Marshal(contextEngineConfig)
	require.NoError(t, err)

	configPath := filepath.Join(configDir, "context_engine.json")
	err = os.WriteFile(configPath, configData, 0644)
	require.NoError(t, err)

	// Test configuration loading
	cfg := &Config{
		Options: &Options{},
	}
	cfg.setDefaults(tempDir, "test-data")

	// Verify that repository config overrode defaults
	require.NotNil(t, cfg.Options.ContextEngine)
	assert.Equal(t, ContextEngineModeEco, cfg.Options.ContextEngine.Mode)
	assert.Equal(t, 16, cfg.Options.ContextEngine.MaxGoroutines)
	assert.Equal(t, 1024, cfg.Options.ContextEngine.MemoryBudgetMB)
	assert.Equal(t, 20, cfg.Options.ContextEngine.PrewarmTopN)

	require.NotNil(t, cfg.Options.ContextEngine.VectorDB)
	assert.Equal(t, "qdrant", cfg.Options.ContextEngine.VectorDB.Type)
	assert.Equal(t, "http://localhost:6333", cfg.Options.ContextEngine.VectorDB.URL)
	assert.Equal(t, "test_collection", cfg.Options.ContextEngine.VectorDB.Collection)
}

func TestContextEngineConfig_PartialOverride(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "context-engine-partial-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create config directory
	configDir := filepath.Join(tempDir, "config")
	err = os.MkdirAll(configDir, 0755)
	require.NoError(t, err)

	// Create context_engine.json file with only some fields
	partialConfig := map[string]interface{}{
		"mode":           "on_demand",
		"max_goroutines": 8,
		// Intentionally omit memory_budget_mb and prewarm_top_n
	}

	configData, err := json.Marshal(partialConfig)
	require.NoError(t, err)

	configPath := filepath.Join(configDir, "context_engine.json")
	err = os.WriteFile(configPath, configData, 0644)
	require.NoError(t, err)

	// Test configuration loading
	cfg := &Config{
		Options: &Options{},
	}
	cfg.setDefaults(tempDir, "test-data")

	// Verify that only specified fields are overridden
	require.NotNil(t, cfg.Options.ContextEngine)
	assert.Equal(t, ContextEngineModeOnDemand, cfg.Options.ContextEngine.Mode) // Overridden
	assert.Equal(t, 8, cfg.Options.ContextEngine.MaxGoroutines)                // Overridden
	assert.Equal(t, 0, cfg.Options.ContextEngine.MemoryBudgetMB)               // Default (not overridden)
	assert.Equal(t, 10, cfg.Options.ContextEngine.PrewarmTopN)                 // Default (not overridden)
	assert.Nil(t, cfg.Options.ContextEngine.VectorDB)                          // Default (not overridden)
}

func TestContextEngineConfig_InvalidJSON(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "context-engine-invalid-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create config directory
	configDir := filepath.Join(tempDir, "config")
	err = os.MkdirAll(configDir, 0755)
	require.NoError(t, err)

	// Create invalid JSON file
	invalidJSON := `{"mode": "performance", "max_goroutines": "invalid"}`
	configPath := filepath.Join(configDir, "context_engine.json")
	err = os.WriteFile(configPath, []byte(invalidJSON), 0644)
	require.NoError(t, err)

	// Test configuration loading (should not crash and use defaults)
	cfg := &Config{
		Options: &Options{},
	}
	cfg.setDefaults(tempDir, "test-data")

	// Should fall back to defaults when JSON is invalid
	require.NotNil(t, cfg.Options.ContextEngine)
	assert.Equal(t, ContextEngineModePerformance, cfg.Options.ContextEngine.Mode)
	assert.Equal(t, 0, cfg.Options.ContextEngine.MaxGoroutines)
}

func TestContextEngineConfig_MissingFile(t *testing.T) {
	// Create a temporary directory without config file
	tempDir, err := os.MkdirTemp("", "context-engine-missing-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Test configuration loading (should use defaults)
	cfg := &Config{
		Options: &Options{},
	}
	cfg.setDefaults(tempDir, "test-data")

	// Should use all defaults when file is missing
	require.NotNil(t, cfg.Options.ContextEngine)
	assert.Equal(t, ContextEngineModePerformance, cfg.Options.ContextEngine.Mode)
	assert.Equal(t, 0, cfg.Options.ContextEngine.MaxGoroutines)
	assert.Equal(t, 0, cfg.Options.ContextEngine.MemoryBudgetMB)
	assert.Equal(t, 10, cfg.Options.ContextEngine.PrewarmTopN)
	assert.Nil(t, cfg.Options.ContextEngine.VectorDB)
}

func TestContextEngineMode_Values(t *testing.T) {
	// Test that mode constants are properly defined
	assert.Equal(t, ContextEngineMode("performance"), ContextEngineModePerformance)
	assert.Equal(t, ContextEngineMode("on_demand"), ContextEngineModeOnDemand)
	assert.Equal(t, ContextEngineMode("eco"), ContextEngineModeEco)
}

func TestVectorDBConfig_Structure(t *testing.T) {
	// Test VectorDBConfig structure
	vectorConfig := &VectorDBConfig{
		Type:       "qdrant",
		URL:        "http://localhost:6333",
		APIKey:     "test-key",
		Collection: "test-collection",
	}

	// Verify all fields are accessible
	assert.Equal(t, "qdrant", vectorConfig.Type)
	assert.Equal(t, "http://localhost:6333", vectorConfig.URL)
	assert.Equal(t, "test-key", vectorConfig.APIKey)
	assert.Equal(t, "test-collection", vectorConfig.Collection)
}

func TestContextEngineConfig_JSONMarshalUnmarshal(t *testing.T) {
	// Test JSON marshaling and unmarshaling
	original := &ContextEngineConfig{
		Mode:           ContextEngineModeEco,
		MaxGoroutines:  12,
		MemoryBudgetMB: 2048,
		PrewarmTopN:    25,
		VectorDB: &VectorDBConfig{
			Type:       "pinecone",
			URL:        "https://pinecone.example.com",
			APIKey:     "secret-key",
			Collection: "my-collection",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal back
	var unmarshaled ContextEngineConfig
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, original.Mode, unmarshaled.Mode)
	assert.Equal(t, original.MaxGoroutines, unmarshaled.MaxGoroutines)
	assert.Equal(t, original.MemoryBudgetMB, unmarshaled.MemoryBudgetMB)
	assert.Equal(t, original.PrewarmTopN, unmarshaled.PrewarmTopN)

	require.NotNil(t, unmarshaled.VectorDB)
	assert.Equal(t, original.VectorDB.Type, unmarshaled.VectorDB.Type)
	assert.Equal(t, original.VectorDB.URL, unmarshaled.VectorDB.URL)
	assert.Equal(t, original.VectorDB.APIKey, unmarshaled.VectorDB.APIKey)
	assert.Equal(t, original.VectorDB.Collection, unmarshaled.VectorDB.Collection)
}

func TestContextEngineConfig_ZeroValueOverrides(t *testing.T) {
	// Test that zero values are properly handled (not overridden)
	tempDir, err := os.MkdirTemp("", "context-engine-zero-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create config directory
	configDir := filepath.Join(tempDir, "config")
	err = os.MkdirAll(configDir, 0755)
	require.NoError(t, err)

	// Create config with zero values (should not override defaults)
	zeroConfig := map[string]interface{}{
		"mode":             "", // Empty string
		"max_goroutines":   0,  // Zero
		"memory_budget_mb": 0,  // Zero
		"prewarm_top_n":    0,  // Zero
	}

	configData, err := json.Marshal(zeroConfig)
	require.NoError(t, err)

	configPath := filepath.Join(configDir, "context_engine.json")
	err = os.WriteFile(configPath, configData, 0644)
	require.NoError(t, err)

	// Test configuration loading
	cfg := &Config{
		Options: &Options{},
	}
	cfg.setDefaults(tempDir, "test-data")

	// Zero values should NOT override defaults (conservative merging)
	require.NotNil(t, cfg.Options.ContextEngine)
	assert.Equal(t, ContextEngineModePerformance, cfg.Options.ContextEngine.Mode) // Default kept
	assert.Equal(t, 0, cfg.Options.ContextEngine.MaxGoroutines)                   // Default kept (also 0)
	assert.Equal(t, 0, cfg.Options.ContextEngine.MemoryBudgetMB)                  // Default kept (also 0)
	assert.Equal(t, 10, cfg.Options.ContextEngine.PrewarmTopN)                    // Default kept
}
