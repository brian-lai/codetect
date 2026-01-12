package db

import "context"

// VectorDB provides an interface for vector similarity search operations.
// This abstraction allows switching between different vector search backends:
//   - Brute-force (Go implementation, current default)
//   - sqlite-vec (SQLite extension)
//   - pgvector (PostgreSQL extension)
//   - ClickHouse (native array operations)
//   - External services (Pinecone, Milvus, etc.)
type VectorDB interface {
	// CreateVectorIndex creates a vector index for the given table/collection.
	// dimensions: the size of the vectors (e.g., 768 for nomic-embed-text)
	// metric: the distance metric to use for similarity
	CreateVectorIndex(ctx context.Context, name string, dimensions int, metric DistanceMetric) error

	// InsertVector inserts a vector with an associated ID.
	// Returns the assigned ID if successful.
	InsertVector(ctx context.Context, index string, id int64, vector []float32) error

	// InsertVectors performs batch vector insertion.
	InsertVectors(ctx context.Context, index string, ids []int64, vectors [][]float32) error

	// SearchKNN performs k-nearest-neighbor search.
	// Returns results sorted by distance ascending (closest first).
	SearchKNN(ctx context.Context, index string, query []float32, k int) ([]VectorSearchResult, error)

	// DeleteVector removes a vector by ID.
	DeleteVector(ctx context.Context, index string, id int64) error

	// DeleteVectors removes multiple vectors by ID.
	DeleteVectors(ctx context.Context, index string, ids []int64) error

	// SupportsNativeSearch returns true if the backend supports native vector operations.
	// If false, search will fall back to brute-force.
	SupportsNativeSearch() bool
}

// VectorSearchResult represents a single result from vector similarity search.
type VectorSearchResult struct {
	ID       int64   // The ID of the matching vector
	Distance float32 // Distance/similarity score (lower = more similar for most metrics)
	Score    float32 // Normalized similarity score (higher = more similar, 0-1 range)
}

// DistanceMetric defines the similarity/distance function for vector comparison.
type DistanceMetric int

const (
	// DistanceCosine uses cosine similarity (1 - cosine_similarity).
	// Good for normalized embeddings, most common for text embeddings.
	DistanceCosine DistanceMetric = iota

	// DistanceEuclidean uses L2 (Euclidean) distance.
	// sqrt(sum((a_i - b_i)^2))
	DistanceEuclidean

	// DistanceDotProduct uses negative dot product.
	// Suitable when vectors are normalized.
	DistanceDotProduct

	// DistanceManhattan uses L1 (Manhattan) distance.
	// sum(|a_i - b_i|)
	DistanceManhattan
)

// String returns the string representation of the distance metric.
func (d DistanceMetric) String() string {
	switch d {
	case DistanceCosine:
		return "cosine"
	case DistanceEuclidean:
		return "euclidean"
	case DistanceDotProduct:
		return "dot_product"
	case DistanceManhattan:
		return "manhattan"
	default:
		return "unknown"
	}
}

// BruteForceVectorDB implements VectorDB using in-memory brute-force search.
// This is the fallback implementation when native vector search is not available.
type BruteForceVectorDB struct {
	vectors map[string]map[int64][]float32 // index -> id -> vector
	metric  map[string]DistanceMetric      // index -> metric
}

// NewBruteForceVectorDB creates a new brute-force vector database.
func NewBruteForceVectorDB() *BruteForceVectorDB {
	return &BruteForceVectorDB{
		vectors: make(map[string]map[int64][]float32),
		metric:  make(map[string]DistanceMetric),
	}
}

// CreateVectorIndex creates a new index for storing vectors.
func (b *BruteForceVectorDB) CreateVectorIndex(_ context.Context, name string, _ int, metric DistanceMetric) error {
	if _, exists := b.vectors[name]; !exists {
		b.vectors[name] = make(map[int64][]float32)
	}
	b.metric[name] = metric
	return nil
}

