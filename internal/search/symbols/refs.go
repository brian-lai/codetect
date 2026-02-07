package symbols

import (
	"fmt"
)

// SymbolRef represents a reference to a symbol (function call, type usage, etc.)
type SymbolRef struct {
	ID            int    `json:"id"`
	RepoRoot      string `json:"repo_root"`
	Name          string `json:"name"`           // Short name (e.g., "Handle")
	QualifiedName string `json:"qualified_name"` // Best-effort qualified name (e.g., "AuthService.Handle")
	Kind          string `json:"kind"`           // call, type_ref
	SourcePath    string `json:"source_path"`    // File containing the reference
	SourceLine    int    `json:"source_line"`    // Line number of the reference
	SourceScope   string `json:"source_scope"`   // Qualified scope containing this reference
}

// TypeRelation represents a type relationship (implements, extends, embeds)
type TypeRelation struct {
	ID         int    `json:"id"`
	RepoRoot   string `json:"repo_root"`
	ChildType  string `json:"child_type"`  // Implementing/extending type
	ParentType string `json:"parent_type"` // Interface/base class
	Relation   string `json:"relation"`    // implements, extends, embeds
	Path       string `json:"path"`
	Line       int    `json:"line"`
}

// InsertRefs inserts symbol references in bulk.
// Uses batch insert with dialect-aware placeholders.
func (idx *Index) InsertRefs(refs []SymbolRef) error {
	if len(refs) == 0 {
		return nil
	}

	// Prepare batch insert
	tx, err := idx.adapter.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Build insert statement with placeholders
	insertSQL := fmt.Sprintf(`
		INSERT INTO symbol_refs (repo_root, name, qualified_name, kind, source_path, source_line, source_scope)
		VALUES (%s, %s, %s, %s, %s, %s, %s)
		ON CONFLICT (repo_root, source_path, source_line, name) DO UPDATE SET
			qualified_name = excluded.qualified_name,
			kind = excluded.kind,
			source_scope = excluded.source_scope
	`,
		idx.dialect.Placeholder(1),
		idx.dialect.Placeholder(2),
		idx.dialect.Placeholder(3),
		idx.dialect.Placeholder(4),
		idx.dialect.Placeholder(5),
		idx.dialect.Placeholder(6),
		idx.dialect.Placeholder(7),
	)

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return fmt.Errorf("prepare insert statement: %w", err)
	}
	defer stmt.Close()

	for _, ref := range refs {
		qualifiedName := nullString(ref.QualifiedName)
		sourceScope := nullString(ref.SourceScope)

		_, err := stmt.Exec(
			ref.RepoRoot,
			ref.Name,
			qualifiedName,
			ref.Kind,
			ref.SourcePath,
			ref.SourceLine,
			sourceScope,
		)
		if err != nil {
			return fmt.Errorf("inserting ref %s at %s:%d: %w", ref.Name, ref.SourcePath, ref.SourceLine, err)
		}
	}

	return tx.Commit()
}

// DeleteRefsByFile deletes all symbol references for a given file path.
// Used for incremental reindexing when a file changes.
func (idx *Index) DeleteRefsByFile(path string) error {
	deleteSQL := fmt.Sprintf(
		"DELETE FROM symbol_refs WHERE repo_root = %s AND source_path = %s",
		idx.dialect.Placeholder(1),
		idx.dialect.Placeholder(2),
	)
	_, err := idx.adapter.Exec(deleteSQL, idx.root, path)
	return err
}

