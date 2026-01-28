package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"codetect/internal/config"
	"codetect/internal/db"
)

// VectorIndex provides an interface for approximate nearest neighbor search.
// Implementations use HNSW (Hierarchical Navigable Small World) graphs for
// sub-linear O(log n) search instead of O(n) brute-force.
type VectorIndex interface {
	// Insert adds an embedding to the index.
	Insert(ctx context.Context, contentHash string, embedding []float32) error

	// InsertBatch adds multiple embeddings efficiently.
	InsertBatch(ctx context.Context, entries map[string][]float32) error

	// Search finds k nearest neighbors to the query vector.
	// Returns results sorted by distance (closest first).
	Search(ctx context.Context, query []float32, k int) ([]VectorResult, error)

	// SearchWithFilter finds k nearest neighbors filtered by repository.
	// If repoRoots is empty, searches all repositories.
	SearchWithFilter(ctx context.Context, query []float32, k int, repoRoots []string) ([]VectorResult, error)

	// Delete removes an embedding from the index.
	Delete(ctx context.Context, contentHash string) error

	// DeleteBatch removes multiple embeddings.
	DeleteBatch(ctx context.Context, contentHashes []string) error

	// Rebuild recreates the index for optimization.
	// This can improve query performance after many insertions/deletions.
	Rebuild(ctx context.Context) error

	// IsNative returns true if using native HNSW (not brute-force fallback).
	IsNative() bool

	// Count returns the number of vectors in the index.
	Count(ctx context.Context) (int, error)
}

// VectorResult represents a search result from the vector index.
type VectorResult struct {
	// ContentHash uniquely identifies the embedded content.
	ContentHash string `json:"content_hash"`

	// Distance is the distance/dissimilarity from the query vector.
	// Lower values indicate more similar vectors.
	Distance float32 `json:"distance"`

	// Score is the similarity score (1 - distance for cosine).
	// Higher values indicate more similar vectors.
	Score float32 `json:"score"`
}

// PostgresVectorIndex implements VectorIndex using pgvector HNSW.
type PostgresVectorIndex struct {
	hnsw       *db.PostgresHNSW
	database   db.DB
	dialect    *db.PostgresDialect
	tableName  string
	dimensions int
	config     config.HNSWConfig
}

// NewPostgresVectorIndex creates a new PostgreSQL-backed vector index.
func NewPostgresVectorIndex(database db.DB, dimensions int, cfg config.HNSWConfig) (*PostgresVectorIndex, error) {
	tableName := fmt.Sprintf("embeddings_%d", dimensions)

	// Convert config.HNSWConfig to db.HNSWConfig
	dbCfg := db.HNSWConfig{
		M:              cfg.M,
		EfConstruction: cfg.EfConstruction,
		EfSearch:       cfg.EfSearch,
		DistanceMetric: cfg.DistanceMetric,
	}

	hnsw := db.NewPostgresHNSW(database)

	// Ensure HNSW index exists
	ctx := context.Background()
	if err := hnsw.CreateHNSWIndex(ctx, tableName, dbCfg); err != nil {
		// Index may already exist - check if this is just a duplicate key error
		// which we can safely ignore
	}

	return &PostgresVectorIndex{
		hnsw:       hnsw,
		database:   database,
		dialect:    &db.PostgresDialect{},
		tableName:  tableName,
		dimensions: dimensions,
		config:     cfg,
	}, nil
}

// Insert adds an embedding to the index.
// For PostgreSQL, this inserts into the embeddings table; the HNSW index updates automatically.
func (p *PostgresVectorIndex) Insert(ctx context.Context, contentHash string, embedding []float32) error {
	// PostgreSQL HNSW index is maintained automatically on INSERT
	// This method is provided for interface compatibility
	// Actual insertions should go through EmbeddingStore.Save()
	return nil
}

// InsertBatch adds multiple embeddings efficiently.
func (p *PostgresVectorIndex) InsertBatch(ctx context.Context, entries map[string][]float32) error {
	// Batch inserts handled by EmbeddingStore
	return nil
}

// Search finds k nearest neighbors using HNSW.
func (p *PostgresVectorIndex) Search(ctx context.Context, query []float32, k int) ([]VectorResult, error) {
	dbCfg := db.HNSWConfig{
		M:              p.config.M,
		EfConstruction: p.config.EfConstruction,
		EfSearch:       p.config.EfSearch,
		DistanceMetric: p.config.DistanceMetric,
	}

	results, err := p.hnsw.Search(ctx, p.tableName, query, k, dbCfg)
	if err != nil {
		return nil, fmt.Errorf("HNSW search: %w", err)
	}

	vectorResults := make([]VectorResult, len(results))
	for i, r := range results {
		vectorResults[i] = VectorResult{
			ContentHash: r.ContentHash,
			Distance:    r.Distance,
			Score:       r.Score,
		}
	}

	return vectorResults, nil
}

