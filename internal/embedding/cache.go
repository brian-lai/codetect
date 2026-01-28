package embedding

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"codetect/internal/db"
)

// EmbeddingCache provides content-addressed embedding storage.
// Embeddings are stored by content hash (SHA-256), enabling deduplication
// across files, repos, and time. Identical code chunks share one embedding.
//
// For PostgreSQL, uses dimension-grouped tables (embedding_cache_768, etc.)
// For SQLite, uses a single embedding_cache table with JSON vectors.
type EmbeddingCache struct {
	database   db.DB
	dialect    db.Dialect
	schema     *db.SchemaBuilder
	dimensions int
	model      string
	mu         sync.RWMutex // Protects concurrent access
}

// CacheEntry represents a cached embedding with metadata.
type CacheEntry struct {
	ContentHash  string    `json:"content_hash"`
	Embedding    []float32 `json:"embedding"`
	Model        string    `json:"model"`
	Dimensions   int       `json:"dimensions"`
	CreatedAt    time.Time `json:"created_at"`
	AccessCount  int       `json:"access_count"`
	LastAccessed time.Time `json:"last_accessed"`
}

// CacheStats provides cache statistics.
type CacheStats struct {
	TotalEntries    int     `json:"total_entries"`
	TotalSize       int64   `json:"total_size_bytes"`
	AvgAccessCount  float64 `json:"avg_access_count"`
	OldestEntry     time.Time
	NewestEntry     time.Time
	MostAccessed    int
	LeastAccessed   int
}

// NewEmbeddingCache creates a new content-addressed embedding cache.
// dimensions specifies the vector size (e.g., 768 for nomic-embed-text).
// model identifies the embedding model for cache invalidation.
func NewEmbeddingCache(database db.DB, dialect db.Dialect, dimensions int, model string) (*EmbeddingCache, error) {
	cache := &EmbeddingCache{
		database:   database,
		dialect:    dialect,
		schema:     db.NewSchemaBuilder(database, dialect),
		dimensions: dimensions,
		model:      model,
	}

	if err := cache.initSchema(); err != nil {
		return nil, fmt.Errorf("initializing cache schema: %w", err)
	}

	return cache, nil
}

// initSchema creates the embedding_cache table if it doesn't exist.
func (c *EmbeddingCache) initSchema() error {
	tableName := c.tableName()

	// Define columns based on dialect
	columns := c.cacheColumns()

	// Create table
	createSQL := c.dialect.CreateTableSQL(tableName, columns)
	if _, err := c.database.Exec(createSQL); err != nil {
		return fmt.Errorf("creating %s table: %w", tableName, err)
	}

	// Create index on model for filtering by embedding provider
	idxModelName := fmt.Sprintf("idx_%s_model", tableName)
	idxModel := c.dialect.CreateIndexSQL(tableName, idxModelName, []string{"model"}, false)
	if _, err := c.database.Exec(idxModel); err != nil {
		return fmt.Errorf("creating model index: %w", err)
	}

	// Create index on last_accessed for LRU eviction
	idxAccessName := fmt.Sprintf("idx_%s_access", tableName)
	idxAccess := c.dialect.CreateIndexSQL(tableName, idxAccessName, []string{"last_accessed"}, false)
	if _, err := c.database.Exec(idxAccess); err != nil {
		return fmt.Errorf("creating access index: %w", err)
	}

	return nil
}

// cacheColumns returns the column definitions for the cache table.
func (c *EmbeddingCache) cacheColumns() []db.ColumnDef {
	embeddingCol := db.ColumnDef{
		Name:     "embedding",
		Nullable: false,
	}

	// Use native vector type for PostgreSQL, TEXT (JSON) for SQLite
	if c.dialect.Name() == "postgres" {
		embeddingCol.Type = db.ColTypeVector
		embeddingCol.VectorDimension = c.dimensions
	} else {
		embeddingCol.Type = db.ColTypeText // JSON storage
	}

	columns := []db.ColumnDef{
		{Name: "content_hash", Type: db.ColTypeText, Nullable: false, PrimaryKey: true},
		embeddingCol,
		{Name: "model", Type: db.ColTypeText, Nullable: false},
		{Name: "dimensions", Type: db.ColTypeInteger, Nullable: false},
		{Name: "created_at", Type: db.ColTypeInteger, Nullable: false},
		{Name: "access_count", Type: db.ColTypeInteger, Nullable: false, Default: "1"},
		{Name: "last_accessed", Type: db.ColTypeInteger, Nullable: false},
	}

	// For PostgreSQL, we store dimensions implicitly in the table name
	// so we can skip the dimensions column
	if c.dialect.Name() == "postgres" {
		// Filter out dimensions column for postgres (implicit in table name)
		var filtered []db.ColumnDef
		for _, col := range columns {
			if col.Name != "dimensions" {
				filtered = append(filtered, col)
			}
		}
		columns = filtered
	}

	return columns
}

