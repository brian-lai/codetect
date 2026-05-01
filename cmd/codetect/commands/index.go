package commands

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/schollz/progressbar/v3"

	"codetect/internal/config"
	"codetect/internal/datadir"
	"codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/indexer"
	"codetect/internal/logging"
	"codetect/internal/registry"
	"codetect/internal/search/symbols"
)

// RunIndex implements the `codetect index` subcommand.
// Moved verbatim from cmd/codetect-index/main.go:runIndex.
func RunIndex(args []string) ExitCode {
	logger := logging.Default("codetect")
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	force := fs.Bool("force", false, "Force full reindex")
	fs.BoolVar(force, "f", false, "Short for --force")
	clearCache := fs.Bool("clear-cache", false, "Clear embedding cache before indexing")
	useV1 := fs.Bool("v1", false, "Use legacy v1 indexer (ctags-based, deprecated)")
	verbose := fs.Bool("verbose", false, "Enable verbose output")
	fs.BoolVar(verbose, "v", false, "Short for --verbose")
	jsonOutput := fs.Bool("json", false, "Output results as JSON")
	parallel := fs.Int("parallel", 0, "Number of parallel workers (0 = auto)")
	fs.IntVar(parallel, "j", 0, "Short for --parallel")
	fs.Parse(args)

	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		logger.Error("invalid path", "error", err)
		return ExitError
	}

	if !*useV1 {
		return runIndexV2(logger, absPath, *force, *clearCache, *verbose, *jsonOutput, *parallel)
	}

	// V1 path: legacy ctags-based symbol indexing (deprecated)
	logger.Warn("⚠️  Using legacy v1 indexer (deprecated)")
	logger.Warn("    The v1 indexer is deprecated and will be removed in a future release")
	logger.Warn("    Remove --v1 flag to use the default AST-based indexer")

	if !symbols.CtagsAvailable() {
		logger.Warn("universal-ctags not found, symbol indexing will be skipped",
			"install", "brew install universal-ctags (macOS)")
		return ExitOK
	}

	dbConfig := config.LoadDatabaseConfigFromEnv()
	if dbConfig.Type == db.DatabaseSQLite {
		dd, err := datadir.ForRepo(absPath)
		if err != nil {
			logger.Error("resolving data directory failed", "error", err)
			return ExitError
		}
		dbConfig.Path = filepath.Join(dd, "symbols.db")
	}

	cfg := dbConfig.ToDBConfig()
	logger.Info("indexing", "path", absPath, "database", dbConfig.String())
	start := time.Now()

	idx, err := symbols.NewIndexWithConfig(cfg, absPath)
	if err != nil {
		logger.Error("opening index failed", "error", err)
		return ExitError
	}
	defer idx.Close()

	if *force {
		logger.Info("running full reindex")
		if err := idx.FullReindex(absPath); err != nil {
			logger.Error("indexing failed", "error", err)
			return ExitError
		}
	} else {
		logger.Info("running incremental index")
		if err := idx.Update(absPath); err != nil {
			logger.Error("indexing failed", "error", err)
			return ExitError
		}
	}

	symbolCount, fileCount, err := idx.Stats()
	if err != nil {
		logger.Warn("could not get stats", "error", err)
	} else {
		elapsed := time.Since(start)
		logger.Info("indexing complete",
			"symbols", symbolCount,
			"files", fileCount,
			"duration", elapsed.Round(time.Millisecond))
	}
	return ExitOK
}

