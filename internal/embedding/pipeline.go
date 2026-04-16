package embedding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"
)

// EmbedResult contains statistics from an embedding operation.
type EmbedResult struct {
	Total        int           `json:"total"`          // Total chunks processed
	CacheHits    int           `json:"cache_hits"`     // Embeddings found in cache
	Embedded     int           `json:"embedded"`       // New embeddings generated
	Skipped      int           `json:"skipped"`        // Chunks skipped (e.g., empty)
	Errors       int           `json:"errors"`         // Chunks that failed
	Truncated    int           `json:"truncated"`      // Chunks recovered via sub-chunking
	FailedChunks int           `json:"failed_chunks"`  // Chunks that failed even after sub-chunking
	Duration     time.Duration `json:"duration"`       // Total processing time
	EmbedTime    time.Duration `json:"embed_time"`     // Time spent on embedding API
	CacheTime    time.Duration `json:"cache_time"`     // Time spent on cache operations
	HitRate      float64       `json:"hit_rate"`       // Cache hit percentage
	ChunksPerSec float64       `json:"chunks_per_sec"` // Throughput
}


// Pipeline provides a cache-aware embedding pipeline.
// It coordinates between the embedding cache, location store, and embedding provider.
type Pipeline struct {
	cache        *EmbeddingCache
	locations    LocationAccess
	embedder     Embedder
	failureStore *FailureStore

	// Configuration
	batchSize     int
	maxWorkers    int
	charsPerToken float64       // 0 means use default (2.5)
	tokenCounter  *TokenCounter // nil = use char estimation (Ollama/fallback)
}

// PipelineOption configures a Pipeline.
type PipelineOption func(*Pipeline)

// WithBatchSize sets the batch size for embedding API calls.
func WithBatchSize(size int) PipelineOption {
	return func(p *Pipeline) {
		if size > 0 {
			p.batchSize = size
		}
	}
}

// WithFailureStore sets the failure store for persisting embedding failures.
func WithFailureStore(fs *FailureStore) PipelineOption {
	return func(p *Pipeline) {
		p.failureStore = fs
	}
}

// WithTokenCounter sets an exact token counter for the pipeline.
// When set, it replaces character-based estimation for pre-filter decisions.
// Pass nil to use character-based estimation (default, used for Ollama).
func WithTokenCounter(tc *TokenCounter) PipelineOption {
	return func(p *Pipeline) {
		p.tokenCounter = tc
	}
}

// WithCharsPerToken sets the chars/token ratio for token estimation.
// Use DefaultCharsPerTokenLiteLLM (1.0) for OpenAI/LiteLLM providers.
func WithCharsPerToken(ratio float64) PipelineOption {
	return func(p *Pipeline) {
		if ratio > 0 {
			p.charsPerToken = ratio
		}
	}
}

// WithMaxWorkers sets the maximum number of concurrent embedding workers.
func WithMaxWorkers(workers int) PipelineOption {
	return func(p *Pipeline) {
		if workers > 0 {
			p.maxWorkers = workers
		}
	}
}

