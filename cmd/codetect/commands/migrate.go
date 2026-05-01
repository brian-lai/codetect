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
)

// RunMigrate implements the `codetect migrate-to-postgres` subcommand.
// Moved verbatim from cmd/migrate-to-postgres/main.go.
func RunMigrate(args []string) ExitCode {
	logger := logging.Default("codetect")

	// Default source path: try centralized data dir for current working dir
	defaultSQLitePath := ".codetect/symbols.db" // legacy fallback
	if dd, err := datadir.ForRepoNoMigrate("."); err == nil {
		defaultSQLitePath = filepath.Join(dd, "symbols.db")
	}

	fs := flag.NewFlagSet("migrate-to-postgres", flag.ExitOnError)
	sqlitePath := fs.String("source", defaultSQLitePath, "SQLite database path")
	batchSize := fs.Int("batch", 1000, "Number of embeddings to migrate per batch")
	skipExisting := fs.Bool("skip-existing", true, "Skip embeddings that already exist in PostgreSQL")
	dropTarget := fs.Bool("drop-target", false, "Drop existing PostgreSQL tables before migration")
	dryRun := fs.Bool("dry-run", false, "Perform validation without migrating data")
	validate := fs.Bool("validate", true, "Validate migration after completion")
	sampleSize := fs.Int("sample-size", 100, "Number of embeddings to sample for validation")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: codetect migrate-to-postgres [options]\n\n")
		fmt.Fprintf(os.Stderr, "Migrate embeddings from SQLite to PostgreSQL.\n\n")
		fmt.Fprintf(os.Stderr, "Environment variables:\n")
		fmt.Fprintf(os.Stderr, "  CODETECT_DB_TYPE=postgres\n")
		fmt.Fprintf(os.Stderr, "  CODETECT_DB_DSN=postgres://user:pass@localhost:5432/dbname\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	pgConfig := config.LoadDatabaseConfigFromEnv()
	if pgConfig.Type != db.DatabasePostgres {
		logger.Error("PostgreSQL not configured",
			"hint", "set CODETECT_DB_TYPE=postgres and CODETECT_DB_DSN")
		return ExitError
	}
	if pgConfig.DSN == "" {
		logger.Error("CODETECT_DB_DSN not set")
		return ExitError
	}

	if _, err := os.Stat(*sqlitePath); os.IsNotExist(err) {
		logger.Error("SQLite database not found",
			"path", *sqlitePath,
			"hint", "run 'codetect embed' first")
		return ExitError
	}

	fmt.Println("PostgreSQL Migration Tool")
	fmt.Println("==========================")
	fmt.Println()
	fmt.Printf("Source:      SQLite (%s)\n", *sqlitePath)
	fmt.Printf("Target:      %s\n", pgConfig.String())
	fmt.Printf("Batch size:  %d\n", *batchSize)
	fmt.Printf("Skip exists: %v\n", *skipExisting)
	fmt.Printf("Drop target: %v\n", *dropTarget)
	fmt.Printf("Dry run:     %v\n", *dryRun)
	fmt.Println()

	if *dropTarget && !*dryRun {
		fmt.Print("WARNING: This will delete all existing data in PostgreSQL!\n")
		fmt.Print("Type 'yes' to continue: ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Migration cancelled")
			return ExitOK
		}
		fmt.Println()
	}

	repoRoot, _ := os.Getwd()
	repoRoot, _ = filepath.Abs(repoRoot)

	sqliteCfg := db.DefaultConfig(*sqlitePath)
	sqliteCfg.VectorDimensions = pgConfig.VectorDimensions
	sourceDB, err := db.Open(sqliteCfg)
	if err != nil {
		logger.Error("error opening SQLite", "error", err)
		return ExitError
	}
	defer sourceDB.Close()

	sourceStore, err := embedding.NewEmbeddingStore(sourceDB, repoRoot)
	if err != nil {
		logger.Error("error creating source embedding store", "error", err)
		return ExitError
	}

	targetCfg := pgConfig.ToDBConfig()
	targetDB, err := db.Open(targetCfg)
	if err != nil {
		logger.Error("error opening PostgreSQL",
			"error", err,
			"hint", "is PostgreSQL running? Check: docker-compose ps")
		return ExitError
	}
	defer targetDB.Close()

	dialect := targetCfg.Dialect()
	targetStore, err := embedding.NewEmbeddingStoreWithDialect(targetDB, dialect, repoRoot)
	if err != nil {
		logger.Error("error creating target embedding store", "error", err)
		return ExitError
	}

	sourceCount, err := sourceStore.Count()
	if err != nil {
		logger.Error("error counting source embeddings", "error", err)
		return ExitError
	}

	if sourceCount == 0 {
		fmt.Println("No embeddings found in SQLite database")
		fmt.Println("Run 'codetect embed' to generate embeddings first")
		return ExitOK
	}

	fmt.Printf("Found %d embeddings in SQLite\n", sourceCount)
	fmt.Println()

	if *dryRun {
		fmt.Println("Dry run mode - no data will be migrated")
		return ExitOK
	}

	opts := embedding.MigrationOptions{
		BatchSize:    *batchSize,
		SkipExisting: *skipExisting,
		DropTarget:   *dropTarget,
		DryRun:       *dryRun,
	}

	startTime := time.Now()
	lastUpdate := time.Now()
	progressFn := func(progress embedding.MigrationProgress) {
		now := time.Now()
		if now.Sub(lastUpdate) < 100*time.Millisecond && progress.MigratedEmbeddings < progress.TotalEmbeddings {
			return
		}
		lastUpdate = now
		percent := float64(progress.MigratedEmbeddings+progress.SkippedEmbeddings) / float64(progress.TotalEmbeddings) * 100
		elapsed := now.Sub(startTime)
		rate := float64(progress.MigratedEmbeddings) / elapsed.Seconds()
		fmt.Printf("\rProgress: %d/%d (%.1f%%) | Migrated: %d | Skipped: %d | Rate: %.0f/s | %s",
			progress.MigratedEmbeddings+progress.SkippedEmbeddings,
			progress.TotalEmbeddings,
			percent,
			progress.MigratedEmbeddings,
			progress.SkippedEmbeddings,
			rate,
			progress.CurrentFile,
		)
		if progress.MigratedEmbeddings+progress.SkippedEmbeddings >= progress.TotalEmbeddings {
			fmt.Println()
		}
	}

	fmt.Println("Starting migration...")
	ctx := context.Background()
	if err := embedding.MigrateDatabaseWithVectorIndex(ctx, sourceStore, targetStore, opts, progressFn); err != nil {
		fmt.Println()
		logger.Error("error during migration", "error", err)
		return ExitError
	}

	duration := time.Since(startTime)
	fmt.Printf("\nMigration completed in %s\n", duration.Round(time.Millisecond))

	if *validate {
		fmt.Println()
		fmt.Println("Validating migration...")
		if err := embedding.ValidateMigration(sourceStore, targetStore, *sampleSize); err != nil {
			logger.Error("validation failed", "error", err)
			return ExitError
		}
		fmt.Println("Validation passed")
	}

	targetCount, err := targetStore.Count()
	if err != nil {
		logger.Error("error counting target embeddings", "error", err)
		return ExitError
	}

	fmt.Println()
	fmt.Println("Migration Summary")
	fmt.Println("=================")
	fmt.Printf("Source (SQLite):     %d embeddings\n", sourceCount)
	fmt.Printf("Target (PostgreSQL): %d embeddings\n", targetCount)
	fmt.Printf("Duration:            %s\n", duration.Round(time.Millisecond))
	fmt.Printf("Rate:                %.0f embeddings/sec\n", float64(sourceCount)/duration.Seconds())
	fmt.Println()
	fmt.Println("✓ Migration successful!")
	return ExitOK
}