// QueryRefsByName finds all references to a symbol by name.
// Returns references matching either the short name or qualified name.
func (idx *Index) QueryRefsByName(name string, kind string, limit int) ([]SymbolRef, error) {
	var query string
	var args []interface{}

	// Build query based on whether kind filter is provided
	if kind == "" || kind == "all" {
		query = fmt.Sprintf(`
			SELECT id, repo_root, name, COALESCE(qualified_name, ''), kind, source_path, source_line, COALESCE(source_scope, '')
			FROM symbol_refs
			WHERE repo_root = %s
			  AND (name = %s OR qualified_name LIKE %s)
			ORDER BY source_path, source_line
			LIMIT %s
		`,
			idx.dialect.Placeholder(1),
			idx.dialect.Placeholder(2),
			idx.dialect.Placeholder(3),
			idx.dialect.Placeholder(4),
		)
		args = []interface{}{idx.root, name, "%" + name + "%", limit}
	} else {
		query = fmt.Sprintf(`
			SELECT id, repo_root, name, COALESCE(qualified_name, ''), kind, source_path, source_line, COALESCE(source_scope, '')
			FROM symbol_refs
			WHERE repo_root = %s
			  AND kind = %s
			  AND (name = %s OR qualified_name LIKE %s)
			ORDER BY source_path, source_line
			LIMIT %s
		`,
			idx.dialect.Placeholder(1),
			idx.dialect.Placeholder(2),
			idx.dialect.Placeholder(3),
			idx.dialect.Placeholder(4),
			idx.dialect.Placeholder(5),
		)
		args = []interface{}{idx.root, kind, name, "%" + name + "%", limit}
	}

	rows, err := idx.adapter.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []SymbolRef
	for rows.Next() {
		var ref SymbolRef
		if err := rows.Scan(
			&ref.ID,
			&ref.RepoRoot,
			&ref.Name,
			&ref.QualifiedName,
			&ref.Kind,
			&ref.SourcePath,
			&ref.SourceLine,
			&ref.SourceScope,
		); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}

	return refs, nil
}

// InsertTypeRelations inserts type relationships in bulk.
// Uses batch insert with dialect-aware placeholders.
func (idx *Index) InsertTypeRelations(relations []TypeRelation) error {
	if len(relations) == 0 {
		return nil
	}

	// Prepare batch insert
	tx, err := idx.adapter.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Build insert statement with placeholders
	insertSQL := fmt.Sprintf(`
		INSERT INTO type_relations (repo_root, child_type, parent_type, relation, path, line)
		VALUES (%s, %s, %s, %s, %s, %s)
		ON CONFLICT (repo_root, child_type, parent_type, path) DO UPDATE SET
			relation = excluded.relation,
			line = excluded.line
	`,
		idx.dialect.Placeholder(1),
		idx.dialect.Placeholder(2),
		idx.dialect.Placeholder(3),
		idx.dialect.Placeholder(4),
		idx.dialect.Placeholder(5),
		idx.dialect.Placeholder(6),
	)

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return fmt.Errorf("prepare insert statement: %w", err)
	}
	defer stmt.Close()

	for _, rel := range relations {
		_, err := stmt.Exec(
			rel.RepoRoot,
			rel.ChildType,
			rel.ParentType,
			rel.Relation,
			rel.Path,
			rel.Line,
		)
		if err != nil {
			return fmt.Errorf("inserting type relation %s %s %s: %w", rel.ChildType, rel.Relation, rel.ParentType, err)
		}
	}

	return tx.Commit()
}

// DeleteTypeRelationsByFile deletes all type relations for a given file path.
// Used for incremental reindexing when a file changes.
func (idx *Index) DeleteTypeRelationsByFile(path string) error {
	deleteSQL := fmt.Sprintf(
		"DELETE FROM type_relations WHERE repo_root = %s AND path = %s",
		idx.dialect.Placeholder(1),
		idx.dialect.Placeholder(2),
	)
	_, err := idx.adapter.Exec(deleteSQL, idx.root, path)
	return err
}

// QueryImplementations finds all types that implement or extend a given interface/class.
func (idx *Index) QueryImplementations(parentType string, limit int) ([]TypeRelation, error) {
	query := fmt.Sprintf(`
		SELECT id, repo_root, child_type, parent_type, relation, path, line
		FROM type_relations
		WHERE repo_root = %s
		  AND parent_type = %s
		ORDER BY child_type
		LIMIT %s
	`,
		idx.dialect.Placeholder(1),
		idx.dialect.Placeholder(2),
		idx.dialect.Placeholder(3),
	)

	rows, err := idx.adapter.Query(query, idx.root, parentType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relations []TypeRelation
	for rows.Next() {
		var rel TypeRelation
		if err := rows.Scan(
			&rel.ID,
			&rel.RepoRoot,
			&rel.ChildType,
			&rel.ParentType,
			&rel.Relation,
			&rel.Path,
			&rel.Line,
		); err != nil {
			return nil, err
		}
		relations = append(relations, rel)
	}

	return relations, nil
}