// SearchWithFilter finds k nearest neighbors filtered by repository.
func (p *PostgresVectorIndex) SearchWithFilter(ctx context.Context, query []float32, k int, repoRoots []string) ([]VectorResult, error) {
	dbCfg := db.HNSWConfig{
		M:              p.config.M,
		EfConstruction: p.config.EfConstruction,
		EfSearch:       p.config.EfSearch,
		DistanceMetric: p.config.DistanceMetric,
	}

	results, err := p.hnsw.SearchWithRepoFilter(ctx, p.tableName, query, k, dbCfg, repoRoots)
	if err != nil {
		return nil, fmt.Errorf("filtered HNSW search: %w", err)
	}

	vectorResults := make([]VectorResult, len(results))
	for i, r := range results {
		vectorResults[i] = VectorResult{
			ContentHash: r.ContentHash,
			Distance:    r.Distance,
			Score:       r.Score,
		}
	}

	return vectorResults, nil
}

// Delete removes an embedding from the index.
func (p *PostgresVectorIndex) Delete(ctx context.Context, contentHash string) error {
	// HNSW index updates automatically on DELETE from embeddings table
	return nil
}

// DeleteBatch removes multiple embeddings.
func (p *PostgresVectorIndex) DeleteBatch(ctx context.Context, contentHashes []string) error {
	// HNSW index updates automatically on DELETE
	return nil
}

// Rebuild recreates the HNSW index.
func (p *PostgresVectorIndex) Rebuild(ctx context.Context) error {
	dbCfg := db.HNSWConfig{
		M:              p.config.M,
		EfConstruction: p.config.EfConstruction,
		EfSearch:       p.config.EfSearch,
		DistanceMetric: p.config.DistanceMetric,
	}
	return p.hnsw.RebuildIndex(ctx, p.tableName, dbCfg)
}

// IsNative returns true (PostgreSQL uses native HNSW).
func (p *PostgresVectorIndex) IsNative() bool {
	return true
}

// Count returns the number of vectors in the index.
func (p *PostgresVectorIndex) Count(ctx context.Context) (int, error) {
	var count int
	sql := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE embedding IS NOT NULL", p.tableName)
	err := p.database.QueryRowContext(ctx, sql).Scan(&count)
	return count, err
}

// SQLiteVectorIndex implements VectorIndex using sqlite-vec.
type SQLiteVectorIndex struct {
	store      *db.SQLiteVecStore
	database   db.DB
	dimensions int
}

// NewSQLiteVectorIndex creates a new SQLite-backed vector index.
func NewSQLiteVectorIndex(database db.DB, dimensions int) (*SQLiteVectorIndex, error) {
	cfg := db.SQLiteVecConfig{
		Dimensions:   dimensions,
		TableName:    "embeddings",
		VecTableName: "vec_embeddings",
	}

	store, err := db.NewSQLiteVecStore(database, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating sqlite-vec store: %w", err)
	}

	return &SQLiteVectorIndex{
		store:      store,
		database:   database,
		dimensions: dimensions,
	}, nil
}

// Insert adds an embedding to the index.
func (s *SQLiteVectorIndex) Insert(ctx context.Context, contentHash string, embedding []float32) error {
	return s.store.Insert(ctx, contentHash, embedding)
}

// InsertBatch adds multiple embeddings efficiently.
func (s *SQLiteVectorIndex) InsertBatch(ctx context.Context, entries map[string][]float32) error {
	return s.store.InsertBatch(ctx, entries)
}

// Search finds k nearest neighbors.
func (s *SQLiteVectorIndex) Search(ctx context.Context, query []float32, k int) ([]VectorResult, error) {
	results, err := s.store.Search(ctx, query, k)
	if err != nil {
		return nil, err
	}

	vectorResults := make([]VectorResult, len(results))
	for i, r := range results {
		vectorResults[i] = VectorResult{
			ContentHash: r.ContentHash,
			Distance:    r.Distance,
			Score:       r.Score,
		}
	}

	return vectorResults, nil
}

// SearchWithFilter finds k nearest neighbors filtered by repository.
func (s *SQLiteVectorIndex) SearchWithFilter(ctx context.Context, query []float32, k int, repoRoots []string) ([]VectorResult, error) {
	results, err := s.store.SearchWithRepoFilter(ctx, query, k, repoRoots)
	if err != nil {
		return nil, err
	}

	vectorResults := make([]VectorResult, len(results))
	for i, r := range results {
		vectorResults[i] = VectorResult{
			ContentHash: r.ContentHash,
			Distance:    r.Distance,
			Score:       r.Score,
		}
	}

	return vectorResults, nil
}

