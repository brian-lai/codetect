package embedding

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"codetect/internal/db"
)

// ChunkLocation represents where a code chunk appears in a repository.
// Multiple locations can reference the same content hash, enabling
// deduplication when code is copied, moved, or appears in multiple repos.
type ChunkLocation struct {
	ID          int64     `json:"id"`
	RepoRoot    string    `json:"repo_root"`
	Path        string    `json:"path"`
	StartLine   int       `json:"start_line"`
	EndLine     int       `json:"end_line"`
	ContentHash string    `json:"content_hash"` // FK to embedding_cache
	NodeType    string    `json:"node_type"`    // AST node type (function, class, etc.)
	NodeName    string    `json:"node_name"`    // Symbol name
	Language    string    `json:"language"`
	CreatedAt   time.Time `json:"created_at"`
}

// LocationStore manages chunk locations in the database.
// Locations are stored separately from embeddings to enable:
// - Tracking where chunks appear across files/repos
// - Efficient file-based queries (what chunks are in this file?)
// - Content deduplication (same code in different places)
type LocationStore struct {
	database db.DB
	dialect  db.Dialect
	schema   *db.SchemaBuilder
	mu       sync.RWMutex
}

// NewLocationStore creates a new location store.
func NewLocationStore(database db.DB, dialect db.Dialect) (*LocationStore, error) {
	store := &LocationStore{
		database: database,
		dialect:  dialect,
		schema:   db.NewSchemaBuilder(database, dialect),
	}

	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("initializing location schema: %w", err)
	}

	return store, nil
}

// initSchema creates the chunk_locations table if it doesn't exist.
func (s *LocationStore) initSchema() error {
	columns := []db.ColumnDef{
		{Name: "id", Type: db.ColTypeAutoIncrement},
		{Name: "repo_root", Type: db.ColTypeText, Nullable: false},
		{Name: "path", Type: db.ColTypeText, Nullable: false},
		{Name: "start_line", Type: db.ColTypeInteger, Nullable: false},
		{Name: "end_line", Type: db.ColTypeInteger, Nullable: false},
		{Name: "content_hash", Type: db.ColTypeText, Nullable: false},
		{Name: "node_type", Type: db.ColTypeText, Nullable: true},
		{Name: "node_name", Type: db.ColTypeText, Nullable: true},
		{Name: "language", Type: db.ColTypeText, Nullable: true},
		{Name: "created_at", Type: db.ColTypeInteger, Nullable: false},
	}

	// Create table
	createSQL := s.dialect.CreateTableSQL("chunk_locations", columns)
	if _, err := s.database.Exec(createSQL); err != nil {
		return fmt.Errorf("creating chunk_locations table: %w", err)
	}

	// Create unique constraint for upserts (repo, path, start, end)
	idxUnique := s.dialect.CreateIndexSQL("chunk_locations", "idx_chunk_locations_unique",
		[]string{"repo_root", "path", "start_line", "end_line"}, true)
	if _, err := s.database.Exec(idxUnique); err != nil {
		return fmt.Errorf("creating unique index: %w", err)
	}

	// Create index for repo-scoped queries
	idxRepo := s.dialect.CreateIndexSQL("chunk_locations", "idx_chunk_locations_repo",
		[]string{"repo_root"}, false)
	if _, err := s.database.Exec(idxRepo); err != nil {
		return fmt.Errorf("creating repo index: %w", err)
	}

	// Create index for path-based queries (finding chunks in a file)
	idxPath := s.dialect.CreateIndexSQL("chunk_locations", "idx_chunk_locations_path",
		[]string{"repo_root", "path"}, false)
	if _, err := s.database.Exec(idxPath); err != nil {
		return fmt.Errorf("creating path index: %w", err)
	}

	// Create index for content hash (finding all locations of a chunk)
	idxHash := s.dialect.CreateIndexSQL("chunk_locations", "idx_chunk_locations_hash",
		[]string{"content_hash"}, false)
	if _, err := s.database.Exec(idxHash); err != nil {
		return fmt.Errorf("creating hash index: %w", err)
	}

	return nil
}

