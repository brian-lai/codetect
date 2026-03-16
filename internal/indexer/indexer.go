// Package indexer provides the v2 incremental indexing pipeline.
// It integrates Merkle tree change detection, AST-based chunking,
// content-addressed embedding cache, and HNSW vector indexing.
package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	ignore "github.com/sabhiram/go-gitignore"

	"codetect/internal/chunker"
	"codetect/internal/datadir"
	"codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/merkle"
)

// Indexer coordinates the v2 indexing pipeline with:
// - Merkle tree change detection for incremental updates
// - AST-based syntactic chunking
// - Content-addressed embedding cache
// - Optional HNSW vector indexing
type Indexer struct {
	repoPath string
	dataDir  string

	// Components
	merkleStore   *merkle.Store
	merkleBuilder *merkle.Builder
	astChunker    *chunker.ASTChunker
	cache         *embedding.EmbeddingCache
	locations     embedding.LocationAccess
	vectorIndex   embedding.VectorIndex
	embedder      embedding.Embedder
	pipeline      *embedding.Pipeline
	failureStore  *embedding.FailureStore
	pathMapper    *embedding.PathMapper // non-nil when HashPaths is enabled

	// Database
	database db.DB
	dialect  db.Dialect

	// Configuration
	config *Config
	logger *slog.Logger
}

// Config configures the indexer.
type Config struct {
	// Database settings
	DBType string // "sqlite" or "postgres"
	DBPath string // SQLite path (for sqlite type)
	DSN    string // PostgreSQL connection string

	// Embedding settings
	EmbeddingProvider string // "ollama", "litellm", or "off"
	EmbeddingModel    string // Model name
	Dimensions        int    // Vector dimensions
	OllamaURL         string // Ollama API URL
	LiteLLMURL        string // LiteLLM API URL
	LiteLLMKey        string // LiteLLM API key

	// Pipeline settings
	BatchSize  int // Batch size for embedding API calls
	MaxWorkers int // Max concurrent embedding workers

	// Token limit for embedding chunks (0 = use default)
	MaxTokens int

	// Parallel is the user-specified concurrency override (--parallel/-j flag).
	// 0 means auto-detect from repo size; >0 overrides both embed and chunk workers.
	Parallel int

	// Ignore patterns (from .gitignore)
	IgnorePatterns []string

	// HashPaths enables SHA-256 hashing of file paths at rest in the DB.
	HashPaths bool
}

// ConcurrencyProfile controls parallelism for the indexing pipeline.
type ConcurrencyProfile struct {
	EmbedWorkers  int    // concurrent embedding workers
	ChunkWorkers  int    // concurrent file reading + chunking goroutines
	FileBatchSize int    // files per batch
	Tier          string // "small", "medium", or "large" (for logging)
}

// ComputeConcurrency returns a ConcurrencyProfile based on repo size,
// embedding provider, and optional user override.
func ComputeConcurrency(fileCount int, provider string, userOverride int) ConcurrencyProfile {
	var p ConcurrencyProfile

	switch {
	case fileCount >= 5000:
		p = ConcurrencyProfile{EmbedWorkers: 8, ChunkWorkers: 8, FileBatchSize: 500, Tier: "large"}
	case fileCount >= 500:
		p = ConcurrencyProfile{EmbedWorkers: 4, ChunkWorkers: 4, FileBatchSize: 200, Tier: "medium"}
	default:
		p = ConcurrencyProfile{EmbedWorkers: 2, ChunkWorkers: 2, FileBatchSize: 100, Tier: "small"}
	}

	// LiteLLM handles batching server-side; fewer client workers avoids contention,
	// but always keep at least 2 workers to avoid sequential bottleneck.
	if provider == "litellm" {
		p.EmbedWorkers = max(2, p.EmbedWorkers/2)
	}

	// User override takes precedence
	if userOverride > 0 {
		p.EmbedWorkers = userOverride
		p.ChunkWorkers = userOverride
	}

	return p
}

