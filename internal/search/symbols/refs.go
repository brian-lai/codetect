package symbols

import (
	"database/sql"
	"fmt"

	"codetect/internal/db"
)

// SymbolRef represents a single reference to a symbol at a specific location.
// Kind is "call" for function/method call sites, or "type_ref" for type usage sites.
type SymbolRef struct {
	Name          string
	QualifiedName string // e.g. "Receiver.Method" or "pkg.Func"; may be empty
	Kind          string // "call" | "type_ref"
	SourcePath    string
	SourceLine    int
	SourceScope   string // enclosing function/method name; may be empty
}

// TypeRelation represents a type hierarchy relationship between two types.
type TypeRelation struct {
	ChildType  string
	ParentType string
	Relation   string // "implements" | "extends" | "embeds"
	Path       string
	Line       int
}

// InsertRefs batch-upserts symbol references for the current repo.
// Must be called within a transaction. Existing rows matching the unique constraint
// (repo_root, source_path, source_line, name) are updated in place.
func (idx *Index) InsertRefs(tx db.Tx, refs []SymbolRef) error {
	if len(refs) == 0 {
		return nil
	}

	upsertSQL := idx.dialect.UpsertSQL(
		"symbol_refs",
		[]string{"repo_root", "name", "qualified_name", "kind", "source_path", "source_line", "source_scope"},
		[]string{"repo_root", "source_path", "source_line", "name"},
		[]string{"qualified_name", "kind", "source_scope"},
	)
	stmt, err := tx.Prepare(upsertSQL)
	if err != nil {
		return fmt.Errorf("preparing refs insert: %w", err)
	}
	defer stmt.Close()

	for _, r := range refs {
		if _, err := stmt.Exec(
			idx.root, r.Name, nullString(r.QualifiedName), r.Kind,
			r.SourcePath, r.SourceLine, nullString(r.SourceScope),
		); err != nil {
			// UpsertSQL uses ON CONFLICT DO UPDATE, so constraint violations should
			// not occur. Any error here is unexpected; surface it rather than losing data.
			return fmt.Errorf("inserting ref %q at %s:%d: %w", r.Name, r.SourcePath, r.SourceLine, err)
		}
	}
	return nil
}

// DeleteRefsByFile removes all symbol_refs rows for a given relative file path in this repo.
// Must be called within a transaction.
func (idx *Index) DeleteRefsByFile(tx db.Tx, path string) error {
	query := fmt.Sprintf(
		"DELETE FROM symbol_refs WHERE repo_root = %s AND source_path = %s",
		idx.dialect.Placeholder(1), idx.dialect.Placeholder(2),
	)
	_, err := tx.Exec(query, idx.root, path)
	return err
}

// InsertTypeRelations batch-upserts type relationship records for the current repo.
// Must be called within a transaction.
func (idx *Index) InsertTypeRelations(tx db.Tx, relations []TypeRelation) error {
	if len(relations) == 0 {
		return nil
	}

	upsertSQL := idx.dialect.UpsertSQL(
		"type_relations",
		[]string{"repo_root", "child_type", "parent_type", "relation", "path", "line"},
		[]string{"repo_root", "child_type", "parent_type", "path"},
		[]string{"relation", "line"},
	)
	stmt, err := tx.Prepare(upsertSQL)
	if err != nil {
		return fmt.Errorf("preparing type_relations insert: %w", err)
	}
	defer stmt.Close()

	for _, r := range relations {
		if _, err := stmt.Exec(
			idx.root, r.ChildType, r.ParentType, r.Relation, r.Path, r.Line,
		); err != nil {
			return fmt.Errorf("inserting type relation %q -> %q at %s:%d: %w", r.ChildType, r.ParentType, r.Path, r.Line, err)
		}
	}
	return nil
}

// DeleteTypeRelationsByFile removes all type_relations rows for a given relative file path.
// Must be called within a transaction.
func (idx *Index) DeleteTypeRelationsByFile(tx db.Tx, path string) error {
	query := fmt.Sprintf(
		"DELETE FROM type_relations WHERE repo_root = %s AND path = %s",
		idx.dialect.Placeholder(1), idx.dialect.Placeholder(2),
	)
	_, err := tx.Exec(query, idx.root, path)
	return err
}

