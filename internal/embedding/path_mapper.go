package embedding

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// PathMapper maintains a bidirectional hash-to-path mapping, persisted
// to a local sidecar JSON file. It is used by HashingLocationStore to
// convert between real paths and their SHA-256 hashes.
type PathMapper struct {
	mu         sync.RWMutex
	filePath   string
	hashToPath map[string]string
	pathToHash map[string]string
}

// pathMapFile is the on-disk JSON format for the sidecar file.
type pathMapFile struct {
	Version  int               `json:"version"`
	Mappings map[string]string `json:"mappings"` // hash → path
}

// NewPathMapper creates a PathMapper, loading existing mappings from filePath
// if it exists. The parent directory is created if needed.
func NewPathMapper(filePath string) (*PathMapper, error) {
	pm := &PathMapper{
		filePath:   filePath,
		hashToPath: make(map[string]string),
		pathToHash: make(map[string]string),
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, fmt.Errorf("creating path mapper directory: %w", err)
	}

	// Load existing mappings if file exists
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return pm, nil // No existing file, start empty
		}
		return nil, fmt.Errorf("reading path map: %w", err)
	}

	var f pathMapFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing path map: %w", err)
	}

	for hash, path := range f.Mappings {
		pm.hashToPath[hash] = path
		pm.pathToHash[path] = hash
	}

	return pm, nil
}

// HashPath returns the SHA-256 hash of realPath and registers the mapping.
// Thread-safe.
func (pm *PathMapper) HashPath(realPath string) string {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Fast path: already mapped
	if h, ok := pm.pathToHash[realPath]; ok {
		return h
	}

	h := sha256.Sum256([]byte(realPath))
	hash := hex.EncodeToString(h[:])
	pm.hashToPath[hash] = realPath
	pm.pathToHash[realPath] = hash
	return hash
}

// ResolvePath returns the real path for a hash. Returns ("", false) if unknown.
// Thread-safe.
func (pm *PathMapper) ResolvePath(hash string) (string, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	path, ok := pm.hashToPath[hash]
	return path, ok
}

// Flush persists the current mappings to disk atomically.
func (pm *PathMapper) Flush() error {
	pm.mu.RLock()
	mappings := make(map[string]string, len(pm.hashToPath))
	for k, v := range pm.hashToPath {
		mappings[k] = v
	}
	pm.mu.RUnlock()

	f := pathMapFile{
		Version:  1,
		Mappings: mappings,
	}

	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshaling path map: %w", err)
	}

	// Write atomically via temp file
	tmp := pm.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing path map: %w", err)
	}
	if err := os.Rename(tmp, pm.filePath); err != nil {
		return fmt.Errorf("renaming path map: %w", err)
	}
	return nil
}