// SaveLocation records a chunk location, updating if it exists.
func (s *LocationStore) SaveLocation(loc ChunkLocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if loc.CreatedAt.IsZero() {
		loc.CreatedAt = time.Now()
	}

	// Use upsert for idempotent saves
	columns := []string{"repo_root", "path", "start_line", "end_line", "content_hash",
		"node_type", "node_name", "language", "created_at"}
	conflictColumns := []string{"repo_root", "path", "start_line", "end_line"}
	updateColumns := []string{"content_hash", "node_type", "node_name", "language"}

	upsertSQL := s.dialect.UpsertSQL("chunk_locations", columns, conflictColumns, updateColumns)
	upsertSQL = s.schema.SubstitutePlaceholders(upsertSQL)

	_, err := s.database.Exec(upsertSQL,
		loc.RepoRoot, loc.Path, loc.StartLine, loc.EndLine, loc.ContentHash,
		nullString(loc.NodeType), nullString(loc.NodeName), nullString(loc.Language), now,
	)

	return err
}

// SaveLocationsBatch saves multiple locations in a transaction.
func (s *LocationStore) SaveLocationsBatch(locs []ChunkLocation) error {
	if len(locs) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.database.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	columns := []string{"repo_root", "path", "start_line", "end_line", "content_hash",
		"node_type", "node_name", "language", "created_at"}
	conflictColumns := []string{"repo_root", "path", "start_line", "end_line"}
	updateColumns := []string{"content_hash", "node_type", "node_name", "language"}

	upsertSQL := s.dialect.UpsertSQL("chunk_locations", columns, conflictColumns, updateColumns)
	upsertSQL = s.schema.SubstitutePlaceholders(upsertSQL)

	stmt, err := tx.Prepare(upsertSQL)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, loc := range locs {
		_, err := stmt.Exec(
			loc.RepoRoot, loc.Path, loc.StartLine, loc.EndLine, loc.ContentHash,
			nullString(loc.NodeType), nullString(loc.NodeName), nullString(loc.Language), now,
		)
		if err != nil {
			return fmt.Errorf("inserting location for %s:%d-%d: %w",
				loc.Path, loc.StartLine, loc.EndLine, err)
		}
	}

	return tx.Commit()
}

// GetByPath retrieves all chunk locations for a file.
func (s *LocationStore) GetByPath(repoRoot, path string) ([]ChunkLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.schema.SubstitutePlaceholders(`
		SELECT id, repo_root, path, start_line, end_line, content_hash,
		       node_type, node_name, language, created_at
		FROM chunk_locations
		WHERE repo_root = ? AND path = ?
		ORDER BY start_line
	`)

	rows, err := s.database.Query(query, repoRoot, path)
	if err != nil {
		return nil, fmt.Errorf("querying locations: %w", err)
	}
	defer rows.Close()

	return scanLocations(rows)
}

// GetByRepo retrieves all chunk locations for a repository.
func (s *LocationStore) GetByRepo(repoRoot string) ([]ChunkLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.schema.SubstitutePlaceholders(`
		SELECT id, repo_root, path, start_line, end_line, content_hash,
		       node_type, node_name, language, created_at
		FROM chunk_locations
		WHERE repo_root = ?
		ORDER BY path, start_line
	`)

	rows, err := s.database.Query(query, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("querying locations: %w", err)
	}
	defer rows.Close()

	return scanLocations(rows)
}

// GetByHash retrieves all locations where a content hash appears.
// Useful for finding duplicated code across files/repos.
func (s *LocationStore) GetByHash(contentHash string) ([]ChunkLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.schema.SubstitutePlaceholders(`
		SELECT id, repo_root, path, start_line, end_line, content_hash,
		       node_type, node_name, language, created_at
		FROM chunk_locations
		WHERE content_hash = ?
		ORDER BY repo_root, path, start_line
	`)

	rows, err := s.database.Query(query, contentHash)
	if err != nil {
		return nil, fmt.Errorf("querying locations: %w", err)
	}
	defer rows.Close()

	return scanLocations(rows)
}