// QueryCallers returns all call-kind symbol_refs where name or qualified_name matches.
// Results are ordered by source_path and source_line.
func (idx *Index) QueryCallers(name string, limit int) ([]SymbolRef, error) {
	if limit <= 0 {
		limit = 20
	}
	query := fmt.Sprintf(`
		SELECT name, qualified_name, kind, source_path, source_line, source_scope
		FROM symbol_refs
		WHERE repo_root = %s
		  AND kind = 'call'
		  AND (name = %s OR qualified_name = %s OR qualified_name LIKE %s)
		ORDER BY source_path, source_line
		LIMIT %s`,
		idx.dialect.Placeholder(1),
		idx.dialect.Placeholder(2),
		idx.dialect.Placeholder(3),
		idx.dialect.Placeholder(4),
		idx.dialect.Placeholder(5),
	)
	rows, err := idx.adapter.Query(query, idx.root, name, name, "%."+name, limit)
	if err != nil {
		return nil, fmt.Errorf("querying callers: %w", err)
	}
	defer rows.Close()
	return scanSymbolRefs(rows)
}

// QueryRefs returns symbol_refs matching name, optionally filtered by kind.
// kind may be "call", "type_ref", or "all" (no filter).
func (idx *Index) QueryRefs(name, kind string, limit int) ([]SymbolRef, error) {
	if limit <= 0 {
		limit = 50
	}

	var query string
	var args []any

	if kind == "" || kind == "all" {
		query = fmt.Sprintf(`
			SELECT name, qualified_name, kind, source_path, source_line, source_scope
			FROM symbol_refs
			WHERE repo_root = %s
			  AND (name = %s OR qualified_name = %s OR qualified_name LIKE %s)
			ORDER BY source_path, source_line
			LIMIT %s`,
			idx.dialect.Placeholder(1),
			idx.dialect.Placeholder(2),
			idx.dialect.Placeholder(3),
			idx.dialect.Placeholder(4),
			idx.dialect.Placeholder(5),
		)
		args = []any{idx.root, name, name, "%."+name, limit}
	} else {
		query = fmt.Sprintf(`
			SELECT name, qualified_name, kind, source_path, source_line, source_scope
			FROM symbol_refs
			WHERE repo_root = %s
			  AND kind = %s
			  AND (name = %s OR qualified_name = %s OR qualified_name LIKE %s)
			ORDER BY source_path, source_line
			LIMIT %s`,
			idx.dialect.Placeholder(1),
			idx.dialect.Placeholder(2),
			idx.dialect.Placeholder(3),
			idx.dialect.Placeholder(4),
			idx.dialect.Placeholder(5),
			idx.dialect.Placeholder(6),
		)
		args = []any{idx.root, kind, name, name, "%."+name, limit}
	}

	rows, err := idx.adapter.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying refs: %w", err)
	}
	defer rows.Close()
	return scanSymbolRefs(rows)
}

// QueryImplementations returns all type_relations rows where parent_type matches name.
// Results are ordered by child_type.
func (idx *Index) QueryImplementations(parentType string, limit int) ([]TypeRelation, error) {
	if limit <= 0 {
		limit = 20
	}
	query := fmt.Sprintf(`
		SELECT child_type, parent_type, relation, path, line
		FROM type_relations
		WHERE repo_root = %s AND parent_type = %s
		ORDER BY child_type
		LIMIT %s`,
		idx.dialect.Placeholder(1),
		idx.dialect.Placeholder(2),
		idx.dialect.Placeholder(3),
	)
	rows, err := idx.adapter.Query(query, idx.root, parentType, limit)
	if err != nil {
		return nil, fmt.Errorf("querying implementations: %w", err)
	}
	defer rows.Close()

	var result []TypeRelation
	for rows.Next() {
		var r TypeRelation
		if err := rows.Scan(&r.ChildType, &r.ParentType, &r.Relation, &r.Path, &r.Line); err != nil {
			return nil, fmt.Errorf("scanning type_relation: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func scanSymbolRefs(rows db.Rows) ([]SymbolRef, error) {
	var result []SymbolRef
	for rows.Next() {
		var r SymbolRef
		var qualifiedName, sourceScope sql.NullString
		if err := rows.Scan(&r.Name, &qualifiedName, &r.Kind, &r.SourcePath, &r.SourceLine, &sourceScope); err != nil {
			return nil, fmt.Errorf("scanning symbol_ref: %w", err)
		}
		r.QualifiedName = qualifiedName.String
		r.SourceScope = sourceScope.String
		result = append(result, r)
	}
	return result, rows.Err()
}
