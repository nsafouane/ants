package vector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultEmbeddingServiceConfig(t *testing.T) {
	t.Parallel()

	config := DefaultEmbeddingServiceConfig()

	require.Equal(t, "http://localhost:11434", config.BaseURL)
	require.Equal(t, "all-minilm", config.Model)
	require.Equal(t, 30*time.Second, config.Timeout)
}

func TestNewEmbeddingService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *EmbeddingServiceConfig
		want   *EmbeddingService
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
			want: &EmbeddingService{
				baseURL: "http://localhost:11434",
				model:   "all-minilm",
				timeout: 30 * time.Second,
			},
		},
		{
			name: "custom config",
			config: &EmbeddingServiceConfig{
				BaseURL: "http://example.com:8080",
				Model:   "custom-model",
				Timeout: 60 * time.Second,
			},
			want: &EmbeddingService{
				baseURL: "http://example.com:8080",
				model:   "custom-model",
				timeout: 60 * time.Second,
			},
		},
		{
			name: "partial config fills defaults",
			config: &EmbeddingServiceConfig{
				Model: "custom-model",
			},
			want: &EmbeddingService{
				baseURL: "http://localhost:11434",
				model:   "custom-model",
				timeout: 30 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := NewEmbeddingService(tt.config)

			require.Equal(t, tt.want.baseURL, service.baseURL)
			require.Equal(t, tt.want.model, service.model)
			require.Equal(t, tt.want.timeout, service.timeout)
			require.NotNil(t, service.httpClient)
			require.Equal(t, tt.want.timeout, service.httpClient.Timeout)
		})
	}
}