// tableName returns the table name for this cache's vector dimensions.
// PostgreSQL uses dimension-grouped tables (embedding_cache_768, etc.)
// SQLite uses a single embedding_cache table.
func (c *EmbeddingCache) tableName() string {
	if c.dialect.Name() == "postgres" {
		return fmt.Sprintf("embedding_cache_%d", c.dimensions)
	}
	return "embedding_cache"
}

// Get retrieves an embedding by content hash.
// Returns nil if not found (cache miss), without error.
// Updates access statistics on cache hit.
func (c *EmbeddingCache) Get(contentHash string) (*CacheEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tableName := c.tableName()

	// Build query based on dialect (postgres doesn't have dimensions column)
	var query string
	if c.dialect.Name() == "postgres" {
		query = fmt.Sprintf(`
			SELECT content_hash, embedding, model, created_at, access_count, last_accessed
			FROM %s WHERE content_hash = %s
		`, tableName, c.dialect.Placeholder(1))
	} else {
		query = fmt.Sprintf(`
			SELECT content_hash, embedding, model, dimensions, created_at, access_count, last_accessed
			FROM %s WHERE content_hash = %s
		`, tableName, c.dialect.Placeholder(1))
	}

	row := c.database.QueryRow(query, contentHash)

	var entry CacheEntry
	var embeddingData string
	var createdAt, lastAccessed int64

	var err error
	if c.dialect.Name() == "postgres" {
		entry.Dimensions = c.dimensions // Implicit from table
		err = row.Scan(
			&entry.ContentHash,
			&embeddingData,
			&entry.Model,
			&createdAt,
			&entry.AccessCount,
			&lastAccessed,
		)
	} else {
		err = row.Scan(
			&entry.ContentHash,
			&embeddingData,
			&entry.Model,
			&entry.Dimensions,
			&createdAt,
			&entry.AccessCount,
			&lastAccessed,
		)
	}

	if err == sql.ErrNoRows {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("scanning cache entry: %w", err)
	}

	// Parse embedding from JSON
	if err := json.Unmarshal([]byte(embeddingData), &entry.Embedding); err != nil {
		return nil, fmt.Errorf("parsing embedding: %w", err)
	}

	entry.CreatedAt = time.Unix(createdAt, 0)
	entry.LastAccessed = time.Unix(lastAccessed, 0)

	// Update access stats asynchronously (fire-and-forget)
	go c.updateAccessStats(contentHash)

	return &entry, nil
}

