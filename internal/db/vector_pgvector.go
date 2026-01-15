package db

import (
	"context"
	"encoding/json"
	"fmt"
)

// PgVectorDB implements VectorDB using PostgreSQL's pgvector extension.
// It provides efficient vector similarity search using native database operations.
type PgVectorDB struct {
	db         DB
	dialect    Dialect
	dimensions int
	metric     DistanceMetric
}

// NewPgVectorDB creates a new pgvector-backed vector database.
// The database must have the pgvector extension installed.
func NewPgVectorDB(database DB, dimensions int, metric DistanceMetric) (*PgVectorDB, error) {
	dialect := GetDialect(DatabasePostgres)

	// Verify pgvector extension is available
	var hasExtension bool
	err := database.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pg_extension WHERE extname = 'vector'
		)
	`).Scan(&hasExtension)
	if err != nil {
		return nil, fmt.Errorf("checking pgvector extension: %w", err)
	}
	if !hasExtension {
		return nil, fmt.Errorf("pgvector extension not installed - run: CREATE EXTENSION vector")
	}

	return &PgVectorDB{
		db:         database,
		dialect:    dialect,
		dimensions: dimensions,
		metric:     metric,
	}, nil
}

// CreateVectorIndex creates a vector index using IVFFlat or HNSW.
// For pgvector, this creates an index on the embeddings table's embedding column.
func (p *PgVectorDB) CreateVectorIndex(ctx context.Context, name string, dimensions int, metric DistanceMetric) error {
	// Determine the operator class based on metric
	var opClass string
	switch metric {
	case DistanceCosine:
		opClass = "vector_cosine_ops"
	case DistanceEuclidean:
		opClass = "vector_l2_ops"
	case DistanceDotProduct:
		opClass = "vector_ip_ops"
	default:
		return fmt.Errorf("unsupported distance metric for pgvector: %s", metric.String())
	}

	// Use HNSW index for best performance (pgvector 0.5.0+)
	// Falls back to IVFFlat if HNSW is not available
	indexName := fmt.Sprintf("%s_embedding_idx", name)

	// Try HNSW first (better performance)
	hnsw := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS %s
		ON %s USING hnsw (embedding %s)
	`, indexName, name, opClass)

	if _, err := p.db.Exec(hnsw); err != nil {
		// If HNSW fails, try IVFFlat as fallback
		// IVFFlat requires specifying number of lists
		// Rule of thumb: lists = rows / 1000 (min 10, max 10000)
		ivfflat := fmt.Sprintf(`
			CREATE INDEX IF NOT EXISTS %s
			ON %s USING ivfflat (embedding %s)
			WITH (lists = 100)
		`, indexName, name, opClass)

		if _, err := p.db.Exec(ivfflat); err != nil {
			return fmt.Errorf("creating vector index (tried HNSW and IVFFlat): %w", err)
		}
	}

	return nil
}

// InsertVector inserts a single vector into the embeddings table.
// Note: This assumes the embeddings table already exists with the correct schema.
func (p *PgVectorDB) InsertVector(ctx context.Context, index string, id int64, vector []float32) error {
	// Convert vector to JSON for PostgreSQL
	vectorJSON, err := json.Marshal(vector)
	if err != nil {
		return fmt.Errorf("marshaling vector: %w", err)
	}

	query := fmt.Sprintf(`
		UPDATE %s
		SET embedding = $1::vector
		WHERE id = $2
	`, index)

	_, err = p.db.Exec(query, string(vectorJSON), id)
	if err != nil {
		return fmt.Errorf("inserting vector: %w", err)
	}

	return nil
}

// InsertVectors performs batch vector insertion using prepared statements.
func (p *PgVectorDB) InsertVectors(ctx context.Context, index string, ids []int64, vectors [][]float32) error {
	if len(ids) != len(vectors) {
		return fmt.Errorf("ids and vectors length mismatch: %d != %d", len(ids), len(vectors))
	}

	// Use a transaction for batch operations
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	query := fmt.Sprintf(`
		UPDATE %s
		SET embedding = $1::vector
		WHERE id = $2
	`, index)

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	for i, id := range ids {
		vectorJSON, err := json.Marshal(vectors[i])
		if err != nil {
			return fmt.Errorf("marshaling vector %d: %w", i, err)
		}

		if _, err := stmt.Exec(string(vectorJSON), id); err != nil {
			return fmt.Errorf("inserting vector %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing batch insert: %w", err)
	}

	return nil
}

// SearchKNN performs k-nearest-neighbor search using pgvector distance operators.
func (p *PgVectorDB) SearchKNN(ctx context.Context, index string, query []float32, k int) ([]VectorSearchResult, error) {
	// Convert query vector to JSON
	queryJSON, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("marshaling query vector: %w", err)
	}

	// Select the distance operator based on metric
	var distanceOp string
	switch p.metric {
	case DistanceCosine:
		distanceOp = "<=>"
	case DistanceEuclidean:
		distanceOp = "<->"
	case DistanceDotProduct:
		distanceOp = "<#>"
	default:
		return nil, fmt.Errorf("unsupported distance metric: %s", p.metric.String())
	}

	// Build the KNN query
	// pgvector automatically uses the index if available
	sql := fmt.Sprintf(`
		SELECT id, embedding %s $1::vector AS distance
		FROM %s
		ORDER BY embedding %s $1::vector
		LIMIT $2
	`, distanceOp, index, distanceOp)

	rows, err := p.db.Query(sql, string(queryJSON), k)
	if err != nil {
		return nil, fmt.Errorf("executing KNN query: %w", err)
	}
	defer rows.Close()

	var results []VectorSearchResult
	for rows.Next() {
		var result VectorSearchResult
		if err := rows.Scan(&result.ID, &result.Distance); err != nil {
			return nil, fmt.Errorf("scanning result: %w", err)
		}

		// Calculate normalized score (higher = more similar)
		result.Score = 1.0 / (1.0 + result.Distance)

		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating results: %w", err)
	}

	return results, nil
}

// DeleteVector removes a vector from the embeddings table.
// This sets the embedding to NULL rather than deleting the row.
func (p *PgVectorDB) DeleteVector(ctx context.Context, index string, id int64) error {
	query := fmt.Sprintf(`
		UPDATE %s
		SET embedding = NULL
		WHERE id = $1
	`, index)

	_, err := p.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("deleting vector: %w", err)
	}

	return nil
}

// DeleteVectors removes multiple vectors in a batch.
func (p *PgVectorDB) DeleteVectors(ctx context.Context, index string, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	// Build IN clause with placeholders
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = p.dialect.Placeholder(i + 1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		UPDATE %s
		SET embedding = NULL
		WHERE id IN (%s)
	`, index, joinStrings(placeholders, ", "))

	_, err := p.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("deleting vectors: %w", err)
	}

	return nil
}

// SupportsNativeSearch returns true (pgvector uses native database operations).
func (p *PgVectorDB) SupportsNativeSearch() bool {
	return true
}

// joinStrings is a helper to join strings (like strings.Join but avoids import).
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// Verify interface compliance at compile time.
var _ VectorDB = (*PgVectorDB)(nil)
