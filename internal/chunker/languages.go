package chunker

import (
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// LanguageConfig defines the chunking strategy for a specific language.
// It includes the tree-sitter language grammar and configuration for
// which AST nodes should be used as chunk boundaries.
type LanguageConfig struct {
	Language     *sitter.Language // Tree-sitter language grammar
	Name         string           // Language identifier (e.g., "go", "python")
	SplitNodes   []string         // AST node types to create chunks from
	NameFields   []string         // Field names that contain symbol names
	MaxChunkSize int              // Max characters per chunk before recursive splitting
}

// languageConfigs maps language names to their configurations.
var languageConfigs = map[string]*LanguageConfig{
	"go": {
		Language:     golang.GetLanguage(),
		Name:         "go",
		SplitNodes:   []string{"function_declaration", "method_declaration", "type_declaration", "const_declaration", "var_declaration"},
		NameFields:   []string{"name"},
		MaxChunkSize: 2000,
	},
	"python": {
		Language:     python.GetLanguage(),
		Name:         "python",
		SplitNodes:   []string{"function_definition", "class_definition", "decorated_definition"},
		NameFields:   []string{"name"},
		MaxChunkSize: 2000,
	},
	"javascript": {
		Language:     javascript.GetLanguage(),
		Name:         "javascript",
		SplitNodes:   []string{"function_declaration", "class_declaration", "method_definition", "arrow_function", "export_statement"},
		NameFields:   []string{"name"},
		MaxChunkSize: 2000,
	},
	"typescript": {
		Language:     typescript.GetLanguage(),
		Name:         "typescript",
		SplitNodes:   []string{"function_declaration", "class_declaration", "method_definition", "arrow_function", "interface_declaration", "type_alias_declaration", "export_statement"},
		NameFields:   []string{"name"},
		MaxChunkSize: 2000,
	},
	"tsx": {
		Language:     tsx.GetLanguage(),
		Name:         "tsx",
		SplitNodes:   []string{"function_declaration", "class_declaration", "method_definition", "arrow_function", "interface_declaration", "type_alias_declaration", "export_statement"},
		NameFields:   []string{"name"},
		MaxChunkSize: 2000,
	},
	"rust": {
		Language:     rust.GetLanguage(),
		Name:         "rust",
		SplitNodes:   []string{"function_item", "impl_item", "struct_item", "enum_item", "trait_item", "mod_item"},
		NameFields:   []string{"name"},
		MaxChunkSize: 2000,
	},
	"java": {
		Language:     java.GetLanguage(),
		Name:         "java",
		SplitNodes:   []string{"method_declaration", "class_declaration", "interface_declaration", "constructor_declaration"},
		NameFields:   []string{"name"},
		MaxChunkSize: 2000,
	},
	"c": {
		Language:     c.GetLanguage(),
		Name:         "c",
		SplitNodes:   []string{"function_definition", "struct_specifier", "enum_specifier", "declaration"},
		NameFields:   []string{"declarator"},
		MaxChunkSize: 2000,
	},
	"cpp": {
		Language:     cpp.GetLanguage(),
		Name:         "cpp",
		SplitNodes:   []string{"function_definition", "class_specifier", "struct_specifier", "namespace_definition"},
		NameFields:   []string{"declarator", "name"},
		MaxChunkSize: 2000,
	},
	"ruby": {
		Language:     ruby.GetLanguage(),
		Name:         "ruby",
		SplitNodes:   []string{"method", "class", "module", "singleton_method"},
		NameFields:   []string{"name"},
		MaxChunkSize: 2000,
	},
}

// extToLanguage maps file extensions to language identifiers.
var extToLanguage = map[string]string{
	".go":   "go",
	".py":   "python",
	".js":   "javascript",
	".mjs":  "javascript",
	".jsx":  "javascript",
	".ts":   "typescript",
	".tsx":  "tsx",
	".rs":   "rust",
	".java": "java",
	".c":    "c",
	".h":    "c",
	".cpp":  "cpp",
	".cc":   "cpp",
	".cxx":  "cpp",
	".hpp":  "cpp",
	".hxx":  "cpp",
	".rb":   "ruby",
}

// GetLanguageConfig returns the language configuration for a file path
// based on its extension. Returns nil if the language is not supported.
func GetLanguageConfig(path string) *LanguageConfig {
	ext := strings.ToLower(filepath.Ext(path))
	langName, ok := extToLanguage[ext]
	if !ok {
		return nil
	}
	return languageConfigs[langName]
}

// GetLanguageConfigByName returns the language configuration for a
// language name. Returns nil if the language is not supported.
func GetLanguageConfigByName(name string) *LanguageConfig {
	return languageConfigs[name]
}

// SupportedExtensions returns all supported file extensions.
func SupportedExtensions() []string {
	exts := make([]string, 0, len(extToLanguage))
	for ext := range extToLanguage {
		exts = append(exts, ext)
	}
	return exts
}

// SupportedLanguages returns all supported language names.
func SupportedLanguages() []string {
	langs := make([]string, 0, len(languageConfigs))
	for lang := range languageConfigs {
		langs = append(langs, lang)
	}
	return langs
}

// IsSupported returns true if the file extension is supported.
func IsSupported(path string) bool {
	return GetLanguageConfig(path) != nil
}