func TestEmbeddingService_GenerateEmbedding(t *testing.T) {
	t.Parallel()

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/api/embed", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse request
		var req OllamaEmbedRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		require.Equal(t, "test-model", req.Model)
		require.Equal(t, []string{"test text"}, req.Input)
		require.True(t, req.Truncate)
		require.Equal(t, "5m", req.KeepAlive)

		// Send response
		response := OllamaEmbedResponse{
			Model: "test-model",
			Embeddings: [][]float32{
				{0.1, 0.2, 0.3, 0.4},
			},
			TotalDuration:   1000000,
			LoadDuration:    100000,
			PromptEvalCount: 2,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create service
	config := &EmbeddingServiceConfig{
		BaseURL: server.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}
	service := NewEmbeddingService(config)

	// Test successful embedding generation
	embedding, err := service.GenerateEmbedding(context.Background(), "test text")

	require.NoError(t, err)
	require.Equal(t, []float32{0.1, 0.2, 0.3, 0.4}, embedding)
}

func TestEmbeddingService_GenerateEmbedding_EmptyText(t *testing.T) {
	t.Parallel()

	service := NewEmbeddingService(nil)

	_, err := service.GenerateEmbedding(context.Background(), "")

	require.Error(t, err)
	require.Contains(t, err.Error(), "text cannot be empty")
}

func TestEmbeddingService_GenerateEmbeddings(t *testing.T) {
	t.Parallel()

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request
		var req OllamaEmbedRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		texts := req.Input.([]interface{})
		require.Len(t, texts, 2)
		require.Equal(t, "text1", texts[0])
		require.Equal(t, "text2", texts[1])

		// Send response with multiple embeddings
		response := OllamaEmbedResponse{
			Model: "test-model",
			Embeddings: [][]float32{
				{0.1, 0.2, 0.3},
				{0.4, 0.5, 0.6},
			},
			TotalDuration:   2000000,
			LoadDuration:    200000,
			PromptEvalCount: 4,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create service
	config := &EmbeddingServiceConfig{
		BaseURL: server.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}
	service := NewEmbeddingService(config)

	// Test multiple embeddings generation
	embeddings, err := service.GenerateEmbeddings(context.Background(), []string{"text1", "text2"})

	require.NoError(t, err)
	require.Len(t, embeddings, 2)
	require.Equal(t, []float32{0.1, 0.2, 0.3}, embeddings[0])
	require.Equal(t, []float32{0.4, 0.5, 0.6}, embeddings[1])
}

func TestEmbeddingService_GenerateEmbeddings_EmptyTexts(t *testing.T) {
	t.Parallel()

	service := NewEmbeddingService(nil)

	_, err := service.GenerateEmbeddings(context.Background(), []string{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "texts cannot be empty")
}

func TestEmbeddingService_GenerateCodeEmbedding(t *testing.T) {
	t.Parallel()

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request
		var req OllamaEmbedRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify the contextual prompt was created
		texts := req.Input.([]interface{})
		require.Len(t, texts, 1)
		prompt := texts[0].(string)

		// Should contain metadata and code
		require.Contains(t, prompt, "Language: go")
		require.Contains(t, prompt, "Type: function")
		require.Contains(t, prompt, "Symbol: main")
		require.Contains(t, prompt, "File: /test/main.go")
		require.Contains(t, prompt, "func main() {}")

		// Send response
		response := OllamaEmbedResponse{
			Model: "test-model",
			Embeddings: [][]float32{
				{0.7, 0.8, 0.9},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create service
	config := &EmbeddingServiceConfig{
		BaseURL: server.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}
	service := NewEmbeddingService(config)

	// Create metadata
	metadata := &EmbeddingMetadata{
		Language: "go",
		Kind:     "function",
		Symbol:   "main",
		Path:     "/test/main.go",
		Summary:  "Main function",
		Tags:     []string{"entry", "main"},
	}

	// Test code embedding generation
	embedding, err := service.GenerateCodeEmbedding(context.Background(), "func main() {}", metadata)

	require.NoError(t, err)
	require.Equal(t, []float32{0.7, 0.8, 0.9}, embedding)
}

func TestEmbeddingService_GenerateCodeEmbedding_EmptyCode(t *testing.T) {
	t.Parallel()

	service := NewEmbeddingService(nil)

	_, err := service.GenerateCodeEmbedding(context.Background(), "", nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "code content cannot be empty")
}

func TestEmbeddingService_buildContextualPrompt(t *testing.T) {
	t.Parallel()

	service := NewEmbeddingService(nil)

	tests := []struct {
		name     string
		code     string
		metadata *EmbeddingMetadata
		want     string
	}{
		{
			name:     "nil metadata returns code only",
			code:     "func test() {}",
			metadata: nil,
			want:     "func test() {}",
		},
		{
			name: "complete metadata",
			code: "func main() {}",
			metadata: &EmbeddingMetadata{
				Language: "go",
				Kind:     "function",
				Symbol:   "main",
				Path:     "/main.go",
				Summary:  "Entry point",
				Tags:     []string{"entry"},
			},
			want: "Language: go\nType: function\nSymbol: main\nFile: /main.go\nSummary: Entry point\nTags: [entry]\n\nCode:\nfunc main() {}",
		},
		{
			name: "partial metadata",
			code: "var x = 1",
			metadata: &EmbeddingMetadata{
				Language: "go",
				Kind:     "variable",
			},
			want: "Language: go\nType: variable\n\nCode:\nvar x = 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := service.buildContextualPrompt(tt.code, tt.metadata)
			require.Equal(t, tt.want, result)
		})
	}
}

func TestEmbeddingService_HealthCheck(t *testing.T) {
	t.Parallel()

	// Create mock server for successful health check
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := OllamaEmbedResponse{
			Model: "test-model",
			Embeddings: [][]float32{
				{0.1, 0.2, 0.3},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create service
	config := &EmbeddingServiceConfig{
		BaseURL: server.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}
	service := NewEmbeddingService(config)

	// Test successful health check
	err := service.HealthCheck(context.Background())
	require.NoError(t, err)
}

func TestEmbeddingService_HealthCheck_Failure(t *testing.T) {
	t.Parallel()

	// Create service with invalid URL
	config := &EmbeddingServiceConfig{
		BaseURL: "http://invalid-url:99999",
		Model:   "test-model",
		Timeout: 1 * time.Second,
	}
	service := NewEmbeddingService(config)

	// Test failed health check
	err := service.HealthCheck(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "Ollama health check failed")
}

func TestEmbeddingService_GetModel(t *testing.T) {
	t.Parallel()

	config := &EmbeddingServiceConfig{
		Model: "custom-model",
	}
	service := NewEmbeddingService(config)

	require.Equal(t, "custom-model", service.GetModel())
}

func TestEmbeddingService_GetBaseURL(t *testing.T) {
	t.Parallel()

	config := &EmbeddingServiceConfig{
		BaseURL: "http://custom-url:8080",
	}
	service := NewEmbeddingService(config)

	require.Equal(t, "http://custom-url:8080", service.GetBaseURL())
}

func TestEmbeddingService_HTTPError(t *testing.T) {
	t.Parallel()

	// Create mock server that returns HTTP error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	// Create service
	config := &EmbeddingServiceConfig{
		BaseURL: server.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}
	service := NewEmbeddingService(config)

	// Test HTTP error handling
	_, err := service.GenerateEmbedding(context.Background(), "test")

	require.Error(t, err)
	require.Contains(t, err.Error(), "Ollama API returned status 500")
}

func TestEmbeddingService_InvalidJSON(t *testing.T) {
	t.Parallel()

	// Create mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	// Create service
	config := &EmbeddingServiceConfig{
		BaseURL: server.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}
	service := NewEmbeddingService(config)

	// Test JSON parsing error handling
	_, err := service.GenerateEmbedding(context.Background(), "test")

	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to decode response")
}