// AdjustConcurrencyForChunks scales embed workers based on actual chunk count.
// A repo with 20 large files may produce more chunks than one with 500 small files.
func AdjustConcurrencyForChunks(profile ConcurrencyProfile, chunkCount int, provider string) ConcurrencyProfile {
	switch {
	case chunkCount >= 2000:
		profile.EmbedWorkers = max(profile.EmbedWorkers, 4)
	case chunkCount >= 500:
		profile.EmbedWorkers = max(profile.EmbedWorkers, 3)
	}
	// Cap LiteLLM workers to avoid rate limiting
	if provider == "litellm" && profile.EmbedWorkers > 6 {
		profile.EmbedWorkers = 6
	}
	return profile
}

// DefaultConfig returns the default indexer configuration.
func DefaultConfig() *Config {
	return &Config{
		DBType:            "sqlite",
		EmbeddingProvider: "ollama",
		EmbeddingModel:    "nomic-embed-text",
		Dimensions:        768,
		OllamaURL:         "http://localhost:11434",
		LiteLLMURL:        "http://localhost:4000",
		BatchSize:         50,
		MaxWorkers:        4,
	}
}

// New creates a new v2 indexer.
func New(repoPath string, cfg *Config) (*Indexer, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	dataDir, err := datadir.ForRepo(absPath)
	if err != nil {
		return nil, fmt.Errorf("resolving data directory: %w", err)
	}

	idx := &Indexer{
		repoPath: absPath,
		dataDir:  dataDir,
		config:   cfg,
		logger:   slog.Default(),
	}

	// Initialize database
	if err := idx.initDatabase(); err != nil {
		return nil, fmt.Errorf("initializing database: %w", err)
	}

	// Initialize components
	if err := idx.initComponents(); err != nil {
		idx.Close()
		return nil, fmt.Errorf("initializing components: %w", err)
	}

	return idx, nil
}

// initDatabase opens the database connection.
func (idx *Indexer) initDatabase() error {
	var dbCfg db.Config

	switch idx.config.DBType {
	case "postgres":
		dbCfg = db.Config{
			Type: db.DatabasePostgres,
			DSN:  idx.config.DSN,
		}
	default:
		dbPath := idx.config.DBPath
		if dbPath == "" {
			dbPath = filepath.Join(idx.dataDir, "index.db")
		}
		dbCfg = db.Config{
			Type: db.DatabaseSQLite,
			Path: dbPath,
		}
	}

	database, err := db.Open(dbCfg)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	idx.database = database
	idx.dialect = db.GetDialect(dbCfg.Type)
	return nil
}