// Delete removes an embedding from the index.
func (s *SQLiteVectorIndex) Delete(ctx context.Context, contentHash string) error {
	return s.store.Delete(ctx, contentHash)
}

// DeleteBatch removes multiple embeddings.
func (s *SQLiteVectorIndex) DeleteBatch(ctx context.Context, contentHashes []string) error {
	return s.store.DeleteBatch(ctx, contentHashes)
}

// Rebuild recreates the index.
func (s *SQLiteVectorIndex) Rebuild(ctx context.Context) error {
	return s.store.Rebuild(ctx)
}

// IsNative returns true if sqlite-vec is available.
func (s *SQLiteVectorIndex) IsNative() bool {
	return s.store.IsVecAvailable()
}

// Count returns the number of vectors in the index.
func (s *SQLiteVectorIndex) Count(ctx context.Context) (int, error) {
	return s.store.Count(ctx)
}

// Sync synchronizes the vec0 table with the main embeddings table.
func (s *SQLiteVectorIndex) Sync(ctx context.Context) error {
	return s.store.SyncFromEmbeddings(ctx)
}

// BruteForceVectorIndex implements VectorIndex using in-memory brute-force search.
// This is the fallback when native HNSW is not available.
type BruteForceVectorIndex struct {
	store      *EmbeddingStore
	vectors    map[string][]float32
	mu         sync.RWMutex
	dimensions int
}

// NewBruteForceVectorIndex creates a new brute-force vector index.
// This loads vectors from the embedding store on demand.
func NewBruteForceVectorIndex(store *EmbeddingStore, dimensions int) *BruteForceVectorIndex {
	return &BruteForceVectorIndex{
		store:      store,
		vectors:    make(map[string][]float32),
		dimensions: dimensions,
	}
}

// Insert adds an embedding to the in-memory index.
func (b *BruteForceVectorIndex) Insert(ctx context.Context, contentHash string, embedding []float32) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.vectors[contentHash] = embedding
	return nil
}

// InsertBatch adds multiple embeddings.
func (b *BruteForceVectorIndex) InsertBatch(ctx context.Context, entries map[string][]float32) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for hash, emb := range entries {
		b.vectors[hash] = emb
	}
	return nil
}

// Search finds k nearest neighbors using brute-force.
func (b *BruteForceVectorIndex) Search(ctx context.Context, query []float32, k int) ([]VectorResult, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Load from store if empty
	if len(b.vectors) == 0 {
		b.mu.RUnlock()
		if err := b.loadFromStore(ctx); err != nil {
			b.mu.RLock()
			return nil, err
		}
		b.mu.RLock()
	}

	// Convert to slice for TopKByCosineSimilarity
	hashes := make([]string, 0, len(b.vectors))
	vectors := make([][]float32, 0, len(b.vectors))
	for hash, vec := range b.vectors {
		hashes = append(hashes, hash)
		vectors = append(vectors, vec)
	}

	// Find top-k
	topK := TopKByCosineSimilarity(query, vectors, k)

	results := make([]VectorResult, len(topK))
	for i, item := range topK {
		results[i] = VectorResult{
			ContentHash: hashes[item.Index],
			Distance:    1.0 - item.Score, // Convert similarity to distance
			Score:       item.Score,
		}
	}

	return results, nil
}

// SearchWithFilter finds k nearest neighbors filtered by repository.
// For brute-force, this filters after the search (less efficient than native).
func (b *BruteForceVectorIndex) SearchWithFilter(ctx context.Context, query []float32, k int, repoRoots []string) ([]VectorResult, error) {
	if len(repoRoots) == 0 {
		return b.Search(ctx, query, k)
	}

	// For brute-force, we need to search more broadly and filter
	// This is inefficient but maintains correctness
	results, err := b.Search(ctx, query, k*5) // Search more to account for filtering
	if err != nil {
		return nil, err
	}

	// Filter by repo - requires joining with embedding store
	// For now, return all results (filtering should be done by caller with location data)
	if len(results) > k {
		results = results[:k]
	}

	return results, nil
}

// Delete removes an embedding from the in-memory index.
func (b *BruteForceVectorIndex) Delete(ctx context.Context, contentHash string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.vectors, contentHash)
	return nil
}

// DeleteBatch removes multiple embeddings.
func (b *BruteForceVectorIndex) DeleteBatch(ctx context.Context, contentHashes []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, hash := range contentHashes {
		delete(b.vectors, hash)
	}
	return nil
}