func runIndexV2(logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
}, absPath string, force, clearCache, verbose, jsonOutput bool, parallel int) ExitCode {
	dbConfig := config.LoadDatabaseConfigFromEnv()
	embConfig := embedding.LoadConfigFromEnv()
	privacyConfig := config.LoadPrivacyConfigFromEnv()

	cfg := &indexer.Config{
		DBType:            string(dbConfig.Type),
		Dimensions:        dbConfig.VectorDimensions,
		EmbeddingProvider: string(embConfig.Provider),
		EmbeddingModel:    embConfig.Model,
		OllamaURL:         embConfig.OllamaURL,
		LiteLLMURL:        embConfig.LiteLLMURL,
		LiteLLMKey:        embConfig.LiteLLMKey,
		BatchSize:         32,
		MaxWorkers:        4,
		HashPaths:         privacyConfig.HashPaths,
		Parallel:          parallel,
	}

	if dbConfig.Type == db.DatabasePostgres {
		cfg.DSN = dbConfig.DSN
	} else {
		dd, err := datadir.ForRepo(absPath)
		if err != nil {
			logger.Error("resolving data directory failed", "error", err)
			return ExitError
		}
		cfg.DBPath = filepath.Join(dd, "index.db")
	}

	cfg.IgnorePatterns = indexer.LoadGitignore(absPath)

	if verbose {
		logger.Info("v2 indexer starting",
			"path", absPath,
			"db_type", cfg.DBType,
			"embedding_provider", cfg.EmbeddingProvider,
			"embedding_model", cfg.EmbeddingModel)
	}

	idx, err := indexer.New(absPath, cfg)
	if err != nil {
		logger.Error("creating v2 indexer failed", "error", err)
		return ExitError
	}
	defer idx.Close()

	if clearCache {
		logger.Warn("clearing embedding cache — all chunks will be re-embedded")
		if cache := idx.Cache(); cache != nil {
			if err := cache.Clear(); err != nil {
				logger.Error("failed to clear embedding cache", "error", err)
				return ExitError
			}
			count, _ := cache.Count()
			if verbose {
				logger.Info("embedding cache cleared", "remaining", count)
			}
		}
	}

	var progressBar *progressbar.ProgressBar
	var progressCallback indexer.ProgressCallback

	if isatty.IsTerminal(os.Stderr.Fd()) && !verbose {
		progressBar = progressbar.NewOptions(-1,
			progressbar.OptionSetDescription("Indexing..."),
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionOnCompletion(func() {
				fmt.Fprintf(os.Stderr, "\n")
			}),
			progressbar.OptionSpinnerType(14),
			progressbar.OptionFullWidth(),
			progressbar.OptionThrottle(65*time.Millisecond),
		)

		currentStage := ""
		progressCallback = func(stage string, current, total int) {
			if stage != currentStage {
				progressBar.Describe(stage)
				currentStage = stage
			}
			if total > 0 {
				progressBar.ChangeMax(total)
				progressBar.Set(current)
			} else {
				progressBar.Add(1)
			}
		}
	}

	ctx := context.Background()
	result, err := idx.Index(ctx, indexer.IndexOptions{
		Force:    force,
		Verbose:  verbose,
		Progress: progressCallback,
	})
	if err != nil {
		logger.Error("v2 indexing failed", "error", err)
		return ExitError
	}

	if progressBar != nil {
		progressBar.Finish()
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			logger.Error("encoding JSON failed", "error", err)
			return ExitError
		}
		return ExitOK
	}

	switch result.ChangeType {
	case "none":
		logger.Info("no changes detected, index is up to date")
	case "incremental":
		logger.Info("incremental index complete",
			"files_processed", result.FilesProcessed,
			"files_deleted", result.FilesDeleted,
			"chunks_created", result.ChunksCreated,
			"cache_hits", result.CacheHits,
			"chunks_embedded", result.ChunksEmbedded,
			"duration", result.Duration.Round(time.Millisecond))
	case "full":
		logger.Info("full index complete",
			"files_processed", result.FilesProcessed,
			"chunks_created", result.ChunksCreated,
			"cache_hits", result.CacheHits,
			"chunks_embedded", result.ChunksEmbedded,
			"duration", result.Duration.Round(time.Millisecond))
	}

	updateRegistry(logger, absPath, idx, cfg, verbose)
	return ExitOK
}

func updateRegistry(logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}, absPath string, idx *indexer.Indexer, cfg *indexer.Config, verbose bool) {
	reg, err := registry.NewRegistry()
	if err != nil {
		logger.Warn("failed to load registry, skipping stats update", "error", err)
		return
	}

	if err := reg.Add(absPath); err != nil {
		logger.Warn("failed to register project", "error", err)
		return
	}

	indexStats, err := idx.Stats()
	if err != nil {
		logger.Warn("failed to get index stats", "error", err)
		return
	}

	dbSize := int64(0)
	if cfg.DBType == "sqlite" && cfg.DBPath != "" {
		if info, err := os.Stat(cfg.DBPath); err == nil {
			dbSize = info.Size()
		}
	}

	registryStats := registry.IndexStats{
		Symbols:     0,
		Embeddings:  indexStats.CachedEmbeddings,
		DBSizeBytes: dbSize,
	}

	if err := reg.UpdateStats(absPath, registryStats); err != nil {
		logger.Warn("failed to update registry stats", "error", err)
		return
	}

	if err := reg.SetLastIndexed(absPath); err != nil {
		logger.Warn("failed to update last indexed timestamp", "error", err)
		return
	}

	if verbose {
		logger.Info("updated registry",
			"embeddings", registryStats.Embeddings,
			"db_size_bytes", registryStats.DBSizeBytes)
	}
}