// initComponents initializes all pipeline components.
func (idx *Indexer) initComponents() error {
	// Merkle tree components
	idx.merkleStore = merkle.NewStore(idx.dataDir)
	idx.merkleBuilder = merkle.NewBuilder()
	// Add any additional ignore patterns
	if len(idx.config.IgnorePatterns) > 0 {
		idx.merkleBuilder.IgnorePatterns = append(
			idx.merkleBuilder.IgnorePatterns,
			idx.config.IgnorePatterns...,
		)
	}

	// AST chunker
	idx.astChunker = chunker.NewASTChunker()

	// Embedding cache and locations
	var err error

	// Load .codetectignore patterns
	codetectIgnore, err := LoadCodetectIgnoreHierarchy(idx.repoPath)
	if err != nil {
		// Non-fatal: log warning and continue without .codetectignore
		idx.logger.Warn("failed to load .codetectignore", "error", err)
	} else if codetectIgnore != nil {
		idx.merkleBuilder.WithCodetectIgnore(codetectIgnore)
		if idx.logger != nil {
			idx.logger.Info("loaded .codetectignore patterns")
		}
	}
	idx.cache, err = embedding.NewEmbeddingCache(
		idx.database,
		idx.dialect,
		idx.config.Dimensions,
		idx.config.EmbeddingModel,
	)
	if err != nil {
		return fmt.Errorf("creating embedding cache: %w", err)
	}

	locationStore, err := embedding.NewLocationStore(idx.database, idx.dialect)
	if err != nil {
		return fmt.Errorf("creating location store: %w", err)
	}

	// Optionally wrap with path hashing
	if idx.config.HashPaths {
		mapperPath := filepath.Join(idx.dataDir, "path_map.json")
		idx.pathMapper, err = embedding.NewPathMapper(mapperPath)
		if err != nil {
			return fmt.Errorf("creating path mapper: %w", err)
		}
		idx.locations = embedding.NewHashingLocationStore(locationStore, idx.pathMapper)
		idx.logger.Info("path hashing enabled", "mapper", mapperPath)
	} else {
		idx.locations = locationStore
	}

	// Vector index (create brute force as fallback)
	// The NewBruteForceVectorIndex needs an EmbeddingStore, but we can skip it
	// for now since vector index is optional
	idx.vectorIndex = nil // Will be initialized when needed

	// Embedder (if enabled)
	if idx.config.EmbeddingProvider != "off" {
		idx.embedder, err = idx.createEmbedder()
		if err != nil {
			return fmt.Errorf("creating embedder: %w", err)
		}
	} else {
		idx.embedder = &embedding.NullEmbedder{}
	}

	// Create failure store for persisting embedding failures
	idx.failureStore, err = embedding.NewFailureStore(idx.database, idx.dialect)
	if err != nil {
		return fmt.Errorf("creating failure store: %w", err)
	}
	if idx.pathMapper != nil {
		idx.failureStore.SetPathMapper(idx.pathMapper)
	}

	// Create pipeline with provider-aware chars/token ratio and optional exact counter
	var charsPerToken float64
	switch idx.config.EmbeddingProvider {
	case "litellm":
		charsPerToken = embedding.DefaultCharsPerTokenLiteLLM
	default:
		charsPerToken = embedding.DefaultCharsPerTokenOllama
	}

	// For LiteLLM/OpenAI, use exact tiktoken counting (cl100k_base covers
	// text-embedding-3-large, text-embedding-3-small, and ada-002).
	var tokenCounter *embedding.TokenCounter
	if idx.config.EmbeddingProvider == "litellm" {
		tc, err := embedding.NewTokenCounter("cl100k_base")
		if err != nil {
			idx.logger.Warn("tiktoken unavailable, falling back to char estimation", "error", err)
		} else {
			tokenCounter = tc
		}
	}

	idx.pipeline = embedding.NewPipeline(
		idx.cache,
		idx.locations,
		idx.embedder,
		embedding.WithBatchSize(idx.config.BatchSize),
		embedding.WithMaxWorkers(idx.config.MaxWorkers),
		embedding.WithFailureStore(idx.failureStore),
		embedding.WithCharsPerToken(charsPerToken),
		embedding.WithTokenCounter(tokenCounter),
	)

	return nil
}

// createEmbedder creates the appropriate embedder based on configuration.
func (idx *Indexer) createEmbedder() (embedding.Embedder, error) {
	cfg := embedding.ProviderConfig{
		Model:      idx.config.EmbeddingModel,
		OllamaURL:  idx.config.OllamaURL,
		LiteLLMURL: idx.config.LiteLLMURL,
		LiteLLMKey: idx.config.LiteLLMKey,
	}

	switch idx.config.EmbeddingProvider {
	case "ollama":
		cfg.Provider = embedding.ProviderOllama
	case "litellm":
		cfg.Provider = embedding.ProviderLiteLLM
	default:
		cfg.Provider = embedding.ProviderOff
	}

	return embedding.NewEmbedder(cfg)
}

// Close releases all resources.
func (idx *Indexer) Close() error {
	if idx.pathMapper != nil {
		if err := idx.pathMapper.Flush(); err != nil {
			idx.logger.Warn("failed to flush path mapper on close", "error", err)
		}
	}
	if idx.database != nil {
		return idx.database.Close()
	}
	return nil
}

// ProgressCallback reports indexing progress.
// stage is the current operation name, current is items processed, total is total items (-1 if unknown).
type ProgressCallback func(stage string, current, total int)

// IndexOptions configures the index operation.
type IndexOptions struct {
	Force    bool             // Force full reindex
	Verbose  bool             // Enable verbose logging
	Progress ProgressCallback // Optional progress callback
}