// DeleteByPath removes all locations for a file.
// Called when a file is re-indexed or deleted.
func (s *LocationStore) DeleteByPath(repoRoot, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := s.schema.SubstitutePlaceholders(
		"DELETE FROM chunk_locations WHERE repo_root = ? AND path = ?",
	)
	_, err := s.database.Exec(query, repoRoot, path)
	return err
}

// DeleteByRepo removes all locations for a repository.
func (s *LocationStore) DeleteByRepo(repoRoot string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := s.schema.SubstitutePlaceholders(
		"DELETE FROM chunk_locations WHERE repo_root = ?",
	)
	_, err := s.database.Exec(query, repoRoot)
	return err
}

// GetHashesForRepo returns all unique content hashes used in a repo.
// Useful for determining which embeddings are referenced.
func (s *LocationStore) GetHashesForRepo(repoRoot string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.schema.SubstitutePlaceholders(`
		SELECT DISTINCT content_hash FROM chunk_locations WHERE repo_root = ?
	`)

	rows, err := s.database.Query(query, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("querying hashes: %w", err)
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			continue
		}
		hashes = append(hashes, hash)
	}

	return hashes, rows.Err()
}

// GetHashesForPath returns all content hashes used in a file.
func (s *LocationStore) GetHashesForPath(repoRoot, path string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.schema.SubstitutePlaceholders(`
		SELECT DISTINCT content_hash FROM chunk_locations
		WHERE repo_root = ? AND path = ?
	`)

	rows, err := s.database.Query(query, repoRoot, path)
	if err != nil {
		return nil, fmt.Errorf("querying hashes: %w", err)
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			continue
		}
		hashes = append(hashes, hash)
	}

	return hashes, rows.Err()
}

// CountByRepo returns the number of chunk locations in a repository.
func (s *LocationStore) CountByRepo(repoRoot string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.schema.SubstitutePlaceholders(
		"SELECT COUNT(*) FROM chunk_locations WHERE repo_root = ?",
	)

	var count int
	err := s.database.QueryRow(query, repoRoot).Scan(&count)
	return count, err
}

// CountByPath returns the number of chunk locations in a file.
func (s *LocationStore) CountByPath(repoRoot, path string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.schema.SubstitutePlaceholders(
		"SELECT COUNT(*) FROM chunk_locations WHERE repo_root = ? AND path = ?",
	)

	var count int
	err := s.database.QueryRow(query, repoRoot, path).Scan(&count)
	return count, err
}

// GetOrphanedHashes returns content hashes that are no longer referenced
// by any location. These embeddings can be safely deleted from the cache.
func (s *LocationStore) GetOrphanedHashes(allCacheHashes []string) ([]string, error) {
	if len(allCacheHashes) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get all referenced hashes from chunk_locations
	referencedQuery := "SELECT DISTINCT content_hash FROM chunk_locations"
	rows, err := s.database.Query(referencedQuery)
	if err != nil {
		return nil, fmt.Errorf("querying referenced hashes: %w", err)
	}
	defer rows.Close()

	referenced := make(map[string]bool)
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			continue
		}
		referenced[hash] = true
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Find orphaned hashes
	var orphaned []string
	for _, hash := range allCacheHashes {
		if !referenced[hash] {
			orphaned = append(orphaned, hash)
		}
	}

	return orphaned, nil
}

// ListPaths returns all unique file paths in a repository.
func (s *LocationStore) ListPaths(repoRoot string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.schema.SubstitutePlaceholders(`
		SELECT DISTINCT path FROM chunk_locations
		WHERE repo_root = ?
		ORDER BY path
	`)

	rows, err := s.database.Query(query, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("querying paths: %w", err)
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			continue
		}
		paths = append(paths, path)
	}

	return paths, rows.Err()
}

// GetLocationsBySymbol finds chunks by symbol name (function name, class name, etc.)
func (s *LocationStore) GetLocationsBySymbol(repoRoot, nodeName string) ([]ChunkLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.schema.SubstitutePlaceholders(`
		SELECT id, repo_root, path, start_line, end_line, content_hash,
		       node_type, node_name, language, created_at
		FROM chunk_locations
		WHERE repo_root = ? AND node_name = ?
		ORDER BY path, start_line
	`)

	rows, err := s.database.Query(query, repoRoot, nodeName)
	if err != nil {
		return nil, fmt.Errorf("querying locations: %w", err)
	}
	defer rows.Close()

	return scanLocations(rows)
}

