package db

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// SQLiteVecStore provides vector storage and HNSW search using the sqlite-vec extension.
// sqlite-vec enables efficient approximate nearest neighbor search in SQLite.
//
// Note: sqlite-vec requires the CGO-enabled sqlite driver. When using modernc.org/sqlite
// (pure Go), this falls back to brute-force search using the existing embeddings table.
type SQLiteVecStore struct {
	db           DB
	dimensions   int
	tableName    string
	vecTableName string
	useVec0      bool // Whether sqlite-vec is available
}

// SQLiteVecConfig configures the sqlite-vec store.
type SQLiteVecConfig struct {
	// Dimensions is the vector size (e.g., 768 for nomic-embed-text)
	Dimensions int

	// TableName is the main embeddings table name
	TableName string

	// VecTableName is the vec0 virtual table name (default: vec_embeddings)
	VecTableName string
}

// NewSQLiteVecStore creates a new sqlite-vec backed vector store.
// If sqlite-vec is not available, operations fall back to brute-force search.
func NewSQLiteVecStore(database DB, cfg SQLiteVecConfig) (*SQLiteVecStore, error) {
	if cfg.VecTableName == "" {
		cfg.VecTableName = "vec_embeddings"
	}
	if cfg.TableName == "" {
		cfg.TableName = "embeddings"
	}

	store := &SQLiteVecStore{
		db:           database,
		dimensions:   cfg.Dimensions,
		tableName:    cfg.TableName,
		vecTableName: cfg.VecTableName,
		useVec0:      false,
	}

	// Check if sqlite-vec is available
	if store.checkVecAvailable() {
		store.useVec0 = true
		if err := store.initVecTable(); err != nil {
			// Log but don't fail - fall back to brute force
			store.useVec0 = false
		}
	}

	return store, nil
}

// checkVecAvailable checks if sqlite-vec extension is loaded.
func (s *SQLiteVecStore) checkVecAvailable() bool {
	// Try to create a vec0 table to check if extension is available
	testTable := "_vec_test_" + fmt.Sprintf("%d", s.dimensions)
	createSQL := fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(
			test_vec FLOAT[%d]
		)
	`, testTable, s.dimensions)

	_, err := s.db.Exec(createSQL)
	if err != nil {
		return false
	}

	// Clean up test table
	s.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", testTable))
	return true
}

// initVecTable creates the vec0 virtual table for vector search.
func (s *SQLiteVecStore) initVecTable() error {
	// vec0 virtual table schema
	// content_hash is used to link back to the main embeddings table
	createSQL := fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(
			content_hash TEXT PRIMARY KEY,
			embedding FLOAT[%d]
		)
	`, s.vecTableName, s.dimensions)

	_, err := s.db.Exec(createSQL)
	if err != nil {
		return fmt.Errorf("creating vec0 table: %w", err)
	}

	return nil
}

// IsVecAvailable returns true if sqlite-vec is available for HNSW search.
func (s *SQLiteVecStore) IsVecAvailable() bool {
	return s.useVec0
}

// Insert adds an embedding to the HNSW index.
func (s *SQLiteVecStore) Insert(ctx context.Context, contentHash string, embedding []float32) error {
	if !s.useVec0 {
		return nil // No-op without sqlite-vec
	}

	// Convert to blob format expected by sqlite-vec
	blob := float32SliceToBlob(embedding)

	_, err := s.db.ExecContext(ctx,
		fmt.Sprintf("INSERT OR REPLACE INTO %s(content_hash, embedding) VALUES (?, ?)", s.vecTableName),
		contentHash, blob,
	)
	return err
}

// InsertBatch adds multiple embeddings to the HNSW index.
func (s *SQLiteVecStore) InsertBatch(ctx context.Context, entries map[string][]float32) error {
	if !s.useVec0 || len(entries) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(
		fmt.Sprintf("INSERT OR REPLACE INTO %s(content_hash, embedding) VALUES (?, ?)", s.vecTableName),
	)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	for contentHash, embedding := range entries {
		blob := float32SliceToBlob(embedding)
		if _, err := stmt.Exec(contentHash, blob); err != nil {
			return fmt.Errorf("inserting embedding: %w", err)
		}
	}

	return tx.Commit()
}

// SQLiteVecSearchResult represents a result from sqlite-vec search.
type SQLiteVecSearchResult struct {
	ContentHash string
	Distance    float32
	Score       float32
}

