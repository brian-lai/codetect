// Package datadir resolves the centralized data directory for a repository.
//
// Instead of storing index data in a local .codetect/ directory at each project
// root, data is stored centrally under ~/.codetect/projects/<basename>-<hash>/.
// This keeps project directories clean and avoids .gitignore modifications.
//
// Key derivation:
//   - Git repos: SHA-256 of normalized remote origin URL (first 8 hex chars)
//   - Non-git dirs: SHA-256 of absolute path (first 8 hex chars)
//   - Directory name: <basename>-<shorthash>
//
// An index.json file at ~/.codetect/projects/index.json maintains a reverse
// lookup from directory name to repository absolute path.
package datadir

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	// indexMu protects concurrent access to index.json
	indexMu sync.Mutex
)

// ForRepo returns the centralized data directory for the given repository path.
// It auto-migrates a local .codetect/ directory if one exists and the centralized
// directory does not.
func ForRepo(repoPath string) (string, error) {
	// Check for env var override
	if override := os.Getenv("CODETECT_DATA_DIR"); override != "" {
		if err := os.MkdirAll(override, 0755); err != nil {
			return "", fmt.Errorf("creating override data dir: %w", err)
		}
		return override, nil
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path: %w", err)
	}

	dirName, err := ComputeDirName(absPath)
	if err != nil {
		return "", fmt.Errorf("computing directory name: %w", err)
	}

	centralDir, err := centralProjectsDir()
	if err != nil {
		return "", err
	}

	dataDir := filepath.Join(centralDir, dirName)

	// Resolution order:
	// 1. Centralized path exists → return it
	// 2. Local .codetect/ exists, centralized does not → migrate
	// 3. Neither exists → create centralized

	centralExists := dirExists(dataDir)
	localDir := filepath.Join(absPath, ".codetect")
	localExists := dirExists(localDir)

	switch {
	case centralExists:
		// Already centralized, nothing to do
	case localExists && !isSymlink(localDir):
		// Migrate local to centralized
		if err := migrateLocal(localDir, dataDir); err != nil {
			return "", fmt.Errorf("migrating local data: %w", err)
		}
	default:
		// Create fresh centralized directory (also covers symlink case)
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return "", fmt.Errorf("creating data directory: %w", err)
		}
	}

	// Update index.json
	if err := updateIndex(centralDir, dirName, absPath); err != nil {
		// Non-fatal: log but don't fail
		fmt.Fprintf(os.Stderr, "warning: could not update index.json: %v\n", err)
	}

	return dataDir, nil
}

// ForRepoNoMigrate returns the centralized data directory for the given
// repository path without performing migration or creating directories.
// Returns the path even if it doesn't exist yet. This is purely a path
// computation — no filesystem side effects.
func ForRepoNoMigrate(repoPath string) (string, error) {
	// Check for env var override
	if override := os.Getenv("CODETECT_DATA_DIR"); override != "" {
		return override, nil
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path: %w", err)
	}

	dirName, err := ComputeDirName(absPath)
	if err != nil {
		return "", fmt.Errorf("computing directory name: %w", err)
	}

	centralDir, err := centralProjectsDirPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(centralDir, dirName), nil
}

// ComputeDirName returns the centralized directory name for a repository path
// in the form <basename>-<shorthash>, without any side effects.
func ComputeDirName(repoPath string) (string, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path: %w", err)
	}

	shortHash, err := computeShortHash(absPath)
	if err != nil {
		return "", err
	}

	baseName := filepath.Base(absPath)
	return baseName + "-" + shortHash, nil
}

// computeShortHash derives the 8-char hex hash for a repository.
// Prefers git remote URL; falls back to absolute path.
func computeShortHash(absPath string) (string, error) {
	// Try git remote origin URL
	remoteURL, err := gitRemoteURL(absPath)
	if err == nil && remoteURL != "" {
		normalized := normalizeGitURL(remoteURL)
		return hashFirst8(normalized), nil
	}

	// Fallback: hash of absolute path
	return hashFirst8(absPath), nil
}

// gitRemoteURL runs `git remote get-url origin` in the given directory.
func gitRemoteURL(dir string) (string, error) {
	ctx, cancel := timeoutContext(2 * time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// normalizeGitURL normalizes a git remote URL for consistent hashing.
// Strips trailing .git and lowercases the URL.
func normalizeGitURL(url string) string {
	url = strings.ToLower(url)
	url = strings.TrimSuffix(url, ".git")
	return url
}

// hashFirst8 returns the first 8 hex characters of the SHA-256 hash of s.
func hashFirst8(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:4])
}

// timeoutContext returns a context with the given timeout.
func timeoutContext(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// centralProjectsDir returns the path to ~/.codetect/projects/, creating it if needed.
func centralProjectsDir() (string, error) {
	dir, err := centralProjectsDirPath()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating projects directory: %w", err)
	}

	return dir, nil
}

// centralProjectsDirPath returns the path to ~/.codetect/projects/ without creating it.
func centralProjectsDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	return filepath.Join(home, ".codetect", "projects"), nil
}

// dirExists reports whether the given path is an existing directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isSymlink reports whether the given path is a symbolic link.
func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

// migrateLocal moves a local .codetect/ directory to the centralized location.
// Uses os.Rename first; falls back to recursive copy if rename fails (cross-device).
func migrateLocal(localDir, centralDir string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(centralDir), 0755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	// Try rename (same filesystem)
	if err := os.Rename(localDir, centralDir); err == nil {
		return nil
	}

	// Fallback: recursive copy then remove
	if err := copyDir(localDir, centralDir); err != nil {
		return fmt.Errorf("copying local data: %w", err)
	}
	if err := os.RemoveAll(localDir); err != nil {
		// Non-fatal: data is already copied
		fmt.Fprintf(os.Stderr, "warning: could not remove old .codetect/ directory: %v\n", err)
	}
	return nil
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, srcInfo.Mode())
}

// indexEntry represents a mapping in the projects index.
type indexEntry struct {
	RepoPath string `json:"repo_path"`
	DirName  string `json:"dir_name"`
}

// projectIndex is the structure stored in index.json.
type projectIndex struct {
	Entries []indexEntry `json:"entries"`
}

// updateIndex updates the ~/.codetect/projects/index.json mapping.
func updateIndex(centralDir, dirName, repoPath string) error {
	indexMu.Lock()
	defer indexMu.Unlock()

	indexPath := filepath.Join(centralDir, "index.json")

	var idx projectIndex

	// Load existing index
	data, err := os.ReadFile(indexPath)
	if err == nil {
		_ = json.Unmarshal(data, &idx)
	}

	// Update or add entry
	found := false
	for i, e := range idx.Entries {
		if e.DirName == dirName {
			idx.Entries[i].RepoPath = repoPath
			found = true
			break
		}
	}
	if !found {
		idx.Entries = append(idx.Entries, indexEntry{
			RepoPath: repoPath,
			DirName:  dirName,
		})
	}

	// Write back
	out, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, out, 0644)
}