// GetBatch retrieves multiple embeddings by content hashes.
// Returns a map of hash -> entry for found embeddings.
// Missing hashes are simply not included in the result (no error).
func (c *EmbeddingCache) GetBatch(hashes []string) (map[string]*CacheEntry, error) {
	if len(hashes) == 0 {
		return make(map[string]*CacheEntry), nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	tableName := c.tableName()

	// Build placeholders for IN clause
	placeholders := make([]string, len(hashes))
	args := make([]interface{}, len(hashes))
	for i, hash := range hashes {
		placeholders[i] = c.dialect.Placeholder(i + 1)
		args[i] = hash
	}

	// Build query based on dialect
	var query string
	if c.dialect.Name() == "postgres" {
		query = fmt.Sprintf(`
			SELECT content_hash, embedding, model, created_at, access_count, last_accessed
			FROM %s WHERE content_hash IN (%s)
		`, tableName, strings.Join(placeholders, ", "))
	} else {
		query = fmt.Sprintf(`
			SELECT content_hash, embedding, model, dimensions, created_at, access_count, last_accessed
			FROM %s WHERE content_hash IN (%s)
		`, tableName, strings.Join(placeholders, ", "))
	}

	rows, err := c.database.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch lookup: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*CacheEntry)
	var foundHashes []string

	for rows.Next() {
		var entry CacheEntry
		var embeddingData string
		var createdAt, lastAccessed int64

		var scanErr error
		if c.dialect.Name() == "postgres" {
			entry.Dimensions = c.dimensions
			scanErr = rows.Scan(
				&entry.ContentHash,
				&embeddingData,
				&entry.Model,
				&createdAt,
				&entry.AccessCount,
				&lastAccessed,
			)
		} else {
			scanErr = rows.Scan(
				&entry.ContentHash,
				&embeddingData,
				&entry.Model,
				&entry.Dimensions,
				&createdAt,
				&entry.AccessCount,
				&lastAccessed,
			)
		}

		if scanErr != nil {
			continue // Skip malformed entries
		}

		// Parse embedding
		if err := json.Unmarshal([]byte(embeddingData), &entry.Embedding); err != nil {
			continue // Skip malformed embeddings
		}

		entry.CreatedAt = time.Unix(createdAt, 0)
		entry.LastAccessed = time.Unix(lastAccessed, 0)

		result[entry.ContentHash] = &entry
		foundHashes = append(foundHashes, entry.ContentHash)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating results: %w", err)
	}

	// Update access stats asynchronously for found entries
	if len(foundHashes) > 0 {
		go c.updateAccessStatsBatch(foundHashes)
	}

	return result, nil
}

// Put stores an embedding in the cache.
// If the hash already exists, increments access_count and updates last_accessed.
func (c *EmbeddingCache) Put(contentHash string, embedding []float32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	tableName := c.tableName()
	now := time.Now().Unix()

	// Serialize embedding to JSON
	embJSON, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("marshaling embedding: %w", err)
	}

	// Use upsert to handle duplicates
	var upsertSQL string
	var args []interface{}

	if c.dialect.Name() == "postgres" {
		// PostgreSQL: dimensions implicit in table name
		// Build custom upsert that increments access_count
		upsertSQL = fmt.Sprintf(`
			INSERT INTO %s (content_hash, embedding, model, created_at, access_count, last_accessed)
			VALUES ($1, $2, $3, $4, 1, $5)
			ON CONFLICT (content_hash) DO UPDATE SET
				access_count = %s.access_count + 1,
				last_accessed = $6
		`, tableName, tableName)
		args = []interface{}{contentHash, string(embJSON), c.model, now, now, now}
	} else {
		// SQLite: include dimensions column
		upsertSQL = c.schema.SubstitutePlaceholders(fmt.Sprintf(`
			INSERT INTO %s (content_hash, embedding, model, dimensions, created_at, access_count, last_accessed)
			VALUES (?, ?, ?, ?, ?, 1, ?)
			ON CONFLICT (content_hash) DO UPDATE SET
				access_count = access_count + 1,
				last_accessed = ?
		`, tableName))
		args = []interface{}{contentHash, string(embJSON), c.model, c.dimensions, now, now, now}
	}

	_, err = c.database.Exec(upsertSQL, args...)
	if err != nil {
		return fmt.Errorf("storing embedding: %w", err)
	}

	return nil
}