// IndexResult contains statistics from an index operation.
type IndexResult struct {
	FilesProcessed int           `json:"files_processed"`
	FilesDeleted   int           `json:"files_deleted"`
	ChunksCreated  int           `json:"chunks_created"`
	CacheHits      int           `json:"cache_hits"`
	ChunksEmbedded int           `json:"chunks_embedded"`
	Duration       time.Duration `json:"duration"`
	ChangeType     string        `json:"change_type"` // "full", "incremental", "none"
}

// Index performs incremental or full indexing.
func (idx *Indexer) Index(ctx context.Context, opts IndexOptions) (*IndexResult, error) {
	start := time.Now()
	result := &IndexResult{}

	// 1. Build current Merkle tree
	if opts.Progress != nil {
		opts.Progress("Scanning files", 0, -1)
	}
	if opts.Verbose {
		idx.logger.Info("building merkle tree", "path", idx.repoPath)
	}

	newTree, err := idx.merkleBuilder.Build(idx.repoPath)
	if err != nil {
		return nil, fmt.Errorf("building merkle tree: %w", err)
	}

	// 2. Determine what changed
	if opts.Progress != nil {
		opts.Progress("Detecting changes", 1, 1)
	}

	var filesToProcess []string
	var filesToDelete []string

	if opts.Force {
		result.ChangeType = "full"
		filesToProcess = idx.collectAllFiles(newTree.Root)
		if opts.Verbose {
			idx.logger.Info("force mode", "files", len(filesToProcess))
		}
	} else {
		oldTree, _ := idx.merkleStore.Load()
		changes := merkle.Diff(oldTree, newTree)

		if changes.IsEmpty() {
			result.ChangeType = "none"
			result.Duration = time.Since(start)
			if opts.Verbose {
				idx.logger.Info("no changes detected")
			}
			return result, nil
		}

		result.ChangeType = "incremental"
		filesToProcess = append(changes.Added, changes.Modified...)
		filesToDelete = changes.Deleted

		if opts.Verbose {
			idx.logger.Info("detected changes",
				"added", len(changes.Added),
				"modified", len(changes.Modified),
				"deleted", len(changes.Deleted))
		}
	}

	// 3. Handle deletions
	if len(filesToDelete) > 0 && opts.Progress != nil {
		opts.Progress("Deleting files", 0, len(filesToDelete))
	}
	for i, path := range filesToDelete {
		if err := idx.locations.DeleteByPath(idx.repoPath, path); err != nil {
			idx.logger.Warn("failed to delete locations", "path", path, "error", err)
		}
		if opts.Progress != nil {
			opts.Progress("Deleting files", i+1, len(filesToDelete))
		}
	}
	result.FilesDeleted = len(filesToDelete)

	// 4. Compute concurrency profile and apply it
	profile := ComputeConcurrency(
		newTree.FileCount,
		idx.config.EmbeddingProvider,
		idx.config.Parallel,
	)
	idx.pipeline.SetMaxWorkers(profile.EmbedWorkers)
	if opts.Verbose {
		idx.logger.Info("concurrency profile",
			"tier", profile.Tier,
			"embed_workers", profile.EmbedWorkers,
			"chunk_workers", profile.ChunkWorkers,
			"batch_size", profile.FileBatchSize,
			"files", newTree.FileCount)
	}

	// 5. Process files in batches
	batchSize := profile.FileBatchSize
	totalBatches := (len(filesToProcess) + batchSize - 1) / batchSize
	for i := 0; i < len(filesToProcess); i += batchSize {
		end := min(i+batchSize, len(filesToProcess))
		batch := filesToProcess[i:end]
		batchNum := i/batchSize + 1

		if opts.Progress != nil {
			opts.Progress("Processing files", batchNum, totalBatches)
		}

		batchResult, err := idx.processBatch(ctx, batch, opts, profile)
		if err != nil {
			idx.logger.Warn("batch processing error", "error", err)
			continue
		}

		result.FilesProcessed += len(batch)
		result.ChunksCreated += batchResult.ChunksCreated
		result.CacheHits += batchResult.CacheHits
		result.ChunksEmbedded += batchResult.ChunksEmbedded
	}

	// 6. Save Merkle tree
	if opts.Progress != nil {
		opts.Progress("Saving index", 1, 1)
	}
	if err := idx.merkleStore.Save(newTree); err != nil {
		return nil, fmt.Errorf("saving merkle tree: %w", err)
	}

	result.Duration = time.Since(start)
	return result, nil
}

