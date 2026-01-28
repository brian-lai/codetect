package merkle

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefaultIgnorePatterns contains common directories and files to skip.
var DefaultIgnorePatterns = []string{
	// Version control
	".git",
	".svn",
	".hg",

	// Dependencies
	"node_modules",
	"vendor",
	"bower_components",

	// Build outputs
	"dist",
	"build",
	"out",
	"target",
	"bin",

	// Cache directories
	"__pycache__",
	".pytest_cache",
	".mypy_cache",
	".tox",
	".cache",

	// IDE directories
	".idea",
	".vscode",
	".vs",

	// OS files
	".DS_Store",
	"Thumbs.db",
}

// Builder constructs Merkle trees from filesystems.
// It walks the directory tree, computes hashes for each file,
// and builds a hierarchical structure that can be compared
// against future builds to detect changes.
type Builder struct {
	// IgnorePatterns contains additional patterns to ignore.
	// These are matched against file/directory names (not paths).
	IgnorePatterns []string

	// IncludeHidden controls whether hidden files (starting with .)
	// are included in the tree. Default is false.
	IncludeHidden bool

	// IncludeDotfiles is a more specific list of hidden files to include
	// even when IncludeHidden is false (e.g., ".gitignore", ".env.example").
	IncludeDotfiles []string
}

// NewBuilder creates a Builder with default settings.
func NewBuilder() *Builder {
	return &Builder{
		IgnorePatterns: DefaultIgnorePatterns,
		IncludeHidden:  false,
		IncludeDotfiles: []string{
			".gitignore",
			".dockerignore",
			".editorconfig",
			".prettierrc",
			".eslintrc",
			".eslintrc.json",
			".eslintrc.js",
		},
	}
}

// Build creates a Merkle tree from the given directory.
// It recursively walks the filesystem, computing hashes for each file
// and rolling up directory hashes from their children.
func (b *Builder) Build(repoPath string) (*Tree, error) {
	// Clean and resolve the path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}

	root, fileCount, err := b.buildNode(absPath, "")
	if err != nil {
		return nil, err
	}

	return &Tree{
		Root:      root,
		RepoPath:  absPath,
		BuildTime: time.Now(),
		FileCount: fileCount,
	}, nil
}

// buildNode recursively builds a node for the given path.
// basePath is the absolute path to the repository root.
// relPath is the relative path from the root to this node.
// Returns the node, file count, and any error.
func (b *Builder) buildNode(basePath, relPath string) (*Node, int, error) {
	fullPath := filepath.Join(basePath, relPath)

	info, err := os.Lstat(fullPath)
	if err != nil {
		return nil, 0, err
	}

	// Skip symlinks to avoid cycles and security issues
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, 0, nil
	}

	node := &Node{
		Path:    relPath,
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}

	fileCount := 0

	if info.IsDir() {
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return nil, 0, err
		}

		for _, entry := range entries {
			name := entry.Name()

			if b.shouldIgnore(name) {
				continue
			}

			childPath := filepath.Join(relPath, name)
			child, count, err := b.buildNode(basePath, childPath)
			if err != nil {
				// Skip unreadable files/directories
				continue
			}

			// Skip nil nodes (e.g., symlinks)
			if child == nil {
				continue
			}

			// Skip empty directories
			if child.IsDir && len(child.Children) == 0 {
				continue
			}

			node.Children = append(node.Children, child)
			fileCount += count
		}

		// Sort children by path for deterministic hashing
		sort.Slice(node.Children, func(i, j int) bool {
			return node.Children[i].Path < node.Children[j].Path
		})

		// Compute directory hash from children
		node.ComputeHash(nil)
	} else {
		// Read file content and compute hash
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, 0, err
		}
		node.ComputeHash(content)
		fileCount = 1
	}

	return node, fileCount, nil
}

// shouldIgnore returns true if the given name should be skipped.
func (b *Builder) shouldIgnore(name string) bool {
	// Check default ignore patterns
	for _, pattern := range b.IgnorePatterns {
		if name == pattern {
			return true
		}
	}

	// Handle hidden files
	if len(name) > 0 && name[0] == '.' {
		if b.IncludeHidden {
			return false
		}

		// Check if it's in the allowed dotfiles list
		for _, allowed := range b.IncludeDotfiles {
			if name == allowed {
				return false
			}
		}

		return true
	}

	return false
}

// WithIgnorePatterns adds additional patterns to ignore.
func (b *Builder) WithIgnorePatterns(patterns ...string) *Builder {
	b.IgnorePatterns = append(b.IgnorePatterns, patterns...)
	return b
}

// WithIncludeHidden enables including all hidden files.
func (b *Builder) WithIncludeHidden(include bool) *Builder {
	b.IncludeHidden = include
	return b
}

// ParseGitignore reads a .gitignore file and adds patterns to the builder.
// This is a simplified parser that handles basic patterns.
func (b *Builder) ParseGitignore(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle negation (we just skip these for simplicity)
		if strings.HasPrefix(line, "!") {
			continue
		}

		// Remove trailing slashes (we treat dirs and files the same)
		line = strings.TrimSuffix(line, "/")

		// For simple patterns (no wildcards), add directly
		// This is a simplified implementation
		if !strings.ContainsAny(line, "*?[") {
			b.IgnorePatterns = append(b.IgnorePatterns, line)
		}
	}

	return nil
}