// InsertVector inserts a single vector.
func (b *BruteForceVectorDB) InsertVector(_ context.Context, index string, id int64, vector []float32) error {
	if _, exists := b.vectors[index]; !exists {
		b.vectors[index] = make(map[int64][]float32)
	}
	// Make a copy to avoid external modification
	v := make([]float32, len(vector))
	copy(v, vector)
	b.vectors[index][id] = v
	return nil
}

// InsertVectors inserts multiple vectors.
func (b *BruteForceVectorDB) InsertVectors(ctx context.Context, index string, ids []int64, vectors [][]float32) error {
	for i, id := range ids {
		if err := b.InsertVector(ctx, index, id, vectors[i]); err != nil {
			return err
		}
	}
	return nil
}

// SearchKNN performs k-nearest-neighbor search using brute force.
func (b *BruteForceVectorDB) SearchKNN(_ context.Context, index string, query []float32, k int) ([]VectorSearchResult, error) {
	idx, exists := b.vectors[index]
	if !exists {
		return nil, nil
	}

	metric := b.metric[index]
	distFunc := b.getDistanceFunc(metric)

	// Calculate distances to all vectors
	type distPair struct {
		id   int64
		dist float32
	}
	var pairs []distPair
	for id, vec := range idx {
		dist := distFunc(query, vec)
		pairs = append(pairs, distPair{id, dist})
	}

	// Sort by distance ascending (simple bubble sort for small k)
	for i := range len(pairs) - 1 {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].dist < pairs[i].dist {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}

	// Take top k
	if k > len(pairs) {
		k = len(pairs)
	}

	results := make([]VectorSearchResult, k)
	for i := range k {
		results[i] = VectorSearchResult{
			ID:       pairs[i].id,
			Distance: pairs[i].dist,
			Score:    1.0 / (1.0 + pairs[i].dist), // Convert distance to similarity score
		}
	}

	return results, nil
}

// DeleteVector removes a vector by ID.
func (b *BruteForceVectorDB) DeleteVector(_ context.Context, index string, id int64) error {
	if idx, exists := b.vectors[index]; exists {
		delete(idx, id)
	}
	return nil
}

// DeleteVectors removes multiple vectors by ID.
func (b *BruteForceVectorDB) DeleteVectors(ctx context.Context, index string, ids []int64) error {
	for _, id := range ids {
		if err := b.DeleteVector(ctx, index, id); err != nil {
			return err
		}
	}
	return nil
}

// SupportsNativeSearch returns false (brute-force is not native).
func (b *BruteForceVectorDB) SupportsNativeSearch() bool {
	return false
}

// getDistanceFunc returns the distance function for the given metric.
func (b *BruteForceVectorDB) getDistanceFunc(metric DistanceMetric) func(a, b []float32) float32 {
	switch metric {
	case DistanceEuclidean:
		return euclideanDistance
	case DistanceDotProduct:
		return negativeDotProduct
	case DistanceManhattan:
		return manhattanDistance
	default: // DistanceCosine
		return cosineDistance
	}
}

// cosineDistance calculates 1 - cosine_similarity
func cosineDistance(a, b []float32) float32 {
	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 1.0
	}
	return 1.0 - (dotProduct / (sqrt32(normA) * sqrt32(normB)))
}

// euclideanDistance calculates L2 distance
func euclideanDistance(a, b []float32) float32 {
	var sum float32
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return sqrt32(sum)
}

// negativeDotProduct returns negative dot product (for max similarity)
func negativeDotProduct(a, b []float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return -sum
}

// manhattanDistance calculates L1 distance
func manhattanDistance(a, b []float32) float32 {
	var sum float32
	for i := range a {
		diff := a[i] - b[i]
		if diff < 0 {
			diff = -diff
		}
		sum += diff
	}
	return sum
}

// sqrt32 calculates square root for float32
func sqrt32(x float32) float32 {
	if x <= 0 {
		return 0
	}
	// Newton-Raphson method
	z := x / 2
	for range 10 {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// Verify interface compliance at compile time.
var _ VectorDB = (*BruteForceVectorDB)(nil)
