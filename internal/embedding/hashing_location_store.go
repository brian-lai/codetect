package embedding

import "fmt"

// HashingLocationStore wraps a LocationStore and a PathMapper to transparently
// hash repo_root and path on writes and resolve them on reads. It implements
// the LocationAccess interface.
type HashingLocationStore struct {
	inner  *LocationStore
	mapper *PathMapper
}

// Verify interface compliance at compile time.
var _ LocationAccess = (*HashingLocationStore)(nil)

// NewHashingLocationStore creates a new hashing wrapper around a LocationStore.
func NewHashingLocationStore(inner *LocationStore, mapper *PathMapper) *HashingLocationStore {
	return &HashingLocationStore{inner: inner, mapper: mapper}
}

// --- Write methods: hash paths before delegating ---

func (h *HashingLocationStore) SaveLocation(loc ChunkLocation) error {
	loc.RepoRoot = h.mapper.HashPath(loc.RepoRoot)
	loc.Path = h.mapper.HashPath(loc.Path)
	return h.inner.SaveLocation(loc)
}

func (h *HashingLocationStore) SaveLocationsBatch(locs []ChunkLocation) error {
	hashed := make([]ChunkLocation, len(locs))
	for i, loc := range locs {
		loc.RepoRoot = h.mapper.HashPath(loc.RepoRoot)
		loc.Path = h.mapper.HashPath(loc.Path)
		hashed[i] = loc
	}
	if err := h.inner.SaveLocationsBatch(hashed); err != nil {
		return err
	}
	return h.mapper.Flush()
}

// --- Query-by-path methods: hash params, resolve results ---

func (h *HashingLocationStore) GetByPath(repoRoot, path string) ([]ChunkLocation, error) {
	hashedRepo := h.mapper.HashPath(repoRoot)
	hashedPath := h.mapper.HashPath(path)
	locs, err := h.inner.GetByPath(hashedRepo, hashedPath)
	if err != nil {
		return nil, err
	}
	return h.resolveLocations(locs), nil
}

func (h *HashingLocationStore) DeleteByPath(repoRoot, path string) error {
	hashedRepo := h.mapper.HashPath(repoRoot)
	hashedPath := h.mapper.HashPath(path)
	return h.inner.DeleteByPath(hashedRepo, hashedPath)
}

func (h *HashingLocationStore) GetHashesForPath(repoRoot, path string) ([]string, error) {
	hashedRepo := h.mapper.HashPath(repoRoot)
	hashedPath := h.mapper.HashPath(path)
	return h.inner.GetHashesForPath(hashedRepo, hashedPath)
}

func (h *HashingLocationStore) CountByPath(repoRoot, path string) (int, error) {
	hashedRepo := h.mapper.HashPath(repoRoot)
	hashedPath := h.mapper.HashPath(path)
	return h.inner.CountByPath(hashedRepo, hashedPath)
}

// --- Query-by-repo methods: hash repoRoot, resolve results ---

func (h *HashingLocationStore) GetByRepo(repoRoot string) ([]ChunkLocation, error) {
	hashedRepo := h.mapper.HashPath(repoRoot)
	locs, err := h.inner.GetByRepo(hashedRepo)
	if err != nil {
		return nil, err
	}
	return h.resolveLocations(locs), nil
}

func (h *HashingLocationStore) DeleteByRepo(repoRoot string) error {
	hashedRepo := h.mapper.HashPath(repoRoot)
	return h.inner.DeleteByRepo(hashedRepo)
}

func (h *HashingLocationStore) GetHashesForRepo(repoRoot string) ([]string, error) {
	hashedRepo := h.mapper.HashPath(repoRoot)
	return h.inner.GetHashesForRepo(hashedRepo)
}

func (h *HashingLocationStore) CountByRepo(repoRoot string) (int, error) {
	hashedRepo := h.mapper.HashPath(repoRoot)
	return h.inner.CountByRepo(hashedRepo)
}

func (h *HashingLocationStore) ListPaths(repoRoot string) ([]string, error) {
	hashedRepo := h.mapper.HashPath(repoRoot)
	hashedPaths, err := h.inner.ListPaths(hashedRepo)
	if err != nil {
		return nil, err
	}
	resolved := make([]string, 0, len(hashedPaths))
	for _, hp := range hashedPaths {
		if real, ok := h.mapper.ResolvePath(hp); ok {
			resolved = append(resolved, real)
		} else {
			resolved = append(resolved, hp) // pass through if unresolvable
		}
	}
	return resolved, nil
}

func (h *HashingLocationStore) GetLocationsBySymbol(repoRoot, nodeName string) ([]ChunkLocation, error) {
	hashedRepo := h.mapper.HashPath(repoRoot)
	locs, err := h.inner.GetLocationsBySymbol(hashedRepo, nodeName)
	if err != nil {
		return nil, err
	}
	return h.resolveLocations(locs), nil
}

func (h *HashingLocationStore) GetLocationsByType(repoRoot, nodeType string) ([]ChunkLocation, error) {
	hashedRepo := h.mapper.HashPath(repoRoot)
	locs, err := h.inner.GetLocationsByType(hashedRepo, nodeType)
	if err != nil {
		return nil, err
	}
	return h.resolveLocations(locs), nil
}

func (h *HashingLocationStore) Stats(repoRoot string) (*LocationStats, error) {
	hashedRepo := h.mapper.HashPath(repoRoot)
	return h.inner.Stats(hashedRepo)
}

// --- Query-by-hash methods: no param hashing needed, resolve result paths ---

func (h *HashingLocationStore) GetByHash(contentHash string) ([]ChunkLocation, error) {
	locs, err := h.inner.GetByHash(contentHash)
	if err != nil {
		return nil, err
	}
	return h.resolveLocations(locs), nil
}

func (h *HashingLocationStore) GetOrphanedHashes(allCacheHashes []string) ([]string, error) {
	return h.inner.GetOrphanedHashes(allCacheHashes)
}

// --- Internal helpers ---

// resolveLocations resolves hashed repo_root and path back to real paths.
func (h *HashingLocationStore) resolveLocations(locs []ChunkLocation) []ChunkLocation {
	for i := range locs {
		if real, ok := h.mapper.ResolvePath(locs[i].RepoRoot); ok {
			locs[i].RepoRoot = real
		}
		if real, ok := h.mapper.ResolvePath(locs[i].Path); ok {
			locs[i].Path = real
		}
	}
	return locs
}

// Mapper returns the underlying PathMapper (used by FailureStore integration).
func (h *HashingLocationStore) Mapper() *PathMapper {
	return h.mapper
}

// Inner returns the underlying LocationStore.
func (h *HashingLocationStore) Inner() *LocationStore {
	return h.inner
}

// String returns a description of the store for logging.
func (h *HashingLocationStore) String() string {
	return fmt.Sprintf("HashingLocationStore(mapper=%s)", h.mapper.filePath)
}
