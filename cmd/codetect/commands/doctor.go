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
)

// RunDoctor implements the `codetect doctor` subcommand.
// Moved verbatim from cmd/codetect-index/main.go:runDoctor.
// Phase 3 will extend this with sentinel-file reading and Ollama GET check.
func RunDoctor(args []string) ExitCode {
	logger := logging.Default("codetect")
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "Output results as JSON")
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

	type DoctorResult struct {
		OllamaAvailable bool     `json:"ollama_available"`
		ModelAvailable  bool     `json:"model_available,omitempty"`
		Model           string   `json:"model,omitempty"`
		IndexExists     bool     `json:"index_exists"`
		FailedChunks    int      `json:"failed_chunks"`
		AffectedFiles   int      `json:"affected_files"`
		Issues          []string `json:"issues,omitempty"`
	}

	result := DoctorResult{}
	var issues []string

	embConfig := embedding.LoadConfigFromEnv()

	if embConfig.Provider == embedding.ProviderOllama {
		result.Model = embConfig.Model
		client := embedding.NewOllamaClient(
			embedding.WithBaseURL(embConfig.OllamaURL),
			embedding.WithModel(embConfig.Model),
		)
		result.OllamaAvailable = client.Available()
		if !result.OllamaAvailable {
			issues = append(issues, "Ollama is not running. Install from https://ollama.ai and start it.")
		} else {
			result.ModelAvailable = client.ModelAvailable()
			if !result.ModelAvailable {
				issues = append(issues, fmt.Sprintf("Model %q is not available. Run: ollama pull %s", embConfig.Model, embConfig.Model))
			}
		}
	}

	dbConfig := config.LoadDatabaseConfigFromEnv()
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
			if !*jsonOutput {
				fmt.Println("No index found for this directory.")
			}
			result.Issues = issues
			if *jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				enc.Encode(result)
			}
			return ExitError
		}
		cfg.DBPath = filepath.Join(dd, "index.db")
	}

	idx, err := indexer.New(absPath, cfg)
	if err != nil {
		if !*jsonOutput {
			fmt.Println("No index found. Run 'codetect index' first.")
		}
		result.Issues = issues
		if *jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(result)
		}
		return ExitError
	}
	defer idx.Close()
	result.IndexExists = true

	if failStore := idx.FailureStore(); failStore != nil {
		summary, err := failStore.GetFailureSummary(absPath)
		if err == nil && summary != nil {
			result.FailedChunks = summary.TotalFailures
			result.AffectedFiles = len(summary.AffectedFiles)

			if summary.TotalFailures > 0 {
				issues = append(issues,
					fmt.Sprintf("%d chunks failed to embed in %d files",
						summary.TotalFailures, len(summary.AffectedFiles)))

				if !*jsonOutput {
					fmt.Printf("\nAffected files:\n")
					for _, f := range summary.AffectedFiles {
						fmt.Printf("  - %s\n", f)
					}
				}
			}
		}
	}

	result.Issues = issues

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		return ExitOK
	}

	fmt.Println("codetect doctor")
	fmt.Println("===============")

	if embConfig.Provider == embedding.ProviderOllama {
		if result.OllamaAvailable {
			fmt.Println("Ollama:     OK")
		} else {
			fmt.Println("Ollama:     NOT RUNNING")
		}
		if result.ModelAvailable {
			fmt.Printf("Model:      %s (available)\n", result.Model)
		} else if result.OllamaAvailable {
			fmt.Printf("Model:      %s (NOT FOUND)\n", result.Model)
		}
	} else {
		fmt.Printf("Provider:   %s\n", embConfig.Provider)
	}

	if result.IndexExists {
		fmt.Println("Index:      OK")
	} else {
		fmt.Println("Index:      NOT FOUND")
	}

	if result.FailedChunks > 0 {
		fmt.Printf("Failures:   %d chunks in %d files\n", result.FailedChunks, result.AffectedFiles)
		fmt.Println("\nRemediation:")
		fmt.Println("  Re-index with: codetect index --force")
	} else if result.IndexExists {
		fmt.Println("Failures:   none")
	}

	if len(issues) > 0 {
		fmt.Println("\nIssues:")
		for _, issue := range issues {
			fmt.Printf("  - %s\n", issue)
		}
		return ExitError
	}

	fmt.Println("\nAll checks passed.")
	return ExitOK
}