// PutBatch stores multiple embeddings in a transaction.
// More efficient than individual Put calls for bulk operations.
func (c *EmbeddingCache) PutBatch(entries map[string][]float32) error {
	if len(entries) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	tx, err := c.database.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	tableName := c.tableName()
	now := time.Now().Unix()

	// Prepare statement based on dialect
	var stmtSQL string
	if c.dialect.Name() == "postgres" {
		stmtSQL = fmt.Sprintf(`
			INSERT INTO %s (content_hash, embedding, model, created_at, access_count, last_accessed)
			VALUES ($1, $2, $3, $4, 1, $5)
			ON CONFLICT (content_hash) DO UPDATE SET
				access_count = %s.access_count + 1,
				last_accessed = $6
		`, tableName, tableName)
	} else {
		stmtSQL = c.schema.SubstitutePlaceholders(fmt.Sprintf(`
			INSERT INTO %s (content_hash, embedding, model, dimensions, created_at, access_count, last_accessed)
			VALUES (?, ?, ?, ?, ?, 1, ?)
			ON CONFLICT (content_hash) DO UPDATE SET
				access_count = access_count + 1,
				last_accessed = ?
		`, tableName))
	}

	stmt, err := tx.Prepare(stmtSQL)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	for hash, embedding := range entries {
		embJSON, err := json.Marshal(embedding)
		if err != nil {
			return fmt.Errorf("marshaling embedding for %s: %w", hash, err)
		}

		var execErr error
		if c.dialect.Name() == "postgres" {
			_, execErr = stmt.Exec(hash, string(embJSON), c.model, now, now, now)
		} else {
			_, execErr = stmt.Exec(hash, string(embJSON), c.model, c.dimensions, now, now, now)
		}

		if execErr != nil {
			return fmt.Errorf("inserting %s: %w", hash, execErr)
		}
	}

	return tx.Commit()
}

// Delete removes an embedding from the cache.
func (c *EmbeddingCache) Delete(contentHash string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	tableName := c.tableName()
	query := c.schema.SubstitutePlaceholders(
		fmt.Sprintf("DELETE FROM %s WHERE content_hash = ?", tableName),
	)
	_, err := c.database.Exec(query, contentHash)
	return err
}

// DeleteBatch removes multiple embeddings from the cache.
func (c *EmbeddingCache) DeleteBatch(hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	tableName := c.tableName()

	placeholders := make([]string, len(hashes))
	args := make([]interface{}, len(hashes))
	for i, hash := range hashes {
		placeholders[i] = c.dialect.Placeholder(i + 1)
		args[i] = hash
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE content_hash IN (%s)",
		tableName, strings.Join(placeholders, ", "))
	_, err := c.database.Exec(query, args...)
	return err
}

// Count returns the number of entries in the cache.
func (c *EmbeddingCache) Count() (int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tableName := c.tableName()
	var count int
	err := c.database.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count)
	return count, err
}

// Stats returns cache statistics.
func (c *EmbeddingCache) Stats() (*CacheStats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tableName := c.tableName()

	var stats CacheStats
	var oldest, newest, totalAccess sql.NullInt64
	var avgAccess sql.NullFloat64
	var mostAccessed, leastAccessed sql.NullInt64

	query := fmt.Sprintf(`
		SELECT
			COUNT(*),
			AVG(access_count),
			MIN(created_at),
			MAX(created_at),
			MAX(access_count),
			MIN(access_count),
			SUM(access_count)
		FROM %s
	`, tableName)

	err := c.database.QueryRow(query).Scan(
		&stats.TotalEntries,
		&avgAccess,
		&oldest,
		&newest,
		&mostAccessed,
		&leastAccessed,
		&totalAccess,
	)
	if err != nil {
		return nil, fmt.Errorf("querying stats: %w", err)
	}

	// Handle nullable values
	if avgAccess.Valid {
		stats.AvgAccessCount = avgAccess.Float64
	}
	if oldest.Valid {
		stats.OldestEntry = time.Unix(oldest.Int64, 0)
	}
	if newest.Valid {
		stats.NewestEntry = time.Unix(newest.Int64, 0)
	}
	if mostAccessed.Valid {
		stats.MostAccessed = int(mostAccessed.Int64)
	}
	if leastAccessed.Valid {
		stats.LeastAccessed = int(leastAccessed.Int64)
	}

	return &stats, nil
}

// Evict removes least-recently-used entries to reduce cache size.
// keepCount specifies the maximum number of entries to retain.
func (c *EmbeddingCache) Evict(keepCount int) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	tableName := c.tableName()

	// Count current entries
	var currentCount int
	err := c.database.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&currentCount)
	if err != nil {
		return 0, fmt.Errorf("counting entries: %w", err)
	}

	if currentCount <= keepCount {
		return 0, nil // Nothing to evict
	}

	toEvict := currentCount - keepCount

	// Delete oldest entries by last_accessed
	// This subquery approach works for both SQLite and PostgreSQL
	var deleteSQL string
	if c.dialect.Name() == "postgres" {
		deleteSQL = fmt.Sprintf(`
			DELETE FROM %s WHERE content_hash IN (
				SELECT content_hash FROM %s
				ORDER BY last_accessed ASC
				LIMIT %d
			)
		`, tableName, tableName, toEvict)
	} else {
		deleteSQL = fmt.Sprintf(`
			DELETE FROM %s WHERE content_hash IN (
				SELECT content_hash FROM %s
				ORDER BY last_accessed ASC
				LIMIT %d
			)
		`, tableName, tableName, toEvict)
	}

	result, err := c.database.Exec(deleteSQL)
	if err != nil {
		return 0, fmt.Errorf("evicting entries: %w", err)
	}

	evicted, _ := result.RowsAffected()
	return int(evicted), nil
}