// GetLocationsByType finds chunks by node type (function, class, etc.)
func (s *LocationStore) GetLocationsByType(repoRoot, nodeType string) ([]ChunkLocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := s.schema.SubstitutePlaceholders(`
		SELECT id, repo_root, path, start_line, end_line, content_hash,
		       node_type, node_name, language, created_at
		FROM chunk_locations
		WHERE repo_root = ? AND node_type = ?
		ORDER BY path, start_line
	`)

	rows, err := s.database.Query(query, repoRoot, nodeType)
	if err != nil {
		return nil, fmt.Errorf("querying locations: %w", err)
	}
	defer rows.Close()

	return scanLocations(rows)
}

// scanLocations scans rows into ChunkLocation structs.
func scanLocations(rows db.Rows) ([]ChunkLocation, error) {
	var locations []ChunkLocation

	for rows.Next() {
		var loc ChunkLocation
		var createdAt int64
		var nodeType, nodeName, language sql.NullString

		err := rows.Scan(
			&loc.ID, &loc.RepoRoot, &loc.Path, &loc.StartLine, &loc.EndLine,
			&loc.ContentHash, &nodeType, &nodeName, &language, &createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning location: %w", err)
		}

		loc.NodeType = nodeType.String
		loc.NodeName = nodeName.String
		loc.Language = language.String
		loc.CreatedAt = time.Unix(createdAt, 0)

		locations = append(locations, loc)
	}

	return locations, rows.Err()
}

// nullString converts an empty string to NULL for database storage.
func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// LocationStats provides statistics about chunk locations.
type LocationStats struct {
	TotalLocations int            `json:"total_locations"`
	UniqueHashes   int            `json:"unique_hashes"`
	FileCount      int            `json:"file_count"`
	ByNodeType     map[string]int `json:"by_node_type"`
	ByLanguage     map[string]int `json:"by_language"`
}

// Stats returns statistics about locations in a repository.
func (s *LocationStore) Stats(repoRoot string) (*LocationStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &LocationStats{
		ByNodeType: make(map[string]int),
		ByLanguage: make(map[string]int),
	}

	// Total locations and unique hashes
	query := s.schema.SubstitutePlaceholders(`
		SELECT COUNT(*), COUNT(DISTINCT content_hash), COUNT(DISTINCT path)
		FROM chunk_locations
		WHERE repo_root = ?
	`)
	err := s.database.QueryRow(query, repoRoot).Scan(
		&stats.TotalLocations,
		&stats.UniqueHashes,
		&stats.FileCount,
	)
	if err != nil {
		return nil, fmt.Errorf("querying stats: %w", err)
	}

	// Count by node type
	typeQuery := s.schema.SubstitutePlaceholders(`
		SELECT COALESCE(node_type, 'unknown'), COUNT(*)
		FROM chunk_locations
		WHERE repo_root = ?
		GROUP BY node_type
	`)
	typeRows, err := s.database.Query(typeQuery, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("querying node types: %w", err)
	}
	defer typeRows.Close()

	for typeRows.Next() {
		var nodeType string
		var count int
		if err := typeRows.Scan(&nodeType, &count); err != nil {
			continue
		}
		stats.ByNodeType[nodeType] = count
	}

	// Count by language
	langQuery := s.schema.SubstitutePlaceholders(`
		SELECT COALESCE(language, 'unknown'), COUNT(*)
		FROM chunk_locations
		WHERE repo_root = ?
		GROUP BY language
	`)
	langRows, err := s.database.Query(langQuery, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("querying languages: %w", err)
	}
	defer langRows.Close()

	for langRows.Next() {
		var language string
		var count int
		if err := langRows.Scan(&language, &count); err != nil {
			continue
		}
		stats.ByLanguage[language] = count
	}

	return stats, nil
}
