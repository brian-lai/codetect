package commands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"codetect/internal/config"
	"codetect/internal/datadir"
	"codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/logging"
	"codetect/internal/search/symbols"
)

// RunEmbed implements the `codetect embed` subcommand.
// Moved verbatim from cmd/codetect-index/main.go:runEmbed.
func RunEmbed(args []string) ExitCode {
	logger := logging.Default("codetect")
	fs := flag.NewFlagSet("embed", flag.ExitOnError)
	force := fs.Bool("force", false, "Re-embed all chunks (ignore cache)")
	fs.BoolVar(force, "f", false, "Short for --force")
	provider := fs.String("provider", "", "Embedding provider (ollama, litellm, off)")
	model := fs.String("model", "", "Embedding model (provider-specific default if empty)")
	parallel := fs.Int("parallel", 10, "Number of parallel embedding workers")
	fs.IntVar(parallel, "j", 10, "Short for --parallel (like make -j)")
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

	cfg := embedding.LoadConfigFromEnv()
	if *provider != "" {
		switch *provider {
		case "ollama":
			cfg.Provider = embedding.ProviderOllama
		case "litellm":
			cfg.Provider = embedding.ProviderLiteLLM
		case "off":
			cfg.Provider = embedding.ProviderOff
		default:
			logger.Error("unknown provider", "provider", *provider)
			return ExitError
		}
	}
	if *model != "" {
		cfg.Model = *model
	}

	if cfg.Provider == embedding.ProviderOff {
		logger.Info("embedding disabled", "provider", "off")
		return ExitOK
	}

	embedder, err := embedding.NewEmbedder(cfg)
	if err != nil {
		logger.Error("creating embedder failed", "error", err)
		return ExitError
	}

	if !embedder.Available() {
		logger.Error("provider not available", "provider", cfg.Provider)
		if cfg.Provider == embedding.ProviderOllama {
			logger.Info("install Ollama from https://ollama.ai, then run: ollama pull nomic-embed-text")
		} else if cfg.Provider == embedding.ProviderLiteLLM {
			logger.Info("check CODETECT_LITELLM_URL and CODETECT_LITELLM_API_KEY")
		}
		return ExitError
	}

	logger.Info("using embedding provider", "provider", embedder.ProviderID())

	dbConfig := config.LoadDatabaseConfigFromEnv()
	if dbConfig.Type == db.DatabaseSQLite {
		dd, err := datadir.ForRepoNoMigrate(absPath)
		if err != nil {
			logger.Error("resolving data directory failed", "error", err)
			return ExitError
		}
		dbPath := filepath.Join(dd, "symbols.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			logger.Error("no symbol index found, run 'codetect index' first")
			return ExitError
		}
		dbConfig.Path = dbPath
	}

	dbCfg := dbConfig.ToDBConfig()
	logger.Debug("database config", "database", dbConfig.String())

	idx, err := symbols.NewIndexWithConfig(dbCfg, absPath)
	if err != nil {
		logger.Error("opening index failed", "error", err)
		return ExitError
	}
	defer idx.Close()

	store, err := embedding.NewEmbeddingStoreWithOptions(
		idx.DBAdapter(),
		idx.Dialect(),
		dbConfig.VectorDimensions,
		absPath,
	)
	if err != nil {
		logger.Error("creating embedding store failed", "error", err)
		return ExitError
	}
	searcher := embedding.NewSemanticSearcher(store, embedder)

	oldDim, hasMismatch, err := store.CheckDimensionMismatch(absPath, dbConfig.VectorDimensions)
	if err != nil {
		logger.Warn("checking dimension mismatch", "error", err)
	}
	if hasMismatch {
		logger.Info("dimension change detected",
			"old_dimensions", oldDim,
			"new_dimensions", dbConfig.VectorDimensions,
			"model", cfg.Model)
		if err := store.MigrateRepoDimensions(absPath, oldDim, dbConfig.VectorDimensions, cfg.Model); err != nil {
			logger.Error("migrating embeddings failed", "error", err)
			return ExitError
		}
		logger.Info("migrated to new dimension group, re-embedding required")
	}

	if *force {
		logger.Info("clearing existing embeddings")
		if err := searcher.Store().DeleteAll(); err != nil {
			logger.Error("clearing embeddings failed", "error", err)
			return ExitError
		}
	}

	gi := loadGitignore(absPath)
	logger.Info("scanning files to embed")
	var filesToEmbed []string
	var totalSize int64

	err = filepath.Walk(absPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(absPath, filePath)
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".codetect" {
				return filepath.SkipDir
			}
			if gi != nil && gi.MatchesPath(relPath+"/") {
				return filepath.SkipDir
			}
			return nil
		}
		if gi != nil && gi.MatchesPath(relPath) {
			return nil
		}
		if isCodeFile(filePath) {
			filesToEmbed = append(filesToEmbed, filePath)
			totalSize += info.Size()
		}
		return nil
	})
	if err != nil {
		logger.Error("scanning directory failed", "error", err)
		return ExitError
	}

	if len(filesToEmbed) == 0 {
		logger.Info("no code files to embed")
		return ExitOK
	}

	fmt.Fprintf(os.Stderr, "\n📊 Embedding Preview:\n")
	fmt.Fprintf(os.Stderr, "   Files to embed: %d\n", len(filesToEmbed))
	fmt.Fprintf(os.Stderr, "   Total size: %s\n", formatBytes(totalSize))
	fmt.Fprintf(os.Stderr, "   Provider: %s\n", cfg.Provider)
	if cfg.Model != "" {
		fmt.Fprintf(os.Stderr, "   Model: %s\n", cfg.Model)
	}
	fmt.Fprintf(os.Stderr, "\n")

	logger.Info("collecting code chunks")
	var allChunks []embedding.Chunk
	chunkerConfig := embedding.DefaultChunkerConfig()

	for _, filePath := range filesToEmbed {
		relPath, _ := filepath.Rel(absPath, filePath)
		syms, _ := idx.ListDefsInFile(relPath)
		chunks, err := embedding.ChunkFile(filePath, syms, chunkerConfig)
		if err != nil {
			continue
		}
		for i := range chunks {
			chunks[i].Path = relPath
		}
		allChunks = append(allChunks, chunks...)
	}

	logger.Info("found chunks to embed", "chunks", len(allChunks))
	if len(allChunks) == 0 {
		logger.Info("no chunks to embed")
		return ExitOK
	}

	start := time.Now()
	ctx := context.Background()
	progressFn := func(current, total int) {
		fmt.Fprintf(os.Stderr, "\rembedding chunk %d/%d...", current, total)
	}

	if err := searcher.IndexChunksParallel(ctx, allChunks, *parallel, progressFn); err != nil {
		fmt.Fprintln(os.Stderr)
		logger.Error("embedding failed", "error", err)
		return ExitError
	}

	count, fileCount, err := searcher.Store().Stats()
	fmt.Fprintln(os.Stderr)
	if err != nil {
		logger.Warn("could not get stats", "error", err)
	} else {
		elapsed := time.Since(start)
		logger.Info("embedding complete",
			"chunks", count,
			"files", fileCount,
			"duration", elapsed.Round(time.Millisecond))
	}

	if err := store.SetRepoConfig(absPath, cfg.Model, dbConfig.VectorDimensions); err != nil {
		logger.Warn("could not update repo config", "error", err)
	}
	return ExitOK
}