// processBatch processes a batch of files with parallel chunking.
func (idx *Indexer) processBatch(ctx context.Context, files []string, opts IndexOptions, profile ConcurrencyProfile) (*IndexResult, error) {
	result := &IndexResult{}

	chunkWorkers := profile.ChunkWorkers
	if chunkWorkers < 1 {
		chunkWorkers = 1
	}

	type chunkResult struct {
		chunks []embedding.Chunk
	}

	resultCh := make(chan chunkResult, len(files))
	sem := make(chan struct{}, chunkWorkers)
	var wg sync.WaitGroup

	for _, relPath := range files {
		wg.Add(1)
		go func(rp string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fullPath := filepath.Join(idx.repoPath, rp)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				if opts.Verbose {
					idx.logger.Debug("skipping file", "path", rp, "error", err)
				}
				return
			}

			// Create per-goroutine chunker to avoid data race on mutable fields
			localChunker := chunker.NewASTChunker()

			// Use AST chunker with token-aware options
			chunkOpts := chunker.DefaultChunkOptions()
			if idx.config.MaxTokens > 0 {
				chunkOpts.MaxTokens = idx.config.MaxTokens
			} else {
				chunkOpts.MaxTokens = chunker.DefaultMaxTokens
			}
			// Set chars/token ratio based on embedding provider
			switch idx.config.EmbeddingProvider {
			case "litellm":
				chunkOpts.CharsPerToken = chunker.DefaultCharsPerTokenLiteLLM
			default:
				chunkOpts.CharsPerToken = chunker.DefaultCharsPerTokenOllama
			}
			astChunks, err := localChunker.ChunkFileWithOptions(ctx, rp, content, chunkOpts)
			if err != nil {
				if opts.Verbose {
					idx.logger.Debug("chunk error", "path", rp, "error", err)
				}
				return
			}

			var local []embedding.Chunk
			for _, ac := range astChunks {
				local = append(local, embedding.Chunk{
					Path:      ac.Path,
					StartLine: ac.StartLine,
					EndLine:   ac.EndLine,
					Content:   ac.Content,
					Kind:      ac.NodeType,
				})
			}
			if len(local) > 0 {
				resultCh <- chunkResult{chunks: local}
			}
		}(relPath)
	}

	// Close channel once all goroutines finish
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var allChunks []embedding.Chunk
	for cr := range resultCh {
		allChunks = append(allChunks, cr.chunks...)
	}

	result.ChunksCreated = len(allChunks)

	if len(allChunks) == 0 {
		return result, nil
	}

	// Scale concurrency based on actual chunk count (may exceed file-based estimate).
	adjusted := AdjustConcurrencyForChunks(profile, len(allChunks), idx.config.EmbeddingProvider)
	if adjusted.EmbedWorkers != profile.EmbedWorkers {
		idx.pipeline.SetMaxWorkers(adjusted.EmbedWorkers)
		if opts.Verbose {
			idx.logger.Info("concurrency adjusted for chunk count",
				"chunks", len(allChunks),
				"embed_workers", adjusted.EmbedWorkers)
		}
	}

	// Process through embedding pipeline (parallel)
	embedResult, err := idx.pipeline.ParallelEmbedChunks(ctx, idx.repoPath, allChunks)
	if err != nil {
		return nil, fmt.Errorf("embedding chunks: %w", err)
	}

	result.CacheHits = embedResult.CacheHits
	result.ChunksEmbedded = embedResult.Embedded

	return result, nil
}

// collectAllFiles recursively collects all file paths from a Merkle tree node.
func (idx *Indexer) collectAllFiles(node *merkle.Node) []string {
	var files []string
	if node == nil {
		return files
	}

	if !node.IsDir {
		return []string{node.Path}
	}

	for _, child := range node.Children {
		files = append(files, idx.collectAllFiles(child)...)
	}
	return files
}