// EvictByModel removes all entries for a specific model.
// Useful when switching embedding providers.
func (c *EmbeddingCache) EvictByModel(model string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	tableName := c.tableName()
	query := c.schema.SubstitutePlaceholders(
		fmt.Sprintf("DELETE FROM %s WHERE model = ?", tableName),
	)

	result, err := c.database.Exec(query, model)
	if err != nil {
		return 0, fmt.Errorf("evicting model %s: %w", model, err)
	}

	evicted, _ := result.RowsAffected()
	return int(evicted), nil
}

// updateAccessStats updates access_count and last_accessed for a single entry.
func (c *EmbeddingCache) updateAccessStats(contentHash string) {
	tableName := c.tableName()
	now := time.Now().Unix()

	query := c.schema.SubstitutePlaceholders(fmt.Sprintf(`
		UPDATE %s SET access_count = access_count + 1, last_accessed = ?
		WHERE content_hash = ?
	`, tableName))

	// Fire-and-forget, ignore errors
	c.database.Exec(query, now, contentHash)
}

// updateAccessStatsBatch updates access stats for multiple entries.
func (c *EmbeddingCache) updateAccessStatsBatch(hashes []string) {
	if len(hashes) == 0 {
		return
	}

	tableName := c.tableName()
	now := time.Now().Unix()

	// Build placeholders starting at index 2 (index 1 is for timestamp)
	placeholders := make([]string, len(hashes))
	args := make([]interface{}, len(hashes)+1)
	args[0] = now

	for i, hash := range hashes {
		placeholders[i] = c.dialect.Placeholder(i + 2)
		args[i+1] = hash
	}

	query := fmt.Sprintf(`
		UPDATE %s SET access_count = access_count + 1, last_accessed = %s
		WHERE content_hash IN (%s)
	`, tableName, c.dialect.Placeholder(1), strings.Join(placeholders, ", "))

	// Fire-and-forget, ignore errors
	c.database.Exec(query, args...)
}

// Model returns the embedding model this cache is configured for.
func (c *EmbeddingCache) Model() string {
	return c.model
}

// Dimensions returns the vector dimensions for this cache.
func (c *EmbeddingCache) Dimensions() int {
	return c.dimensions
}

// HasEntry checks if a content hash exists in the cache.
func (c *EmbeddingCache) HasEntry(contentHash string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tableName := c.tableName()
	query := c.schema.SubstitutePlaceholders(
		fmt.Sprintf("SELECT 1 FROM %s WHERE content_hash = ? LIMIT 1", tableName),
	)

	var exists int
	err := c.database.QueryRow(query, contentHash).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// HasEntryBatch checks which content hashes exist in the cache.
// Returns a map of hash -> exists.
func (c *EmbeddingCache) HasEntryBatch(hashes []string) (map[string]bool, error) {
	if len(hashes) == 0 {
		return make(map[string]bool), nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	tableName := c.tableName()

	placeholders := make([]string, len(hashes))
	args := make([]interface{}, len(hashes))
	for i, hash := range hashes {
		placeholders[i] = c.dialect.Placeholder(i + 1)
		args[i] = hash
	}

	query := fmt.Sprintf("SELECT content_hash FROM %s WHERE content_hash IN (%s)",
		tableName, strings.Join(placeholders, ", "))

	rows, err := c.database.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]bool)
	// Initialize all as false
	for _, hash := range hashes {
		result[hash] = false
	}

	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			continue
		}
		result[hash] = true
	}

	return result, rows.Err()
}
