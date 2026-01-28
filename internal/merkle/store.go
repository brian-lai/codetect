package merkle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TreeFileName is the default name for the persisted Merkle tree.
const TreeFileName = "merkle-tree.json"

// Store handles persistence of Merkle trees to disk.
// Trees are stored as JSON files in the data directory,
// typically .codetect/ within the repository.
type Store struct {
	dataDir string
}

// NewStore creates a store that persists data to the given directory.
// The directory will be created if it doesn't exist.
func NewStore(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

// Save persists the tree to disk as JSON.
// The file is written atomically using a temp file + rename.
func (s *Store) Save(tree *Tree) error {
	if tree == nil {
		return fmt.Errorf("cannot save nil tree")
	}

	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Marshal with indentation for human readability
	data, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tree: %w", err)
	}

	// Write atomically using temp file + rename
	targetPath := filepath.Join(s.dataDir, TreeFileName)
	tempPath := targetPath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tempPath, targetPath); err != nil {
		os.Remove(tempPath) // Clean up on failure
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// Load reads a tree from disk.
// Returns nil, nil if no tree exists (first run).
// Returns error only for actual read/parse failures.
func (s *Store) Load() (*Tree, error) {
	path := filepath.Join(s.dataDir, TreeFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No previous tree is not an error
		}
		return nil, fmt.Errorf("read tree file: %w", err)
	}

	var tree Tree
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("unmarshal tree: %w", err)
	}

	return &tree, nil
}

// Exists returns true if a tree file exists.
func (s *Store) Exists() bool {
	path := filepath.Join(s.dataDir, TreeFileName)
	_, err := os.Stat(path)
	return err == nil
}

// Delete removes the stored tree file.
func (s *Store) Delete() error {
	path := filepath.Join(s.dataDir, TreeFileName)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil // Already deleted is not an error
	}
	return err
}

// Path returns the path to the tree file.
func (s *Store) Path() string {
	return filepath.Join(s.dataDir, TreeFileName)
}

// Metadata contains information about a stored tree without loading it fully.
type Metadata struct {
	Path      string    // Path to the tree file
	Size      int64     // File size in bytes
	ModTime   time.Time // Last modification time
	FileCount int       // Number of files in the tree
	RootHash  string    // Root hash of the tree
}

// GetMetadata returns metadata about the stored tree without fully loading it.
// This is useful for quick checks without the overhead of parsing the entire tree.
func (s *Store) GetMetadata() (*Metadata, error) {
	path := filepath.Join(s.dataDir, TreeFileName)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// We need to load and parse to get file count and root hash
	tree, err := s.Load()
	if err != nil {
		return nil, err
	}

	return &Metadata{
		Path:      path,
		Size:      info.Size(),
		ModTime:   info.ModTime(),
		FileCount: tree.FileCount,
		RootHash:  tree.RootHash(),
	}, nil
}

// SaveWithBackup saves the tree and keeps a backup of the previous version.
func (s *Store) SaveWithBackup(tree *Tree) error {
	currentPath := filepath.Join(s.dataDir, TreeFileName)
	backupPath := filepath.Join(s.dataDir, TreeFileName+".backup")

	// If current file exists, rename it to backup
	if _, err := os.Stat(currentPath); err == nil {
		if err := os.Rename(currentPath, backupPath); err != nil {
			return fmt.Errorf("backup existing tree: %w", err)
		}
	}

	// Save new tree
	if err := s.Save(tree); err != nil {
		// Try to restore backup on failure
		if _, backupErr := os.Stat(backupPath); backupErr == nil {
			os.Rename(backupPath, currentPath)
		}
		return err
	}

	return nil
}

// LoadBackup loads the backup tree if it exists.
func (s *Store) LoadBackup() (*Tree, error) {
	path := filepath.Join(s.dataDir, TreeFileName+".backup")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read backup file: %w", err)
	}

	var tree Tree
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("unmarshal backup: %w", err)
	}

	return &tree, nil
}
