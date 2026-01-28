package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// HNSWConfig holds HNSW index configuration parameters.
// These are imported from config package at runtime.
type HNSWConfig struct {
	M              int
	EfConstruction int
	EfSearch       int
	DistanceMetric string
}

// DefaultHNSWConfig returns sensible defaults for HNSW indexing.
func DefaultHNSWConfig() HNSWConfig {
	return HNSWConfig{
		M:              16,
		EfConstruction: 64,
		EfSearch:       40,
		DistanceMetric: "cosine",
	}
}

// PostgresHNSW provides HNSW index management for PostgreSQL with pgvector.
// pgvector 0.5.0+ supports native HNSW indexing for efficient approximate
// nearest neighbor search.
type PostgresHNSW struct {
	db      DB
	dialect *PostgresDialect
}

// NewPostgresHNSW creates a new PostgreSQL HNSW helper.
func NewPostgresHNSW(database DB) *PostgresHNSW {
	return &PostgresHNSW{
		db:      database,
		dialect: &PostgresDialect{},
	}
}

// CreateHNSWIndexSQL generates SQL to create an HNSW index on an embedding table.
// The table must have a 'embedding' column of type vector.
func (p *PostgresHNSW) CreateHNSWIndexSQL(tableName string, cfg HNSWConfig) string {
	opClass := metricToOpClass(cfg.DistanceMetric)
	indexName := fmt.Sprintf("idx_%s_hnsw", tableName)

	return fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS %s
		ON %s
		USING hnsw (embedding %s)
		WITH (m = %d, ef_construction = %d)
	`, indexName, tableName, opClass, cfg.M, cfg.EfConstruction)
}

// CreateHNSWIndex creates an HNSW index on an embedding table.
func (p *PostgresHNSW) CreateHNSWIndex(ctx context.Context, tableName string, cfg HNSWConfig) error {
	sql := p.CreateHNSWIndexSQL(tableName, cfg)
	_, err := p.db.ExecContext(ctx, sql)
	if err != nil {
		return fmt.Errorf("creating HNSW index on %s: %w", tableName, err)
	}
	return nil
}

// DropHNSWIndex removes an HNSW index from a table.
func (p *PostgresHNSW) DropHNSWIndex(ctx context.Context, tableName string) error {
	indexName := fmt.Sprintf("idx_%s_hnsw", tableName)
	sql := fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)
	_, err := p.db.ExecContext(ctx, sql)
	if err != nil {
		return fmt.Errorf("dropping HNSW index %s: %w", indexName, err)
	}
	return nil
}

// SetEfSearchSQL returns SQL to set the ef_search parameter for HNSW queries.
// This should be executed before running search queries.
func (p *PostgresHNSW) SetEfSearchSQL(efSearch int) string {
	return fmt.Sprintf("SET hnsw.ef_search = %d", efSearch)
}

// SetEfSearch sets the ef_search parameter for subsequent HNSW queries.
func (p *PostgresHNSW) SetEfSearch(ctx context.Context, efSearch int) error {
	sql := p.SetEfSearchSQL(efSearch)
	_, err := p.db.ExecContext(ctx, sql)
	if err != nil {
		return fmt.Errorf("setting ef_search: %w", err)
	}
	return nil
}

// HNSWSearchSQL returns SQL for HNSW nearest neighbor search.
// The query uses the HNSW index automatically when available.
func (p *PostgresHNSW) HNSWSearchSQL(tableName string, limit int, metric string) string {
	distOp := metricToOperator(metric)
	return fmt.Sprintf(`
		SELECT content_hash, embedding %s $1 as distance
		FROM %s
		WHERE embedding IS NOT NULL
		ORDER BY embedding %s $1
		LIMIT %d
	`, distOp, tableName, distOp, limit)
}

// HNSWSearchResult represents a result from HNSW search.
type HNSWSearchResult struct {
	ContentHash string
	Distance    float32
	Score       float32 // 1 - distance for cosine
}

// Search performs HNSW nearest neighbor search.
func (p *PostgresHNSW) Search(ctx context.Context, tableName string, query []float32, k int, cfg HNSWConfig) ([]HNSWSearchResult, error) {
	// Set ef_search for this query session
	if err := p.SetEfSearch(ctx, cfg.EfSearch); err != nil {
		return nil, err
	}

	// Format query vector for pgvector
	queryVec := formatVectorForPgvector(query)

	// Execute search
	sql := p.HNSWSearchSQL(tableName, k, cfg.DistanceMetric)
	rows, err := p.db.QueryContext(ctx, sql, queryVec)
	if err != nil {
		return nil, fmt.Errorf("executing HNSW search: %w", err)
	}
	defer rows.Close()

	var results []HNSWSearchResult
	for rows.Next() {
		var r HNSWSearchResult
		if err := rows.Scan(&r.ContentHash, &r.Distance); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		// Convert distance to similarity score
		r.Score = distanceToScore(r.Distance, cfg.DistanceMetric)
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating results: %w", err)
	}

	return results, nil
}

// SearchWithRepoFilter performs HNSW search filtered to specific repositories.
func (p *PostgresHNSW) SearchWithRepoFilter(ctx context.Context, tableName string, query []float32, k int, cfg HNSWConfig, repoRoots []string) ([]HNSWSearchResult, error) {
	if len(repoRoots) == 0 {
		return p.Search(ctx, tableName, query, k, cfg)
	}

	// Set ef_search for this query session
	if err := p.SetEfSearch(ctx, cfg.EfSearch); err != nil {
		return nil, err
	}

	// Format query vector
	queryVec := formatVectorForPgvector(query)

	// Build query with repo filter
	distOp := metricToOperator(cfg.DistanceMetric)
	placeholders := make([]string, len(repoRoots))
	args := make([]interface{}, 0, len(repoRoots)+1)
	args = append(args, queryVec)
	for i, repo := range repoRoots {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, repo)
	}

	sql := fmt.Sprintf(`
		SELECT content_hash, embedding %s $1 as distance
		FROM %s
		WHERE embedding IS NOT NULL AND repo_root IN (%s)
		ORDER BY embedding %s $1
		LIMIT %d
	`, distOp, tableName, strings.Join(placeholders, ", "), distOp, k)

	rows, err := p.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("executing filtered HNSW search: %w", err)
	}
	defer rows.Close()

	var results []HNSWSearchResult
	for rows.Next() {
		var r HNSWSearchResult
		if err := rows.Scan(&r.ContentHash, &r.Distance); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		r.Score = distanceToScore(r.Distance, cfg.DistanceMetric)
		results = append(results, r)
	}

	return results, rows.Err()
}

// RebuildIndex drops and recreates the HNSW index.
// This is useful after bulk inserts to optimize index structure.
func (p *PostgresHNSW) RebuildIndex(ctx context.Context, tableName string, cfg HNSWConfig) error {
	// Drop existing index
	if err := p.DropHNSWIndex(ctx, tableName); err != nil {
		return err
	}

	// Recreate with potentially new parameters
	return p.CreateHNSWIndex(ctx, tableName, cfg)
}

// IndexStats returns statistics about an HNSW index.
type IndexStats struct {
	IndexName   string
	TableName   string
	IndexSize   int64  // Size in bytes
	IsValid     bool   // Whether index is valid and usable
	IndexMethod string // Should be "hnsw"
}

// GetIndexStats retrieves statistics about the HNSW index on a table.
func (p *PostgresHNSW) GetIndexStats(ctx context.Context, tableName string) (*IndexStats, error) {
	indexName := fmt.Sprintf("idx_%s_hnsw", tableName)

	sql := `
		SELECT
			indexrelname as index_name,
			pg_relation_size(indexrelid) as index_size,
			indisvalid as is_valid,
			am.amname as index_method
		FROM pg_stat_user_indexes sui
		JOIN pg_index i ON sui.indexrelid = i.indexrelid
		JOIN pg_class c ON i.indexrelid = c.oid
		JOIN pg_am am ON c.relam = am.oid
		WHERE sui.indexrelname = $1
	`

	var stats IndexStats
	stats.TableName = tableName
	err := p.db.QueryRowContext(ctx, sql, indexName).Scan(
		&stats.IndexName,
		&stats.IndexSize,
		&stats.IsValid,
		&stats.IndexMethod,
	)
	if err != nil {
		return nil, fmt.Errorf("getting index stats: %w", err)
	}

	return &stats, nil
}

// HasHNSWIndex checks if an HNSW index exists on the table.
func (p *PostgresHNSW) HasHNSWIndex(ctx context.Context, tableName string) (bool, error) {
	indexName := fmt.Sprintf("idx_%s_hnsw", tableName)

	sql := `
		SELECT EXISTS(
			SELECT 1 FROM pg_indexes
			WHERE indexname = $1
		)
	`

	var exists bool
	err := p.db.QueryRowContext(ctx, sql, indexName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking HNSW index: %w", err)
	}

	return exists, nil
}

// SupportsHNSW checks if the database supports HNSW indexing (pgvector 0.5.0+).
func (p *PostgresHNSW) SupportsHNSW(ctx context.Context) (bool, error) {
	// Check pgvector extension version
	sql := `
		SELECT extversion FROM pg_extension WHERE extname = 'vector'
	`

	var version string
	err := p.db.QueryRowContext(ctx, sql).Scan(&version)
	if err != nil {
		return false, fmt.Errorf("checking pgvector version: %w", err)
	}

	// HNSW was added in pgvector 0.5.0
	// Parse version and check >= 0.5.0
	return isVersionAtLeast(version, "0.5.0"), nil
}

// metricToOpClass returns the pgvector operator class for a distance metric.
func metricToOpClass(metric string) string {
	switch metric {
	case "euclidean":
		return "vector_l2_ops"
	case "dot_product":
		return "vector_ip_ops"
	default: // cosine
		return "vector_cosine_ops"
	}
}

// metricToOperator returns the pgvector distance operator for a metric.
func metricToOperator(metric string) string {
	switch metric {
	case "euclidean":
		return "<->"
	case "dot_product":
		return "<#>"
	default: // cosine
		return "<=>"
	}
}

// distanceToScore converts a distance to a similarity score (0-1).
func distanceToScore(distance float32, metric string) float32 {
	switch metric {
	case "euclidean":
		// L2 distance: use inverse transform
		return 1.0 / (1.0 + distance)
	case "dot_product":
		// Negative inner product: negate to get similarity
		return -distance
	default: // cosine
		// Cosine distance is 1 - cosine_similarity
		return 1.0 - distance
	}
}

// formatVectorForPgvector formats a float32 slice as a pgvector string.
func formatVectorForPgvector(v []float32) string {
	// pgvector accepts JSON array format or '[...]' format
	b, _ := json.Marshal(v)
	return string(b)
}

// isVersionAtLeast checks if version >= minVersion (semver comparison).
func isVersionAtLeast(version, minVersion string) bool {
	// Parse major.minor.patch
	var vMajor, vMinor, vPatch int
	fmt.Sscanf(version, "%d.%d.%d", &vMajor, &vMinor, &vPatch)

	var mMajor, mMinor, mPatch int
	fmt.Sscanf(minVersion, "%d.%d.%d", &mMajor, &mMinor, &mPatch)

	if vMajor != mMajor {
		return vMajor > mMajor
	}
	if vMinor != mMinor {
		return vMinor > mMinor
	}
	return vPatch >= mPatch
}