// NewPipeline creates a new embedding pipeline.
func NewPipeline(cache *EmbeddingCache, locations LocationAccess, embedder Embedder, opts ...PipelineOption) *Pipeline {
	p := &Pipeline{
		cache:      cache,
		locations:  locations,
		embedder:   embedder,
		batchSize:  50, // Default batch size
		maxWorkers: 1,  // Default single worker
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// PipelineChunk extends Chunk with content hash for pipeline processing.
// If ContentHash is empty, it will be computed from Content.
type PipelineChunk struct {
	Chunk
	ContentHash string `json:"content_hash"`
}

// EmbedChunks processes chunks through the content-addressed cache pipeline.
// 1. Computes content hashes for all chunks
// 2. Batch looks up existing embeddings in cache
// 3. Embeds only chunks not found in cache
// 4. Stores new embeddings in cache
// 5. Records all chunk locations
//
// This achieves near-100% cache hit rate on unchanged code.
func (p *Pipeline) EmbedChunks(ctx context.Context, repoRoot string, chunks []Chunk) (*EmbedResult, error) {
	start := time.Now()
	result := &EmbedResult{
		Total: len(chunks),
	}

	if len(chunks) == 0 {
		return result, nil
	}

	// 1. Convert to pipeline chunks with content hashes
	pChunks := make([]PipelineChunk, len(chunks))
	for i, chunk := range chunks {
		pChunks[i] = PipelineChunk{
			Chunk:       chunk,
			ContentHash: HashContent(chunk.Content),
		}
	}

	// 2. Collect unique hashes
	hashSet := make(map[string]bool)
	for _, pc := range pChunks {
		if pc.Content == "" {
			result.Skipped++
			continue
		}
		hashSet[pc.ContentHash] = true
	}

	uniqueHashes := make([]string, 0, len(hashSet))
	for hash := range hashSet {
		uniqueHashes = append(uniqueHashes, hash)
	}

	// 3. Batch lookup existing embeddings
	cacheStart := time.Now()
	existing, err := p.cache.GetBatch(uniqueHashes)
	if err != nil {
		return nil, fmt.Errorf("cache lookup failed: %w", err)
	}
	result.CacheHits = len(existing)
	result.CacheTime = time.Since(cacheStart)

	// 4. Identify chunks needing embedding
	toEmbed := make([]PipelineChunk, 0)
	for _, pc := range pChunks {
		if pc.Content == "" {
			continue
		}
		if _, found := existing[pc.ContentHash]; !found {
			toEmbed = append(toEmbed, pc)
		}
	}

	// 5. Embed new chunks
	if len(toEmbed) > 0 {
		// Count unique hashes sent to embed
		toEmbedUnique := make(map[string]bool)
		for _, pc := range toEmbed {
			toEmbedUnique[pc.ContentHash] = true
		}

		embedStart := time.Now()
		newEmbeddings, eStats, err := p.embedNewChunks(ctx, toEmbed, repoRoot)
		if err != nil {
			return nil, fmt.Errorf("embedding failed: %w", err)
		}
		result.EmbedTime = time.Since(embedStart)

		// Track sub-chunking recovery and true failures
		result.Truncated = eStats.Recovered
		result.FailedChunks = eStats.Failed
		result.Errors = len(toEmbedUnique) - len(newEmbeddings)

		// 6. Store in cache
		if len(newEmbeddings) > 0 {
			cacheStoreStart := time.Now()
			if err := p.cache.PutBatch(newEmbeddings); err != nil {
				return nil, fmt.Errorf("cache store failed: %w", err)
			}
			result.CacheTime += time.Since(cacheStoreStart)
		}

		result.Embedded = len(newEmbeddings)
	}

	// 7. Save all chunk locations
	locations := make([]ChunkLocation, 0, len(pChunks)-result.Skipped)
	for _, pc := range pChunks {
		if pc.Content == "" {
			continue
		}
		locations = append(locations, ChunkLocation{
			RepoRoot:    repoRoot,
			Path:        pc.Path,
			StartLine:   pc.StartLine,
			EndLine:     pc.EndLine,
			ContentHash: pc.ContentHash,
			NodeType:    pc.Kind,
			NodeName:    "", // Could be extracted from chunk metadata
			Language:    detectLanguage(pc.Path),
		})
	}

	if err := p.locations.SaveLocationsBatch(locations); err != nil {
		return nil, fmt.Errorf("location store failed: %w", err)
	}

	// Calculate final stats
	result.Duration = time.Since(start)
	processed := result.Total - result.Skipped
	if processed > 0 {
		result.HitRate = float64(result.CacheHits) / float64(processed) * 100
		result.ChunksPerSec = float64(processed) / result.Duration.Seconds()
	}

	return result, nil
}

// embedStats tracks sub-chunking outcomes from embedNewChunks.
type embedStats struct {
	Recovered int // hashes recovered via sub-chunking
	Failed    int // hashes that failed even after sub-chunking
}

// embedNewChunks embeds chunks that weren't found in cache.
// Returns a map of content hash -> embedding for successfully embedded chunks.
// Chunks that fail direct embedding are retried via recursive sub-chunking.
// The first sub-chunk's embedding is used as representative for the original chunk.
func (p *Pipeline) embedNewChunks(ctx context.Context, chunks []PipelineChunk, repoRoot string) (map[string][]float32, *embedStats, error) {
	stats := &embedStats{}
	if len(chunks) == 0 {
		return make(map[string][]float32), stats, nil
	}

	// Deduplicate by hash (multiple chunks may have same content)
	hashToContent := make(map[string]string)
	hashToChunk := make(map[string]PipelineChunk)
	for _, pc := range chunks {
		hashToContent[pc.ContentHash] = pc.Content
		hashToChunk[pc.ContentHash] = pc
	}

	// Convert to slices for batch embedding
	hashes := make([]string, 0, len(hashToContent))
	contents := make([]string, 0, len(hashToContent))
	for hash, content := range hashToContent {
		hashes = append(hashes, hash)
		contents = append(contents, content)
	}

	// Pre-embed guard: route oversized chunks directly to sub-chunking
	// instead of truncating them (which would lose data).
	result := make(map[string][]float32)
	var failedHashes []string     // failed during batch embed — will retry via sub-chunking
	var trulyFailedHashes []string // already tried sub-chunking and failed
	maxChars := MaxCharsForTokensWithRatio(DefaultMaxTokens, p.charsPerToken)

	var normalHashes []string
	var normalContents []string
	for idx, content := range contents {
		if p.exceedsTokenLimit(content, maxChars) {
			slog.Info("sub-chunking oversized chunk before embedding",
				"hash", hashes[idx][:12], "content_len", len(content), "max_chars", maxChars)
			subEmbeddings := p.splitAndEmbed(ctx, content)
			if len(subEmbeddings) > 0 {
				result[hashes[idx]] = subEmbeddings[0]
				stats.Recovered++
			} else {
				trulyFailedHashes = append(trulyFailedHashes, hashes[idx])
			}
		} else {
			normalHashes = append(normalHashes, hashes[idx])
			normalContents = append(normalContents, content)
		}
	}

	// Embed normal-sized chunks in batches, tolerating per-batch failures
	for i := 0; i < len(normalContents); i += p.batchSize {
		end := i + p.batchSize
		if end > len(normalContents) {
			end = len(normalContents)
		}

		batchContents := make([]string, end-i)
		copy(batchContents, normalContents[i:end])
		batchHashes := normalHashes[i:end]

		embeddings, err := p.embedder.Embed(ctx, batchContents)
		if err != nil {
			// Entire batch failed — queue all for sub-chunking recovery
			slog.Debug("batch embedding failed, queuing for sub-chunking",
				"batch", fmt.Sprintf("%d-%d", i, end),
				"error", err)
			failedHashes = append(failedHashes, batchHashes...)
			continue
		}

		for j, emb := range embeddings {
			if emb != nil {
				result[batchHashes[j]] = emb
			} else {
				failedHashes = append(failedHashes, batchHashes[j])
			}
		}
	}

	// Try iterative splitting for chunks that failed during batch embedding
	for _, hash := range failedHashes {
		content := hashToContent[hash]
		subEmbeddings := p.splitAndEmbed(ctx, content)
		if len(subEmbeddings) > 0 {
			// Use first sub-chunk's embedding as representative
			result[hash] = subEmbeddings[0]
			stats.Recovered++
			slog.Info("recovered chunk via sub-chunking",
				"hash", hash[:12],
				"sub_chunks", len(subEmbeddings),
				"content_len", len(content))
		} else {
			trulyFailedHashes = append(trulyFailedHashes, hash)
		}
	}

	// Record true failures (failed even after sub-chunking)
	for _, hash := range trulyFailedHashes {
		stats.Failed++
		content := hashToContent[hash]
		pc := hashToChunk[hash]
		slog.Warn("chunk failed even after sub-chunking",
			"path", pc.Path,
			"lines", fmt.Sprintf("%d-%d", pc.StartLine, pc.EndLine),
			"content_len", len(content),
			"estimated_tokens", EstimateTokens(content))

		if p.failureStore != nil {
			model := p.embedder.ProviderID()
			numPieces := int(math.Ceil(float64(len(content)) / float64(maxChars)))
			_ = p.failureStore.RecordFailure(
				repoRoot, pc.Path, pc.StartLine, pc.EndLine,
				content, "embedding failed after sub-chunking",
				model, numPieces, // TODO: rename column max_depth_reached to num_pieces_attempted
			)
		}
	}

	return result, stats, nil
}

// splitAndEmbed splits text into correctly-sized pieces and embeds them.
// Uses iterative equal splitting (no recursion or depth limits).
// Returns all successful embeddings. Returns nil if nothing can be embedded.
func (p *Pipeline) splitAndEmbed(ctx context.Context, text string) [][]float32 {
	if len(text) == 0 {
		return nil
	}

	maxChars := MaxCharsForTokensWithRatio(DefaultMaxTokens, p.charsPerToken)

	// Small text: embed directly
	if len(text) <= maxChars {
		embs, err := p.embedder.Embed(ctx, []string{text})
		if err != nil {
			return nil
		}
		if len(embs) > 0 && embs[0] != nil {
			return embs
		}
		return nil
	}

	// Calculate number of pieces needed
	numPieces := int(math.Ceil(float64(len(text)) / float64(maxChars)))
	targetSize := len(text) / numPieces

	// Split at nearest newline boundaries
	var pieces []string
	start := 0
	for i := 0; i < numPieces-1; i++ {
		target := start + targetSize
		if target >= len(text) {
			break
		}

		// Find nearest newline to target position
		splitIdx := target
		leftNL := strings.LastIndex(text[start:target], "\n")
		rightNL := strings.Index(text[target:], "\n")

		if leftNL >= 0 && rightNL >= 0 {
			leftAbs := start + leftNL + 1
			rightAbs := target + rightNL + 1
			if target-leftAbs <= rightAbs-target {
				splitIdx = leftAbs
			} else {
				splitIdx = rightAbs
			}
		} else if leftNL >= 0 {
			splitIdx = start + leftNL + 1
		} else if rightNL >= 0 {
			splitIdx = target + rightNL + 1
		}

		// Don't create empty pieces
		if splitIdx <= start {
			splitIdx = target
		}
		if splitIdx >= len(text) {
			break
		}

		pieces = append(pieces, text[start:splitIdx])
		start = splitIdx
	}
	// Last piece
	if start < len(text) {
		pieces = append(pieces, text[start:])
	}

	// Defensive post-split check: if any piece exceeds maxChars,
	// re-split it at raw char-boundary intervals
	var finalPieces []string
	for _, piece := range pieces {
		if len(piece) <= maxChars {
			finalPieces = append(finalPieces, piece)
		} else {
			// Re-split oversized piece at char boundaries
			for j := 0; j < len(piece); j += maxChars {
				end := j + maxChars
				if end > len(piece) {
					end = len(piece)
				}
				finalPieces = append(finalPieces, piece[j:end])
			}
		}
	}

	// Batch embed all pieces
	embs, err := p.embedder.Embed(ctx, finalPieces)
	if err != nil {
		slog.Warn("splitAndEmbed: batch embed failed",
			"pieces", len(finalPieces), "error", err)
		return nil
	}

	var result [][]float32
	for i, emb := range embs {
		if emb != nil {
			result = append(result, emb)
		} else {
			slog.Warn("splitAndEmbed: nil embedding for piece",
				"piece_index", i, "piece_len", len(finalPieces[i]))
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// EmbedFile processes a single file through the pipeline.
// Convenience method that chunks the file and processes chunks.
func (p *Pipeline) EmbedFile(ctx context.Context, repoRoot, path string, config ChunkerConfig) (*EmbedResult, error) {
	chunks, err := ChunkFileSimple(path, config)
	if err != nil {
		return nil, fmt.Errorf("chunking file: %w", err)
	}

	return p.EmbedChunks(ctx, repoRoot, chunks)
}

// ReindexFile re-indexes a file, removing old locations first.
func (p *Pipeline) ReindexFile(ctx context.Context, repoRoot, path string, chunks []Chunk) (*EmbedResult, error) {
	// Delete old locations for this file
	if err := p.locations.DeleteByPath(repoRoot, path); err != nil {
		return nil, fmt.Errorf("deleting old locations: %w", err)
	}

	// Process new chunks
	return p.EmbedChunks(ctx, repoRoot, chunks)
}

// ReindexRepo re-indexes an entire repository.
// Clears all locations and processes new chunks.
func (p *Pipeline) ReindexRepo(ctx context.Context, repoRoot string, chunks []Chunk) (*EmbedResult, error) {
	// Delete all locations for this repo
	if err := p.locations.DeleteByRepo(repoRoot); err != nil {
		return nil, fmt.Errorf("deleting repo locations: %w", err)
	}

	// Process new chunks
	return p.EmbedChunks(ctx, repoRoot, chunks)
}

// IncrementalUpdate updates specific files while preserving others.
// Only processes files that have changed based on content hash comparison.
func (p *Pipeline) IncrementalUpdate(ctx context.Context, repoRoot string, files map[string][]Chunk) (*EmbedResult, error) {
	totalResult := &EmbedResult{}
	start := time.Now()

	for path, chunks := range files {
		// Get existing hashes for this file
		existingHashes, err := p.locations.GetHashesForPath(repoRoot, path)
		if err != nil {
			return nil, fmt.Errorf("getting hashes for %s: %w", path, err)
		}

		// Compute new hashes
		newHashes := make(map[string]bool)
		for _, chunk := range chunks {
			hash := HashContent(chunk.Content)
			newHashes[hash] = true
		}

		// Check if file has changed
		existingSet := make(map[string]bool)
		for _, h := range existingHashes {
			existingSet[h] = true
		}

		changed := len(existingHashes) != len(chunks)
		if !changed {
			for hash := range newHashes {
				if !existingSet[hash] {
					changed = true
					break
				}
			}
		}

		if !changed {
			// File unchanged, skip
			totalResult.CacheHits += len(chunks)
			totalResult.Total += len(chunks)
			continue
		}

		// File changed, re-index
		result, err := p.ReindexFile(ctx, repoRoot, path, chunks)
		if err != nil {
			return nil, fmt.Errorf("reindexing %s: %w", path, err)
		}

		// Aggregate results
		totalResult.Total += result.Total
		totalResult.CacheHits += result.CacheHits
		totalResult.Embedded += result.Embedded
		totalResult.Skipped += result.Skipped
		totalResult.Errors += result.Errors
		totalResult.Truncated += result.Truncated
		totalResult.FailedChunks += result.FailedChunks
		totalResult.EmbedTime += result.EmbedTime
		totalResult.CacheTime += result.CacheTime
	}

	totalResult.Duration = time.Since(start)
	if totalResult.Total > 0 {
		totalResult.HitRate = float64(totalResult.CacheHits) / float64(totalResult.Total) * 100
		totalResult.ChunksPerSec = float64(totalResult.Total) / totalResult.Duration.Seconds()
	}

	return totalResult, nil
}

// CleanupOrphanedEmbeddings removes embeddings not referenced by any location.
func (p *Pipeline) CleanupOrphanedEmbeddings(ctx context.Context) (int, error) {
	// Get all cache hashes
	stats, err := p.cache.Stats()
	if err != nil {
		return 0, fmt.Errorf("getting cache stats: %w", err)
	}

	if stats.TotalEntries == 0 {
		return 0, nil
	}

	// This is a simplified approach - in production, you'd want to
	// iterate through cache entries in batches
	// For now, we'll skip this optimization

	return 0, nil
}

// ParallelEmbedChunks embeds chunks using multiple workers.
// Use this for large batch operations.
func (p *Pipeline) ParallelEmbedChunks(ctx context.Context, repoRoot string, chunks []Chunk) (*EmbedResult, error) {
	if p.maxWorkers <= 1 {
		// Fall back to sequential processing for single worker
		return p.EmbedChunks(ctx, repoRoot, chunks)
	}

	start := time.Now()
	result := &EmbedResult{
		Total: len(chunks),
	}

	// Convert to pipeline chunks with hashes
	pChunks := make([]PipelineChunk, len(chunks))
	for i, chunk := range chunks {
		pChunks[i] = PipelineChunk{
			Chunk:       chunk,
			ContentHash: HashContent(chunk.Content),
		}
	}

	// Collect unique hashes
	hashSet := make(map[string]bool)
	for _, pc := range pChunks {
		if pc.Content == "" {
			result.Skipped++
			continue
		}
		hashSet[pc.ContentHash] = true
	}

	uniqueHashes := make([]string, 0, len(hashSet))
	for hash := range hashSet {
		uniqueHashes = append(uniqueHashes, hash)
	}

	// Batch lookup existing embeddings
	existing, err := p.cache.GetBatch(uniqueHashes)
	if err != nil {
		return nil, fmt.Errorf("cache lookup failed: %w", err)
	}
	result.CacheHits = len(existing)

	// Identify chunks needing embedding
	var toEmbed []PipelineChunk
	for _, pc := range pChunks {
		if pc.Content == "" {
			continue
		}
		if _, found := existing[pc.ContentHash]; !found {
			toEmbed = append(toEmbed, pc)
		}
	}

	// Parallel embedding
	if len(toEmbed) > 0 {
		// Split into work items
		workItems := p.packBatchByTokens(toEmbed)

		// Create worker pool
		results := make(chan map[string][]float32, len(workItems))
		errors := make(chan error, len(workItems))

		var wg sync.WaitGroup
		sem := make(chan struct{}, p.maxWorkers)

		for _, batch := range workItems {
			wg.Add(1)
			go func(batch []PipelineChunk) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				embeddings, _, err := p.embedNewChunks(ctx, batch, repoRoot)
				if err != nil {
					errors <- err
					return
				}
				results <- embeddings
			}(batch)
		}

		// Wait for all workers
		go func() {
			wg.Wait()
			close(results)
			close(errors)
		}()

		// Collect results
		allEmbeddings := make(map[string][]float32)
		for emb := range results {
			for hash, vec := range emb {
				allEmbeddings[hash] = vec
			}
		}

		// Check for errors
		for err := range errors {
			if err != nil {
				return nil, err
			}
		}

		// Store in cache
		if err := p.cache.PutBatch(allEmbeddings); err != nil {
			return nil, fmt.Errorf("cache store failed: %w", err)
		}

		result.Embedded = len(allEmbeddings)
	}

	// Save all chunk locations
	locations := make([]ChunkLocation, 0, len(pChunks)-result.Skipped)
	for _, pc := range pChunks {
		if pc.Content == "" {
			continue
		}
		locations = append(locations, ChunkLocation{
			RepoRoot:    repoRoot,
			Path:        pc.Path,
			StartLine:   pc.StartLine,
			EndLine:     pc.EndLine,
			ContentHash: pc.ContentHash,
			NodeType:    pc.Kind,
			Language:    detectLanguage(pc.Path),
		})
	}

	if err := p.locations.SaveLocationsBatch(locations); err != nil {
		return nil, fmt.Errorf("location store failed: %w", err)
	}

	// Calculate final stats
	result.Duration = time.Since(start)
	processed := result.Total - result.Skipped
	if processed > 0 {
		result.HitRate = float64(result.CacheHits) / float64(processed) * 100
		result.ChunksPerSec = float64(processed) / result.Duration.Seconds()
	}

	return result, nil
}

// splitIntoBatches splits chunks into batches of the given size.
func splitIntoBatches(chunks []PipelineChunk, batchSize int) [][]PipelineChunk {
	var batches [][]PipelineChunk
	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batches = append(batches, chunks[i:end])
	}
	return batches
}

// exceedsTokenLimit returns true if content exceeds the token limit.
// Uses exact tiktoken counting when available, falls back to char estimation.
func (p *Pipeline) exceedsTokenLimit(content string, maxChars int) bool {
	if p.tokenCounter != nil {
		return p.tokenCounter.ExceedsLimit(content, DefaultMaxTokens)
	}
	return maxChars > 0 && len(content) > maxChars
}

// maxTokensPerBatch is the total token budget per API request.
// OpenAI's limit is 2M tokens/min; 100K per batch is safe at any concurrency.
const maxTokensPerBatch = 100_000

// packBatchByTokens groups chunks into batches by total token budget rather than
// fixed chunk count. When a TokenCounter is available, this packs small chunks
// more densely and isolates large chunks, reducing total API calls.
// Falls back to fixed-size batching when no TokenCounter is set.
func (p *Pipeline) packBatchByTokens(chunks []PipelineChunk) [][]PipelineChunk {
	if p.tokenCounter == nil {
		return splitIntoBatches(chunks, p.batchSize)
	}

	var batches [][]PipelineChunk
	var current []PipelineChunk
	var currentTokens int

	for _, chunk := range chunks {
		tokens := p.tokenCounter.CountTokens(chunk.Chunk.Content)
		if currentTokens+tokens > maxTokensPerBatch && len(current) > 0 {
			batches = append(batches, current)
			current = nil
			currentTokens = 0
		}
		current = append(current, chunk)
		currentTokens += tokens
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

// HashContent computes SHA-256 hash of content.
// Exported for use by other packages.
func HashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// detectLanguage guesses the programming language from file extension.
func detectLanguage(path string) string {
	// Simple extension-based detection
	ext := ""
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			ext = path[i+1:]
			break
		}
	}

	switch ext {
	case "go":
		return "go"
	case "py":
		return "python"
	case "js":
		return "javascript"
	case "ts":
		return "typescript"
	case "tsx":
		return "typescript"
	case "jsx":
		return "javascript"
	case "rs":
		return "rust"
	case "java":
		return "java"
	case "c":
		return "c"
	case "cpp", "cc", "cxx":
		return "cpp"
	case "h", "hpp":
		return "cpp"
	case "rb":
		return "ruby"
	case "php":
		return "php"
	case "swift":
		return "swift"
	case "kt":
		return "kotlin"
	case "scala":
		return "scala"
	case "cs":
		return "csharp"
	case "sh", "bash":
		return "shell"
	case "sql":
		return "sql"
	case "yaml", "yml":
		return "yaml"
	case "json":
		return "json"
	case "xml":
		return "xml"
	case "md":
		return "markdown"
	default:
		return "unknown"
	}
}

// SetMaxWorkers updates the maximum number of concurrent embedding workers.
func (p *Pipeline) SetMaxWorkers(workers int) {
	if workers > 0 {
		p.maxWorkers = workers
	}
}

// Cache returns the underlying embedding cache.
func (p *Pipeline) Cache() *EmbeddingCache {
	return p.cache
}

// Locations returns the underlying location store.
func (p *Pipeline) Locations() LocationAccess {
	return p.locations
}

// Embedder returns the underlying embedder.
func (p *Pipeline) Embedder() Embedder {
	return p.embedder
}

// FailureStore returns the underlying failure store (may be nil).
func (p *Pipeline) FailureStore() *FailureStore {
	return p.failureStore
}
