package embedding

// LocationAccess defines the interface for chunk location storage.
// Both LocationStore (no hashing) and HashingLocationStore (hashing wrapper)
// satisfy this interface. Consumers should depend on this interface to
// enable transparent path hashing when CODETECT_HASH_PATHS=true.
type LocationAccess interface {
	SaveLocation(loc ChunkLocation) error
	SaveLocationsBatch(locs []ChunkLocation) error
	GetByPath(repoRoot, path string) ([]ChunkLocation, error)
	GetByRepo(repoRoot string) ([]ChunkLocation, error)
	GetByHash(contentHash string) ([]ChunkLocation, error)
	DeleteByPath(repoRoot, path string) error
	DeleteByRepo(repoRoot string) error
	GetHashesForRepo(repoRoot string) ([]string, error)
	GetHashesForPath(repoRoot, path string) ([]string, error)
	CountByRepo(repoRoot string) (int, error)
	CountByPath(repoRoot, path string) (int, error)
	ListPaths(repoRoot string) ([]string, error)
	GetLocationsBySymbol(repoRoot, nodeName string) ([]ChunkLocation, error)
	GetLocationsByType(repoRoot, nodeType string) ([]ChunkLocation, error)
	Stats(repoRoot string) (*LocationStats, error)
	GetOrphanedHashes(allCacheHashes []string) ([]string, error)
}

// Verify that LocationStore implements LocationAccess at compile time.
var _ LocationAccess = (*LocationStore)(nil)
