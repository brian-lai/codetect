package embedding

import (
	"fmt"
	"time"

	"codetect/internal/db"
)

// FailedChunk records a chunk that could not be embedded even after
// recursive sub-chunking attempts.
type FailedChunk struct {
	ID               int       `json:"id"`
	RepoRoot         string    `json:"repo_root"`
	Path             string    `json:"path"`
	StartLine        int       `json:"start_line"`
	EndLine          int       `json:"end_line"`
	ContentHash      string    `json:"content_hash"`
	ContentLength    int       `json:"content_length"`
	EstimatedTokens  int       `json:"estimated_tokens"`
	ErrorMessage     string    `json:"error_message"`
	Model            string    `json:"model"`
	MaxDepthReached  int       `json:"max_depth_reached"`
	CreatedAt        time.Time `json:"created_at"`
}

// FailureStore persists embedding failures for visibility and diagnosis.
type FailureStore struct {
	database   db.DB
	dialect    db.Dialect
	pathMapper *PathMapper // optional; when non-nil, paths are hashed at rest
}

// NewFailureStore creates a new FailureStore and ensures the schema exists.
func NewFailureStore(database db.DB, dialect db.Dialect) (*FailureStore, error) {
	fs := &FailureStore{
		database: database,
		dialect:  dialect,
	}
	if err := fs.ensureSchema(); err != nil {
		return nil, fmt.Errorf("creating failed_chunks table: %w", err)
	}
	return fs, nil
}

func (fs *FailureStore) ensureSchema() error {
	query := `CREATE TABLE IF NOT EXISTS failed_chunks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		repo_root TEXT NOT NULL,
		path TEXT NOT NULL,
		start_line INTEGER,
		end_line INTEGER,
		content_hash TEXT NOT NULL,
		content_length INTEGER,
		estimated_tokens INTEGER,
		error_message TEXT,
		model TEXT,
		max_depth_reached INTEGER,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`
	_, err := fs.database.Exec(query)
	if err != nil {
		return err
	}

	// Create index for repo lookups
	_, err = fs.database.Exec(
		`CREATE INDEX IF NOT EXISTS idx_failed_chunks_repo ON failed_chunks(repo_root)`,
	)
	return err
}

// SetPathMapper enables path hashing for the failure store.
// When set, repo_root and path are hashed on write and resolved on read.
func (fs *FailureStore) SetPathMapper(pm *PathMapper) {
	fs.pathMapper = pm
}

// RecordFailure persists a failed chunk.
func (fs *FailureStore) RecordFailure(repoRoot, path string, startLine, endLine int, content, errMsg, model string, maxDepth int) error {
	query := `INSERT INTO failed_chunks
		(repo_root, path, start_line, end_line, content_hash, content_length, estimated_tokens, error_message, model, max_depth_reached)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	contentHash := HashContent(content)
	contentLen := len(content)
	tokens := EstimateTokens(content)

	storeRepo, storePath := repoRoot, path
	if fs.pathMapper != nil {
		storeRepo = fs.pathMapper.HashPath(repoRoot)
		storePath = fs.pathMapper.HashPath(path)
	}

	_, err := fs.database.Exec(query,
		storeRepo, storePath, startLine, endLine,
		contentHash, contentLen, tokens,
		errMsg, model, maxDepth,
	)
	return err
}

// GetFailures returns all failed chunks for a repository.
func (fs *FailureStore) GetFailures(repoRoot string) ([]FailedChunk, error) {
	query := `SELECT id, repo_root, path, start_line, end_line, content_hash,
		content_length, estimated_tokens, error_message, model, max_depth_reached, created_at
		FROM failed_chunks WHERE repo_root = ? ORDER BY created_at DESC`

	queryRepo := repoRoot
	if fs.pathMapper != nil {
		queryRepo = fs.pathMapper.HashPath(repoRoot)
	}

	rows, err := fs.database.Query(query, queryRepo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var failures []FailedChunk
	for rows.Next() {
		var f FailedChunk
		if err := rows.Scan(
			&f.ID, &f.RepoRoot, &f.Path, &f.StartLine, &f.EndLine,
			&f.ContentHash, &f.ContentLength, &f.EstimatedTokens,
			&f.ErrorMessage, &f.Model, &f.MaxDepthReached, &f.CreatedAt,
		); err != nil {
			return nil, err
		}
		if fs.pathMapper != nil {
			if real, ok := fs.pathMapper.ResolvePath(f.RepoRoot); ok {
				f.RepoRoot = real
			}
			if real, ok := fs.pathMapper.ResolvePath(f.Path); ok {
				f.Path = real
			}
		}
		failures = append(failures, f)
	}
	return failures, rows.Err()
}

// ClearFailures removes all failure records for a repository.
func (fs *FailureStore) ClearFailures(repoRoot string) error {
	queryRepo := repoRoot
	if fs.pathMapper != nil {
		queryRepo = fs.pathMapper.HashPath(repoRoot)
	}
	_, err := fs.database.Exec(
		`DELETE FROM failed_chunks WHERE repo_root = ?`, queryRepo,
	)
	return err
}

// CountFailures returns the number of failed chunks for a repository.
func (fs *FailureStore) CountFailures(repoRoot string) (int, error) {
	queryRepo := repoRoot
	if fs.pathMapper != nil {
		queryRepo = fs.pathMapper.HashPath(repoRoot)
	}
	row := fs.database.QueryRow(
		`SELECT COUNT(*) FROM failed_chunks WHERE repo_root = ?`, queryRepo,
	)
	var count int
	err := row.Scan(&count)
	return count, err
}

// FailureSummary provides an overview of failures for a repository.
type FailureSummary struct {
	TotalFailures int      `json:"total_failures"`
	AffectedFiles []string `json:"affected_files"`
}

// GetFailureSummary returns a summary of failures for a repository.
func (fs *FailureStore) GetFailureSummary(repoRoot string) (*FailureSummary, error) {
	count, err := fs.CountFailures(repoRoot)
	if err != nil {
		return nil, err
	}

	queryRepo := repoRoot
	if fs.pathMapper != nil {
		queryRepo = fs.pathMapper.HashPath(repoRoot)
	}

	rows, err := fs.database.Query(
		`SELECT DISTINCT path FROM failed_chunks WHERE repo_root = ? ORDER BY path`, queryRepo,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		if fs.pathMapper != nil {
			if real, ok := fs.pathMapper.ResolvePath(path); ok {
				path = real
			}
		}
		files = append(files, path)
	}

	return &FailureSummary{
		TotalFailures: count,
		AffectedFiles: files,
	}, rows.Err()
}
