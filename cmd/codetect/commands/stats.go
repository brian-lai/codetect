package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"codetect/internal/config"
	"codetect/internal/datadir"
	"codetect/internal/db"
	"codetect/internal/embedding"
	"codetect/internal/indexer"
	"codetect/internal/logging"
	"codetect/internal/search/symbols"
)

// RunStats implements the `codetect stats` subcommand.
// Moved verbatim from cmd/codetect-index/main.go:runStats.
func RunStats(args []string) ExitCode {
	logger := logging.Default("codetect")
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	useV1 := fs.Bool("v1", false, "Show v1 index stats (deprecated)")
	jsonOutput := fs.Bool("json", false, "Output stats as JSON")
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
		return runStatsV2(logger, absPath, *jsonOutput)
	}

	logger.Warn("⚠️  Showing v1 index stats (deprecated)")

	dbConfig := config.LoadDatabaseConfigFromEnv()
	if dbConfig.Type == db.DatabaseSQLite {
		dd, err := datadir.ForRepoNoMigrate(absPath)
		if err != nil {
			logger.Error("resolving data directory failed", "error", err)
			return ExitError
		}
		dbPath := filepath.Join(dd, "symbols.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			logger.Error("no index found, run 'index' first")
			return ExitError
		}
		dbConfig.Path = dbPath
	}

	dbCfg := dbConfig.ToDBConfig()
	idx, err := symbols.NewIndexWithConfig(dbCfg, absPath)
	if err != nil {
		logger.Error("opening index failed", "error", err)
		return ExitError
	}
	defer idx.Close()

	symbolCount, fileCount, err := idx.Stats()
	if err != nil {
		logger.Error("getting stats failed", "error", err)
		return ExitError
	}

	fmt.Printf("Database: %s\n", dbConfig.String())
	fmt.Printf("Symbols: %d\n", symbolCount)
	fmt.Printf("Files: %d\n", fileCount)

	store, err := embedding.NewEmbeddingStoreWithOptions(
		idx.DBAdapter(),
		idx.Dialect(),
		dbConfig.VectorDimensions,
		absPath,
	)
	if err == nil {
		embCount, embFileCount, err := store.Stats()
		if err == nil && embCount > 0 {
			fmt.Printf("Embeddings: %d chunks from %d files\n", embCount, embFileCount)
		}
	}
	return ExitOK
}

func runStatsV2(logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}, absPath string, jsonOutput bool) ExitCode {
	dbConfig := config.LoadDatabaseConfigFromEnv()
	embConfig := embedding.LoadConfigFromEnv()

	cfg := &indexer.Config{
		DBType:            string(dbConfig.Type),
		Dimensions:        dbConfig.VectorDimensions,
		EmbeddingProvider: "off",
		EmbeddingModel:    embConfig.Model,
	}

	if dbConfig.Type == db.DatabasePostgres {
		cfg.DSN = dbConfig.DSN
	} else {
		dd, err := datadir.ForRepoNoMigrate(absPath)
		if err != nil {
			logger.Error("resolving data directory failed", "error", err)
			return ExitError
		}
		cfg.DBPath = filepath.Join(dd, "index.db")
	}

	idx, err := indexer.New(absPath, cfg)
	if err != nil {
		logger.Error("opening v2 indexer failed", "error", err)
		return ExitError
	}
	defer idx.Close()

	stats, err := idx.Stats()
	if err != nil {
		logger.Error("getting v2 stats failed", "error", err)
		return ExitError
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(stats); err != nil {
			logger.Error("encoding JSON failed", "error", err)
			return ExitError
		}
		return ExitOK
	}

	fmt.Printf("v2 Index Statistics\n")
	fmt.Printf("==================\n")
	fmt.Printf("Total Chunks:      %d\n", stats.TotalChunks)
	fmt.Printf("Unique Hashes:     %d\n", stats.UniqueHashes)
	fmt.Printf("Files:             %d\n", stats.FileCount)
	fmt.Printf("Cached Embeddings: %d\n", stats.CachedEmbeddings)

	if stats.IndexedVectors > 0 {
		indexType := "brute-force"
		if stats.VectorIndexNative {
			indexType = "native HNSW"
		}
		fmt.Printf("Indexed Vectors:   %d (%s)\n", stats.IndexedVectors, indexType)
	}

	if len(stats.ByNodeType) > 0 {
		fmt.Printf("\nBy Node Type:\n")
		for nodeType, count := range stats.ByNodeType {
			fmt.Printf("  %-20s %d\n", nodeType+":", count)
		}
	}

	if len(stats.ByLanguage) > 0 {
		fmt.Printf("\nBy Language:\n")
		for lang, count := range stats.ByLanguage {
			fmt.Printf("  %-20s %d\n", lang+":", count)
		}
	}
	return ExitOK
}