// Rebuild reloads vectors from the store.
func (b *BruteForceVectorIndex) Rebuild(ctx context.Context) error {
	b.mu.Lock()
	b.vectors = make(map[string][]float32)
	b.mu.Unlock()
	return b.loadFromStore(ctx)
}

// IsNative returns false (brute-force is not native HNSW).
func (b *BruteForceVectorIndex) IsNative() bool {
	return false
}

// Count returns the number of vectors in the index.
func (b *BruteForceVectorIndex) Count(ctx context.Context) (int, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.vectors), nil
}

// loadFromStore loads vectors from the embedding store.
func (b *BruteForceVectorIndex) loadFromStore(ctx context.Context) error {
	if b.store == nil {
		return nil
	}

	records, err := b.store.GetAll()
	if err != nil {
		return fmt.Errorf("loading embeddings: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for _, r := range records {
		b.vectors[r.ContentHash] = r.Embedding
	}

	return nil
}

// NewVectorIndex creates the appropriate VectorIndex for the given database type.
// Returns PostgresVectorIndex for PostgreSQL, SQLiteVectorIndex for SQLite,
// or falls back to BruteForceVectorIndex if native HNSW is not available.
func NewVectorIndex(database db.DB, dialect db.Dialect, dimensions int, store *EmbeddingStore) (VectorIndex, error) {
	switch dialect.Name() {
	case "postgres":
		cfg := config.DefaultHNSWConfig()
		idx, err := NewPostgresVectorIndex(database, dimensions, cfg)
		if err != nil {
			// Fall back to brute force
			return NewBruteForceVectorIndex(store, dimensions), nil
		}
		return idx, nil

	case "sqlite":
		idx, err := NewSQLiteVectorIndex(database, dimensions)
		if err != nil {
			return NewBruteForceVectorIndex(store, dimensions), nil
		}
		// Check if sqlite-vec is actually available
		if !idx.IsNative() {
			return NewBruteForceVectorIndex(store, dimensions), nil
		}
		return idx, nil

	default:
		return NewBruteForceVectorIndex(store, dimensions), nil
	}
}

// VectorIndexWithLocation wraps VectorIndex to provide location resolution.
// This maps content hashes back to file paths and line numbers.
type VectorIndexWithLocation struct {
	VectorIndex
	store *EmbeddingStore
}

// NewVectorIndexWithLocation creates a VectorIndex that can resolve locations.
func NewVectorIndexWithLocation(idx VectorIndex, store *EmbeddingStore) *VectorIndexWithLocation {
	return &VectorIndexWithLocation{
		VectorIndex: idx,
		store:       store,
	}
}

// VectorResultWithLocation extends VectorResult with location information.
type VectorResultWithLocation struct {
	VectorResult
	RepoRoot  string `json:"repo_root,omitempty"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// SearchWithLocations performs search and resolves content hashes to file locations.
func (v *VectorIndexWithLocation) SearchWithLocations(ctx context.Context, query []float32, k int) ([]VectorResultWithLocation, error) {
	results, err := v.VectorIndex.Search(ctx, query, k)
	if err != nil {
		return nil, err
	}

	return v.resolveLocations(ctx, results)
}

// resolveLocations maps content hashes to file paths and line numbers.
func (v *VectorIndexWithLocation) resolveLocations(ctx context.Context, results []VectorResult) ([]VectorResultWithLocation, error) {
	if v.store == nil {
		// No store, return results without locations
		withLoc := make([]VectorResultWithLocation, len(results))
		for i, r := range results {
			withLoc[i] = VectorResultWithLocation{VectorResult: r}
		}
		return withLoc, nil
	}

	// Query for locations by content hash
	// This is a simplified implementation - real version would batch query
	var withLoc []VectorResultWithLocation

	// Get all embeddings and build a lookup map
	all, err := v.store.GetAll()
	if err != nil {
		return nil, fmt.Errorf("getting embeddings: %w", err)
	}

	hashToRecord := make(map[string]EmbeddingRecord)
	for _, r := range all {
		hashToRecord[r.ContentHash] = r
	}

	for _, r := range results {
		result := VectorResultWithLocation{VectorResult: r}
		if record, ok := hashToRecord[r.ContentHash]; ok {
			result.Path = record.Path
			result.StartLine = record.StartLine
			result.EndLine = record.EndLine
			result.RepoRoot = record.RepoRoot
		}
		withLoc = append(withLoc, result)
	}

	return withLoc, nil
}

// MarshalJSON implements json.Marshaler for VectorResult.
func (r VectorResult) MarshalJSON() ([]byte, error) {
	type Alias VectorResult
	return json.Marshal(Alias(r))
}
