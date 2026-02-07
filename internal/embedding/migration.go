package embedding

import (
	"context"
	"encoding/json"
	"fmt"

	"codetect/internal/db"
)

// ========================================
// Type Migration (vector format changes)
// ========================================
// Migrates embeddings from TEXT (JSON) to native vector type.
// Used when upgrading from JSON storage to pgvector.

// MigrateToVectorType migrates embeddings from TEXT (JSON) to native vector type.
// This is useful when migrating from SQLite to PostgreSQL or when upgrading
// an existing PostgreSQL database that was using TEXT storage.
//
// The migration process:
// 1. Creates a new temporary table with vector column
// 2. Copies data, converting JSON arrays to vector format
// 3. Drops old table and renames new table
//
// WARNING: This operation requires a table lock and may take time for large datasets.
func (s *EmbeddingStore) MigrateToVectorType() error {
	if !s.useNativeVec {
		return fmt.Errorf("migration only supported for PostgreSQL with pgvector")
	}

	// Check if already using vector type
	hasVectorType, err := s.checkIfVectorType()
	if err != nil {
		return fmt.Errorf("checking current schema: %w", err)
	}
	if hasVectorType {
		return nil // Already migrated
	}

	// Create temporary table with vector type
	tempColumns := embeddingColumnsForDialect(s.dialect, s.vectorDim)
	tempTableSQL := s.dialect.CreateTableSQL("embeddings_new", tempColumns)

	if _, err := s.db.Exec(tempTableSQL); err != nil {
		return fmt.Errorf("creating temporary table: %w", err)
	}

	// Copy data with type conversion
	// PostgreSQL can cast JSON array string to vector automatically
	copySQL := `
		INSERT INTO embeddings_new (id, path, start_line, end_line, content_hash, embedding, model, created_at)
		SELECT id, path, start_line, end_line, content_hash, embedding::vector, model, created_at
		FROM embeddings
	`

	if _, err := s.db.Exec(copySQL); err != nil {
		// Rollback: drop temporary table
		s.db.Exec("DROP TABLE embeddings_new")
		return fmt.Errorf("copying data to new table: %w", err)
	}

	// Start transaction for the swap
	tx, err := s.db.Begin()
	if err != nil {
		s.db.Exec("DROP TABLE embeddings_new")
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Drop old table
	if _, err := tx.Exec("DROP TABLE embeddings"); err != nil {
		return fmt.Errorf("dropping old table: %w", err)
	}

	// Rename new table
	if _, err := tx.Exec("ALTER TABLE embeddings_new RENAME TO embeddings"); err != nil {
		return fmt.Errorf("renaming new table: %w", err)
	}

	// Recreate indexes
	idxPath := s.dialect.CreateIndexSQL("embeddings", "idx_embeddings_path", []string{"path"}, false)
	if _, err := tx.Exec(idxPath); err != nil {
		return fmt.Errorf("creating path index: %w", err)
	}

	idxHash := s.dialect.CreateIndexSQL("embeddings", "idx_embeddings_hash", []string{"content_hash"}, false)
	if _, err := tx.Exec(idxHash); err != nil {
		return fmt.Errorf("creating hash index: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration: %w", err)
	}

	return nil
}

// checkIfVectorType checks if the embeddings table is already using vector type.
func (s *EmbeddingStore) checkIfVectorType() (bool, error) {
	// Query PostgreSQL information schema to check column type
	query := `
		SELECT data_type
		FROM information_schema.columns
		WHERE table_name = 'embeddings'
		AND column_name = 'embedding'
	`

	var dataType string
	err := s.db.QueryRow(query).Scan(&dataType)
	if err != nil {
		return false, err
	}

	// PostgreSQL pgvector type shows as "USER-DEFINED"
	return dataType == "USER-DEFINED", nil
}

// ValidateTypeMigration validates that all embeddings were migrated correctly
// from JSON to vector type by comparing the data format.
func (s *EmbeddingStore) ValidateTypeMigration(sampleSize int) error {
	if !s.useNativeVec {
		return fmt.Errorf("validation only supported for PostgreSQL with pgvector")
	}

	// Get a sample of embeddings
	query := fmt.Sprintf(`
		SELECT embedding
		FROM embeddings
		LIMIT %d
	`, sampleSize)

	rows, err := s.db.Query(query)
	if err != nil {
		return fmt.Errorf("querying embeddings: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var vectorStr string
		if err := rows.Scan(&vectorStr); err != nil {
			return fmt.Errorf("scanning embedding %d: %w", count, err)
		}

		// Parse vector string (pgvector format: [1,2,3,...])
		var vector []float32
		if err := json.Unmarshal([]byte(vectorStr), &vector); err != nil {
			return fmt.Errorf("parsing vector %d: %w", count, err)
		}

		// Check dimensions
		if len(vector) != s.vectorDim {
			return fmt.Errorf("vector %d has incorrect dimensions: got %d, want %d",
				count, len(vector), s.vectorDim)
		}

		count++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating embeddings: %w", err)
	}

	return nil
}

// EstimateMigrationTime provides an estimate of how long migration will take
// based on the number of embeddings.
func (s *EmbeddingStore) EstimateMigrationTime() (embeddingCount int, estimatedSeconds int, err error) {
	embeddingCount, err = s.Count()
	if err != nil {
		return 0, 0, err
	}

	// Rough estimate: ~1000 embeddings per second for type conversion
	estimatedSeconds = embeddingCount / 1000
	if estimatedSeconds < 1 {
		estimatedSeconds = 1
	}

	return embeddingCount, estimatedSeconds, nil
}

// ========================================
// Database Migration (SQLite -> PostgreSQL)
// ========================================
// Migrates embeddings from one database to another.
// Used when moving from SQLite to PostgreSQL.

// MigrationOptions configures database migration behavior.
type MigrationOptions struct {
	// BatchSize controls how many embeddings to migrate at once
	BatchSize int

	// SkipExisting skips embeddings that already exist in the target
	SkipExisting bool

	// DropTarget drops the target database tables before migration
	DropTarget bool

	// DryRun performs validation without actually migrating data
	DryRun bool
}

// DefaultMigrationOptions returns sensible defaults for migration.
func DefaultMigrationOptions() MigrationOptions {
	return MigrationOptions{
		BatchSize:    1000,
		SkipExisting: true,
		DropTarget:   false,
		DryRun:       false,
	}
}

// MigrationProgress tracks the progress of a database migration.
type MigrationProgress struct {
	TotalEmbeddings    int
	MigratedEmbeddings int
	SkippedEmbeddings  int
	FailedEmbeddings   int
	CurrentFile        string
}

// MigrationCallback is called periodically during migration to report progress.
type MigrationCallback func(progress MigrationProgress)

// MigrateDatabase migrates all embeddings from one database to another.
// This is useful for migrating from SQLite to PostgreSQL.
//
// Example:
//
//	sourceStore, _ := NewEmbeddingStore(sqliteDB)
//	targetStore, _ := NewEmbeddingStoreWithDialect(postgresDB, postgresDialect)
//	err := MigrateDatabase(ctx, sourceStore, targetStore, opts, progressCallback)
func MigrateDatabase(
	ctx context.Context,
	source *EmbeddingStore,
	target *EmbeddingStore,
	opts MigrationOptions,
	callback MigrationCallback,
) error {
	// Validate options
	if opts.BatchSize <= 0 {
		opts.BatchSize = 1000
	}

	// Initialize target schema if needed
	if opts.DropTarget && !opts.DryRun {
		if err := target.DeleteAll(); err != nil {
			return fmt.Errorf("clearing target database: %w", err)
		}
	}

	// Get total count for progress tracking
	totalCount, err := source.Count()
	if err != nil {
		return fmt.Errorf("counting source embeddings: %w", err)
	}

	progress := MigrationProgress{
		TotalEmbeddings: totalCount,
	}

	if opts.DryRun {
		if callback != nil {
			callback(progress)
		}
		return nil
	}

	// Get all embeddings from source
	// For large datasets, this should be paginated, but for now we load all
	embeddings, err := source.GetAll()
	if err != nil {
		return fmt.Errorf("fetching source embeddings: %w", err)
	}

	// Process in batches
	for i := 0; i < len(embeddings); i += opts.BatchSize {
		end := i + opts.BatchSize
		if end > len(embeddings) {
			end = len(embeddings)
		}

		batch := embeddings[i:end]

		// Convert to chunks and vectors for batch insertion
		chunks := make([]Chunk, len(batch))
		vectors := make([][]float32, len(batch))
		model := ""

		for j, emb := range batch {
			chunks[j] = Chunk{
				Path:      emb.Path,
				StartLine: emb.StartLine,
				EndLine:   emb.EndLine,
				Content:   "", // Not needed for migration
			}
			vectors[j] = emb.Embedding
			if model == "" {
				model = emb.Model
			}

			// Update progress
			if emb.Path != progress.CurrentFile {
				progress.CurrentFile = emb.Path
			}
		}

		// Check for existing embeddings if SkipExisting is enabled
		if opts.SkipExisting {
			// Filter out existing embeddings
			filteredChunks := make([]Chunk, 0, len(chunks))
			filteredVectors := make([][]float32, 0, len(vectors))

			for j, chunk := range chunks {
				exists, err := target.HasEmbedding(chunk, model)
				if err != nil {
					return fmt.Errorf("checking existing embedding: %w", err)
				}

				if !exists {
					filteredChunks = append(filteredChunks, chunk)
					filteredVectors = append(filteredVectors, vectors[j])
				} else {
					progress.SkippedEmbeddings++
				}
			}

			chunks = filteredChunks
			vectors = filteredVectors
		}

		// Save batch to target
		if len(chunks) > 0 {
			if err := target.SaveBatch(chunks, vectors, model); err != nil {
				progress.FailedEmbeddings += len(chunks)
				return fmt.Errorf("saving batch to target: %w", err)
			}

			progress.MigratedEmbeddings += len(chunks)
		}

		// Report progress
		if callback != nil {
			callback(progress)
		}

		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// MigrateDatabaseWithVectorIndex migrates embeddings and creates a vector index
// on the target database (if it supports native vector search).
func MigrateDatabaseWithVectorIndex(
	ctx context.Context,
	source *EmbeddingStore,
	target *EmbeddingStore,
	opts MigrationOptions,
	callback MigrationCallback,
) error {
	// First, migrate the data
	if err := MigrateDatabase(ctx, source, target, opts, callback); err != nil {
		return err
	}

	// If target is PostgreSQL, migrate to vector type and create index
	if target.useNativeVec && !opts.DryRun {
		// Migrate to native vector type (if not already)
		if err := target.MigrateToVectorType(); err != nil {
			return fmt.Errorf("migrating to vector type: %w", err)
		}

		// Create vector index using PgVectorDB
		vdb, err := db.NewPgVectorDB(target.db, target.vectorDim, db.DistanceCosine)
		if err != nil {
			return fmt.Errorf("creating vector database: %w", err)
		}

		if err := vdb.CreateVectorIndex(ctx, "embeddings", target.vectorDim, db.DistanceCosine); err != nil {
			return fmt.Errorf("creating vector index: %w", err)
		}
	}

	return nil
}

// ========================================
// Validation
// ========================================

// ValidateDatabaseMigration validates that a migration was successful by comparing
// embedding counts and sampling random embeddings.
func ValidateDatabaseMigration(source *EmbeddingStore, target *EmbeddingStore, sampleSize int) error {
	// Compare counts
	sourceCount, err := source.Count()
	if err != nil {
		return fmt.Errorf("counting source embeddings: %w", err)
	}

	targetCount, err := target.Count()
	if err != nil {
		return fmt.Errorf("counting target embeddings: %w", err)
	}

	if sourceCount != targetCount {
		return fmt.Errorf("embedding count mismatch: source=%d, target=%d", sourceCount, targetCount)
	}

	// Sample random embeddings and compare
	sourceEmbeddings, err := source.GetAll()
	if err != nil {
		return fmt.Errorf("fetching source embeddings: %w", err)
	}

	if len(sourceEmbeddings) == 0 {
		return nil // Empty database, nothing to validate
	}

	// Sample at most sampleSize embeddings
	step := len(sourceEmbeddings) / sampleSize
	if step < 1 {
		step = 1
	}

	for i := 0; i < len(sourceEmbeddings); i += step {
		srcEmb := sourceEmbeddings[i]

		// Find matching embedding in target
		targetEmbeddings, err := target.GetByPath(srcEmb.Path)
		if err != nil {
			return fmt.Errorf("fetching target embeddings for %s: %w", srcEmb.Path, err)
		}

		// Find exact match
		found := false
		for _, tgtEmb := range targetEmbeddings {
			if tgtEmb.StartLine == srcEmb.StartLine &&
				tgtEmb.EndLine == srcEmb.EndLine &&
				tgtEmb.Model == srcEmb.Model {
				found = true

				// Compare embedding dimensions
				if len(tgtEmb.Embedding) != len(srcEmb.Embedding) {
					return fmt.Errorf("embedding dimension mismatch for %s:%d-%d",
						srcEmb.Path, srcEmb.StartLine, srcEmb.EndLine)
				}

				// Compare first few values (floating point comparison with tolerance)
				compareCount := min(10, len(srcEmb.Embedding))
				for j := range compareCount {
					diff := srcEmb.Embedding[j] - tgtEmb.Embedding[j]
					if diff < 0 {
						diff = -diff
					}
					if diff > 0.0001 {
						return fmt.Errorf("embedding value mismatch for %s:%d-%d at index %d",
							srcEmb.Path, srcEmb.StartLine, srcEmb.EndLine, j)
					}
				}

				break
			}
		}

		if !found {
			return fmt.Errorf("embedding not found in target: %s:%d-%d",
				srcEmb.Path, srcEmb.StartLine, srcEmb.EndLine)
		}
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