// Search performs HNSW nearest neighbor search.
// If sqlite-vec is not available, returns empty results (caller should use brute-force).
func (s *SQLiteVecStore) Search(ctx context.Context, query []float32, k int) ([]SQLiteVecSearchResult, error) {
	if !s.useVec0 {
		return nil, nil
	}

	blob := float32SliceToBlob(query)

	// vec0 uses MATCH syntax for KNN search
	sql := fmt.Sprintf(`
		SELECT content_hash, distance
		FROM %s
		WHERE embedding MATCH ?
		ORDER BY distance
		LIMIT ?
	`, s.vecTableName)

	rows, err := s.db.QueryContext(ctx, sql, blob, k)
	if err != nil {
		return nil, fmt.Errorf("executing vec0 search: %w", err)
	}
	defer rows.Close()

	var results []SQLiteVecSearchResult
	for rows.Next() {
		var r SQLiteVecSearchResult
		if err := rows.Scan(&r.ContentHash, &r.Distance); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		// Convert distance to similarity score (assuming cosine/L2)
		r.Score = 1.0 - r.Distance
		if r.Score < 0 {
			r.Score = 1.0 / (1.0 + r.Distance)
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// SearchWithRepoFilter performs HNSW search filtered to specific repositories.
// This requires joining with the main embeddings table.
func (s *SQLiteVecStore) SearchWithRepoFilter(ctx context.Context, query []float32, k int, repoRoots []string) ([]SQLiteVecSearchResult, error) {
	if !s.useVec0 {
		return nil, nil
	}

	if len(repoRoots) == 0 {
		return s.Search(ctx, query, k)
	}

	blob := float32SliceToBlob(query)

	// Build placeholders for IN clause
	placeholders := make([]string, len(repoRoots))
	args := make([]interface{}, 0, len(repoRoots)+2)
	args = append(args, blob)
	for i, repo := range repoRoots {
		placeholders[i] = "?"
		args = append(args, repo)
	}
	args = append(args, k)

	// Join vec0 results with main table for repo filtering
	sql := fmt.Sprintf(`
		SELECT v.content_hash, v.distance
		FROM %s v
		INNER JOIN %s e ON v.content_hash = e.content_hash
		WHERE v.embedding MATCH ?
		AND e.repo_root IN (%s)
		ORDER BY v.distance
		LIMIT ?
	`, s.vecTableName, s.tableName, joinPlaceholders(placeholders))

	rows, err := s.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("executing filtered vec0 search: %w", err)
	}
	defer rows.Close()

	var results []SQLiteVecSearchResult
	for rows.Next() {
		var r SQLiteVecSearchResult
		if err := rows.Scan(&r.ContentHash, &r.Distance); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		r.Score = 1.0 - r.Distance
		if r.Score < 0 {
			r.Score = 1.0 / (1.0 + r.Distance)
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// Delete removes an embedding from the HNSW index.
func (s *SQLiteVecStore) Delete(ctx context.Context, contentHash string) error {
	if !s.useVec0 {
		return nil
	}

	_, err := s.db.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM %s WHERE content_hash = ?", s.vecTableName),
		contentHash,
	)
	return err
}

// DeleteBatch removes multiple embeddings from the HNSW index.
func (s *SQLiteVecStore) DeleteBatch(ctx context.Context, contentHashes []string) error {
	if !s.useVec0 || len(contentHashes) == 0 {
		return nil
	}

	// Build IN clause
	placeholders := make([]string, len(contentHashes))
	args := make([]interface{}, len(contentHashes))
	for i, hash := range contentHashes {
		placeholders[i] = "?"
		args[i] = hash
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE content_hash IN (%s)",
		s.vecTableName, joinPlaceholders(placeholders))

	_, err := s.db.ExecContext(ctx, sql, args...)
	return err
}

// Rebuild recreates the vec0 table from the main embeddings table.
// This is useful after bulk modifications or to optimize index structure.
func (s *SQLiteVecStore) Rebuild(ctx context.Context) error {
	if !s.useVec0 {
		return nil
	}

	// Drop and recreate vec0 table
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", s.vecTableName)); err != nil {
		return fmt.Errorf("dropping vec0 table: %w", err)
	}

	if err := s.initVecTable(); err != nil {
		return err
	}

	// Repopulate from main embeddings table
	// This reads embeddings stored as JSON and converts to vec0 format
	sql := fmt.Sprintf(`
		SELECT content_hash, embedding FROM %s
	`, s.tableName)

	rows, err := s.db.QueryContext(ctx, sql)
	if err != nil {
		return fmt.Errorf("reading embeddings: %w", err)
	}
	defer rows.Close()

	// Use batch insert for efficiency
	entries := make(map[string][]float32)
	for rows.Next() {
		var contentHash string
		var embeddingJSON string
		if err := rows.Scan(&contentHash, &embeddingJSON); err != nil {
			continue
		}

		// Parse JSON embedding
		var embedding []float32
		if err := parseJSONEmbedding(embeddingJSON, &embedding); err != nil {
			continue
		}

		entries[contentHash] = embedding

		// Batch every 1000 entries
		if len(entries) >= 1000 {
			if err := s.InsertBatch(ctx, entries); err != nil {
				return fmt.Errorf("batch insert during rebuild: %w", err)
			}
			entries = make(map[string][]float32)
		}
	}

	// Insert remaining entries
	if len(entries) > 0 {
		if err := s.InsertBatch(ctx, entries); err != nil {
			return fmt.Errorf("final batch insert during rebuild: %w", err)
		}
	}

	return rows.Err()
}

// Count returns the number of vectors in the HNSW index.
func (s *SQLiteVecStore) Count(ctx context.Context) (int, error) {
	if !s.useVec0 {
		return 0, nil
	}

	var count int
	err := s.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM %s", s.vecTableName),
	).Scan(&count)

	return count, err
}

// SyncFromEmbeddings synchronizes the vec0 table with the main embeddings table.
// This adds any missing embeddings and removes stale ones.
func (s *SQLiteVecStore) SyncFromEmbeddings(ctx context.Context) error {
	if !s.useVec0 {
		return nil
	}

	// Find embeddings that need to be added
	sql := fmt.Sprintf(`
		SELECT e.content_hash, e.embedding
		FROM %s e
		LEFT JOIN %s v ON e.content_hash = v.content_hash
		WHERE v.content_hash IS NULL
	`, s.tableName, s.vecTableName)

	rows, err := s.db.QueryContext(ctx, sql)
	if err != nil {
		return fmt.Errorf("finding missing embeddings: %w", err)
	}
	defer rows.Close()

	entries := make(map[string][]float32)
	for rows.Next() {
		var contentHash string
		var embeddingJSON string
		if err := rows.Scan(&contentHash, &embeddingJSON); err != nil {
			continue
		}

		var embedding []float32
		if err := parseJSONEmbedding(embeddingJSON, &embedding); err != nil {
			continue
		}

		entries[contentHash] = embedding
	}

	if len(entries) > 0 {
		if err := s.InsertBatch(ctx, entries); err != nil {
			return fmt.Errorf("adding missing embeddings: %w", err)
		}
	}

	// Remove stale entries from vec0 table
	deleteSQL := fmt.Sprintf(`
		DELETE FROM %s
		WHERE content_hash NOT IN (SELECT content_hash FROM %s)
	`, s.vecTableName, s.tableName)

	_, err = s.db.ExecContext(ctx, deleteSQL)
	if err != nil {
		return fmt.Errorf("removing stale embeddings: %w", err)
	}

	return rows.Err()
}

// float32SliceToBlob converts a []float32 to []byte for sqlite-vec.
// sqlite-vec expects vectors as little-endian binary blobs.
func float32SliceToBlob(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		bits := math.Float32bits(f)
		binary.LittleEndian.PutUint32(buf[i*4:], bits)
	}
	return buf
}

// blobToFloat32Slice converts a []byte blob back to []float32.
func blobToFloat32Slice(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	result := make([]float32, len(b)/4)
	for i := range result {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		result[i] = math.Float32frombits(bits)
	}
	return result
}

// parseJSONEmbedding parses a JSON array string into a float32 slice.
func parseJSONEmbedding(jsonStr string, dest *[]float32) error {
	// Quick validation
	if len(jsonStr) < 2 || jsonStr[0] != '[' || jsonStr[len(jsonStr)-1] != ']' {
		return fmt.Errorf("invalid JSON array format")
	}

	// Use json.Unmarshal for correctness
	return json.Unmarshal([]byte(jsonStr), dest)
}

// joinPlaceholders joins placeholders with commas.
func joinPlaceholders(placeholders []string) string {
	if len(placeholders) == 0 {
		return ""
	}
	result := placeholders[0]
	for i := 1; i < len(placeholders); i++ {
		result += ", " + placeholders[i]
	}
	return result
}