// Stats returns statistics about the index.
func (idx *Indexer) Stats() (*IndexStats, error) {
	stats := &IndexStats{}

	// Location stats
	locStats, err := idx.locations.Stats(idx.repoPath)
	if err != nil {
		return nil, fmt.Errorf("getting location stats: %w", err)
	}
	stats.TotalChunks = locStats.TotalLocations
	stats.UniqueHashes = locStats.UniqueHashes
	stats.FileCount = locStats.FileCount
	stats.ByNodeType = locStats.ByNodeType
	stats.ByLanguage = locStats.ByLanguage

	// Cache stats
	cacheStats, err := idx.cache.Stats()
	if err != nil {
		return nil, fmt.Errorf("getting cache stats: %w", err)
	}
	stats.CachedEmbeddings = cacheStats.TotalEntries

	// Vector index stats
	if idx.vectorIndex != nil {
		count, err := idx.vectorIndex.Count(context.Background())
		if err == nil {
			stats.IndexedVectors = count
		}
		stats.VectorIndexNative = idx.vectorIndex.IsNative()
	}

	return stats, nil
}

// IndexStats contains statistics about the index.
type IndexStats struct {
	TotalChunks       int            `json:"total_chunks"`
	UniqueHashes      int            `json:"unique_hashes"`
	FileCount         int            `json:"file_count"`
	CachedEmbeddings  int            `json:"cached_embeddings"`
	IndexedVectors    int            `json:"indexed_vectors"`
	VectorIndexNative bool           `json:"vector_index_native"`
	ByNodeType        map[string]int `json:"by_node_type"`
	ByLanguage        map[string]int `json:"by_language"`
}

// LoadGitignore loads .gitignore patterns for the repository.
func LoadGitignore(repoPath string) []string {
	var patterns []string

	// Load global gitignore
	if homeDir, err := os.UserHomeDir(); err == nil {
		globalPath := filepath.Join(homeDir, ".gitignore")
		if content, err := os.ReadFile(globalPath); err == nil {
			patterns = append(patterns, parseGitignore(string(content))...)
		}
	}

	// Load local .gitignore
	localPath := filepath.Join(repoPath, ".gitignore")
	if content, err := os.ReadFile(localPath); err == nil {
		patterns = append(patterns, parseGitignore(string(content))...)
	}

	return patterns
}

// parseGitignore extracts patterns from gitignore content.
func parseGitignore(content string) []string {
	var patterns []string
	start := 0
	for i := 0; i <= len(content); i++ {
		if i == len(content) || content[i] == '\n' {
			line := content[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			// Skip empty lines and comments
			trimmed := line
			for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t') {
				trimmed = trimmed[1:]
			}
			if len(trimmed) > 0 && trimmed[0] != '#' {
				patterns = append(patterns, trimmed)
			}
			start = i + 1
		}
	}
	return patterns
}

// CompileGitignore compiles patterns into a matcher.
func CompileGitignore(patterns []string) *ignore.GitIgnore {
	if len(patterns) == 0 {
		return nil
	}
	return ignore.CompileIgnoreLines(patterns...)
}

// RepoPath returns the repository path.
func (idx *Indexer) RepoPath() string {
	return idx.repoPath
}

// Pipeline returns the embedding pipeline for external use.
func (idx *Indexer) Pipeline() *embedding.Pipeline {
	return idx.pipeline
}

// Locations returns the location access for external use.
// When HashPaths is enabled, this returns a HashingLocationStore.
func (idx *Indexer) Locations() embedding.LocationAccess {
	return idx.locations
}

// Cache returns the embedding cache for external use.
func (idx *Indexer) Cache() *embedding.EmbeddingCache {
	return idx.cache
}

// VectorIndex returns the vector index for external use.
// May be nil if no vector index is available.
func (idx *Indexer) VectorIndex() embedding.VectorIndex {
	return idx.vectorIndex
}

// FailureStore returns the failure store for external use.
func (idx *Indexer) FailureStore() *embedding.FailureStore {
	return idx.failureStore
}
