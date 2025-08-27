package vector

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/charmbracelet/crush/internal/config"
)

// Client wraps the Qdrant client with additional functionality for the Context Engine.
type Client struct {
	client            *qdrant.Client
	config            *config.VectorDBConfig
	collectionManager *CollectionManager
}

// NewClient creates a new vector database client based on the provided configuration.
func NewClient(cfg *config.VectorDBConfig) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("vector DB configuration is required")
	}

	// Set default values for missing configuration
	port := cfg.Port
	if port == 0 {
		port = 6334 // Default Qdrant port
	}

	connectionTimeout := time.Duration(cfg.ConnectionTimeoutMs) * time.Millisecond
	if connectionTimeout == 0 {
		connectionTimeout = 5 * time.Second // Default timeout
	}

	// Prepare gRPC options
	var grpcOptions []grpc.DialOption

	// Configure TLS
	if cfg.UseTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
			MinVersion:         tls.VersionTLS13,
		}
		grpcOptions = append(grpcOptions, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		grpcOptions = append(grpcOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Configure timeouts
	grpcOptions = append(grpcOptions,
		grpc.WithTimeout(connectionTimeout),
	)

	// Create Qdrant client configuration
	qdrantConfig := &qdrant.Config{
		Host:        cfg.URL,
		Port:        port,
		APIKey:      cfg.APIKey,
		UseTLS:      cfg.UseTLS,
		GrpcOptions: grpcOptions,
	}

	// Create Qdrant client
	client, err := qdrant.NewClient(qdrantConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}

	clientWrapper := &Client{
		client: client,
		config: cfg,
	}

	// Initialize collection manager
	clientWrapper.collectionManager = NewCollectionManager(clientWrapper, cfg)

	return clientWrapper, nil
}

// Close closes the underlying Qdrant client connection.
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// Ping tests the connection to the vector database.
func (c *Client) Ping(ctx context.Context) error {
	// Use the health check endpoint to verify connectivity
	_, err := c.client.HealthCheck(ctx)
	if err != nil {
		return fmt.Errorf("failed to ping vector database: %w", err)
	}
	return nil
}

// CollectionExists checks if the configured collection exists.
func (c *Client) CollectionExists(ctx context.Context) (bool, error) {
	// Try to create a simple count operation to check if collection exists
	_, err := c.client.Count(ctx, &qdrant.CountPoints{
		CollectionName: c.config.Collection,
	})
	if err != nil {
		// If count fails, likely the collection doesn't exist
		return false, nil
	}
	return true, nil
}

// CreateCollection creates the collection with the configured parameters.
func (c *Client) CreateCollection(ctx context.Context) error {
	// Set default vector size if not configured
	vectorSize := c.config.VectorSize
	if vectorSize == 0 {
		vectorSize = 1536 // Default OpenAI embedding size
	}

	// Set default distance metric if not configured
	distanceMetric := qdrant.Distance_Cosine
	switch c.config.DistanceMetric {
	case "Euclid":
		distanceMetric = qdrant.Distance_Euclid
	case "Dot":
		distanceMetric = qdrant.Distance_Dot
	default:
		distanceMetric = qdrant.Distance_Cosine
	}

	// Create collection configuration
	createReq := &qdrant.CreateCollection{
		CollectionName: c.config.Collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(vectorSize),
			Distance: distanceMetric,
		}),
	}

	err := c.client.CreateCollection(ctx, createReq)
	if err != nil {
		return fmt.Errorf("failed to create collection %s: %w", c.config.Collection, err)
	}

	return nil
}

// EnsureCollection creates the collection if it doesn't exist.
func (c *Client) EnsureCollection(ctx context.Context) error {
	exists, err := c.CollectionExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}

	if !exists {
		if err := c.CreateCollection(ctx); err != nil {
			return fmt.Errorf("failed to create collection: %w", err)
		}
	}

	return nil
}

// EnsureOptimizedCollection creates an optimized collection if it doesn't exist.
func (c *Client) EnsureOptimizedCollection(ctx context.Context, strategy *CollectionStrategy) error {
	exists, err := c.CollectionExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}

	if !exists {
		if err := c.collectionManager.CreateOptimizedCollection(ctx, strategy); err != nil {
			return fmt.Errorf("failed to create optimized collection: %w", err)
		}
	}

	return nil
}

// GetCollectionManager returns the collection manager for advanced operations.
func (c *Client) GetCollectionManager() *CollectionManager {
	return c.collectionManager
}
