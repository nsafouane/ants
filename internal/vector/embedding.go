package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/log"
)

// EmbeddingService provides embedding generation capabilities using Ollama.
type EmbeddingService struct {
	baseURL    string
	model      string
	httpClient *http.Client
	timeout    time.Duration
}

// EmbeddingServiceConfig configures the embedding service.
type EmbeddingServiceConfig struct {
	BaseURL string        `json:"base_url"` // Ollama base URL (default: http://localhost:11434)
	Model   string        `json:"model"`    // Embedding model (default: all-minilm)
	Timeout time.Duration `json:"timeout"`  // Request timeout (default: 30s)
}

// OllamaEmbedRequest represents the request to Ollama's /api/embed endpoint.
type OllamaEmbedRequest struct {
	Model     string      `json:"model"`
	Input     interface{} `json:"input"`      // Can be string or []string
	Truncate  bool        `json:"truncate"`   // Truncate input to fit context
	KeepAlive string      `json:"keep_alive"` // How long to keep model loaded
}

// OllamaEmbedResponse represents the response from Ollama's /api/embed endpoint.
type OllamaEmbedResponse struct {
	Model           string      `json:"model"`
	Embeddings      [][]float32 `json:"embeddings"`
	TotalDuration   int64       `json:"total_duration"`
	LoadDuration    int64       `json:"load_duration"`
	PromptEvalCount int         `json:"prompt_eval_count"`
}

// DefaultEmbeddingServiceConfig returns sensible defaults for the embedding service.
func DefaultEmbeddingServiceConfig() *EmbeddingServiceConfig {
	return &EmbeddingServiceConfig{
		BaseURL: "http://localhost:11434",
		Model:   "all-minilm",
		Timeout: 30 * time.Second,
	}
}

// NewEmbeddingService creates a new embedding service with the given configuration.
func NewEmbeddingService(cfg *EmbeddingServiceConfig) *EmbeddingService {
	if cfg == nil {
		cfg = DefaultEmbeddingServiceConfig()
	}

	// Set defaults for missing values
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434"
	}
	if cfg.Model == "" {
		cfg.Model = "all-minilm"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	httpClient := log.NewHTTPClient()
	httpClient.Timeout = cfg.Timeout

	return &EmbeddingService{
		baseURL:    cfg.BaseURL,
		model:      cfg.Model,
		httpClient: httpClient,
		timeout:    cfg.Timeout,
	}
}

// NewEmbeddingServiceFromConfig creates an embedding service from the application configuration.
func NewEmbeddingServiceFromConfig(cfg *config.Config) *EmbeddingService {
	// For now, use defaults. Later this can be extended to read from config
	embeddingConfig := DefaultEmbeddingServiceConfig()

	// TODO: Add embedding service configuration to config.json
	// if cfg.Options.ContextEngine.EmbeddingService != nil {
	//     embeddingConfig = cfg.Options.ContextEngine.EmbeddingService
	// }

	return NewEmbeddingService(embeddingConfig)
}

// GenerateEmbedding generates a single embedding for the given text.
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	embeddings, err := s.GenerateEmbeddings(ctx, []string{text})
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return embeddings[0], nil
}

// GenerateEmbeddings generates embeddings for multiple texts in a single request.
func (s *EmbeddingService) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("texts cannot be empty")
	}

	// Prepare request
	request := OllamaEmbedRequest{
		Model:     s.model,
		Input:     texts,
		Truncate:  true, // Truncate to fit context
		KeepAlive: "5m", // Keep model loaded for 5 minutes
	}

	// Serialize request
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/embed", s.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API returned status %d", resp.StatusCode)
	}

	// Parse response
	var embedResponse OllamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return embedResponse.Embeddings, nil
}

// GenerateCodeEmbedding generates an embedding for code content with metadata context.
func (s *EmbeddingService) GenerateCodeEmbedding(ctx context.Context, codeContent string, metadata *EmbeddingMetadata) ([]float32, error) {
	if codeContent == "" {
		return nil, fmt.Errorf("code content cannot be empty")
	}

	// Create contextual prompt for better embeddings
	contextualText := s.buildContextualPrompt(codeContent, metadata)

	return s.GenerateEmbedding(ctx, contextualText)
}

// buildContextualPrompt creates a contextual prompt that includes metadata for better embeddings.
func (s *EmbeddingService) buildContextualPrompt(codeContent string, metadata *EmbeddingMetadata) string {
	if metadata == nil {
		return codeContent
	}

	// Build a context-aware prompt that includes metadata
	var prompt bytes.Buffer

	// Add metadata context
	if metadata.Language != "" {
		prompt.WriteString(fmt.Sprintf("Language: %s\n", metadata.Language))
	}
	if metadata.Kind != "" {
		prompt.WriteString(fmt.Sprintf("Type: %s\n", metadata.Kind))
	}
	if metadata.Symbol != "" {
		prompt.WriteString(fmt.Sprintf("Symbol: %s\n", metadata.Symbol))
	}
	if metadata.Path != "" {
		prompt.WriteString(fmt.Sprintf("File: %s\n", metadata.Path))
	}

	// Add summary if available
	if metadata.Summary != "" {
		prompt.WriteString(fmt.Sprintf("Summary: %s\n", metadata.Summary))
	}

	// Add tags if available
	if len(metadata.Tags) > 0 {
		prompt.WriteString(fmt.Sprintf("Tags: %v\n", metadata.Tags))
	}

	prompt.WriteString("\nCode:\n")
	prompt.WriteString(codeContent)

	return prompt.String()
}

// HealthCheck verifies that the Ollama service is accessible and the model is available.
func (s *EmbeddingService) HealthCheck(ctx context.Context) error {
	// Test with a simple embedding request
	_, err := s.GenerateEmbedding(ctx, "test")
	if err != nil {
		return fmt.Errorf("Ollama health check failed: %w", err)
	}
	return nil
}

// GetModel returns the currently configured embedding model.
func (s *EmbeddingService) GetModel() string {
	return s.model
}

// GetBaseURL returns the currently configured Ollama base URL.
func (s *EmbeddingService) GetBaseURL() string {
	return s.baseURL
}

// EmbeddingGenerator defines the interface for embedding generation services.
type EmbeddingGenerator interface {
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
	GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
	GenerateCodeEmbedding(ctx context.Context, codeContent string, metadata *EmbeddingMetadata) ([]float32, error)
	HealthCheck(ctx context.Context) error
	GetModel() string
}

// Ensure EmbeddingService implements EmbeddingGenerator
var _ EmbeddingGenerator = (*EmbeddingService)(nil)
