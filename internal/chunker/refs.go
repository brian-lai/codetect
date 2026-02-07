package chunker

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// SymbolReference represents a reference to a symbol (function call, type usage, etc.)
type SymbolReference struct {
	Name          string // Short name (e.g., "Handle")
	QualifiedName string // Best-effort qualified name (e.g., "AuthService.Handle")
	Kind          string // call, type_ref
	SourcePath    string
	SourceLine    int
	SourceScope   string // Qualified scope containing this reference
}

// TypeRelationship represents a type relationship (implements, extends, embeds)
type TypeRelationship struct {
	ChildType  string // Implementing/extending type
	ParentType string // Interface/base class
	Relation   string // implements, extends, embeds
	Path       string
	Line       int
}

// ExtractReferences parses a file and extracts all symbol references and type relationships.
// Returns references (calls, type usages) and type relationships (implements/extends/embeds).
func ExtractReferences(ctx context.Context, path string, content []byte) ([]SymbolReference, []TypeRelationship, error) {
	config := GetLanguageConfig(path)
	if config == nil {
		// Unsupported language
		return nil, nil, nil
	}

	// Parse with tree-sitter
	parser := sitter.NewParser()
	parser.SetLanguage(config.Language)

	tree, err := parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, nil, err
	}
	defer tree.Close()

	root := tree.RootNode()

	var refs []SymbolReference
	var rels []TypeRelationship

	// Track parent scopes during traversal
	stack := &scopeStack{}

	// Walk tree and extract references + type relationships
	walkTreeForRefs(root, content, path, config, stack, &refs, &rels)

	return refs, rels, nil
}

// walkTreeForRefs recursively traverses the AST and extracts references and type relationships.
func walkTreeForRefs(node *sitter.Node, content []byte, path string, config *LanguageConfig, stack *scopeStack, refs *[]SymbolReference, rels *[]TypeRelationship) {
	nodeType := node.Type()

	// Check if this is a scope-defining node (function, class, method)
	isScopeNode := false
	for _, splitNode := range config.SplitNodes {
		if nodeType == splitNode {
			isScopeNode = true
			break
		}
	}

	if isScopeNode {
		// Extract node name and push onto scope stack
		nodeName := extractNodeName(node, content, config)
		scopeKind := mapNodeTypeToKind(nodeType, config.Name)
		receiverType := extractReceiverType(node, content, config.Name)

		if nodeName != "" {
			stack.push(nodeName, scopeKind, receiverType)
			defer stack.pop()
		}
	}

	// Extract references based on node type and language
	switch config.Name {
	case "go":
		extractGoRefs(node, content, path, config, stack, refs, rels)
	case "typescript", "tsx":
		extractTypeScriptRefs(node, content, path, config, stack, refs, rels)
	}

	// Recurse into children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		walkTreeForRefs(child, content, path, config, stack, refs, rels)
	}
}

// extractGoRefs extracts Go-specific references and type relationships.
func extractGoRefs(node *sitter.Node, content []byte, path string, config *LanguageConfig, stack *scopeStack, refs *[]SymbolReference, rels *[]TypeRelationship) {
	nodeType := node.Type()
	line := int(node.StartPoint().Row) + 1

	switch nodeType {
	case "call_expression":
		// Extract function/method calls
		ref := extractGoCallExpression(node, content, path, stack, line)
		if ref != nil {
			*refs = append(*refs, *ref)
		}

	case "type_spec":
		// Extract struct/interface embedding (type relationships)
		extractGoTypeRelations(node, content, path, rels)
	}
}

// extractGoCallExpression extracts a Go call expression (function or method call).
func extractGoCallExpression(node *sitter.Node, content []byte, path string, stack *scopeStack, line int) *SymbolReference {
	// call_expression has function as first child
	if node.ChildCount() == 0 {
		return nil
	}

	funcNode := node.Child(0)
	funcType := funcNode.Type()

	var name, qualifiedName string

	switch funcType {
	case "identifier":
		// Direct function call: foo()
		name = string(content[funcNode.StartByte():funcNode.EndByte()])
		qualifiedName = name

	case "selector_expression":
		// Method call: obj.Method() or pkg.Func()
		// selector_expression has field "field" for the method name
		if fieldNode := funcNode.ChildByFieldName("field"); fieldNode != nil {
			name = string(content[fieldNode.StartByte():fieldNode.EndByte()])

			// Try to get receiver/package name
			if operandNode := funcNode.ChildByFieldName("operand"); operandNode != nil {
				receiver := string(content[operandNode.StartByte():operandNode.EndByte()])
				qualifiedName = receiver + "." + name
			} else {
				qualifiedName = name
			}
		}

	default:
		// Complex expression (e.g., function returned from another call)
		// Skip for now
		return nil
	}

	if name == "" {
		return nil
	}

	return &SymbolReference{
		Name:          name,
		QualifiedName: qualifiedName,
		Kind:          "call",
		SourcePath:    path,
		SourceLine:    line,
		SourceScope:   stack.current(),
	}
}

// extractGoTypeRelations extracts Go type relationships (struct/interface embedding).
func extractGoTypeRelations(node *sitter.Node, content []byte, path string, rels *[]TypeRelationship) {
	// type_spec → name (type being defined) + type (struct_type or interface_type)

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	childType := string(content[nameNode.StartByte():nameNode.EndByte()])

	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return
	}

	// Check for struct or interface embedding
	switch typeNode.Type() {
	case "struct_type":
		extractGoStructEmbedding(typeNode, content, path, childType, rels)
	case "interface_type":
		extractGoInterfaceEmbedding(typeNode, content, path, childType, rels)
	}
}

// extractGoStructEmbedding extracts embedded types from a struct definition.
func extractGoStructEmbedding(structNode *sitter.Node, content []byte, path string, childType string, rels *[]TypeRelationship) {
	// Find field_declaration_list
	fieldList := structNode.ChildByFieldName("fields")
	if fieldList == nil {
		return
	}

	for i := 0; i < int(fieldList.ChildCount()); i++ {
		child := fieldList.Child(i)
		if child.Type() != "field_declaration" {
			continue
		}

		// Embedded field: just a type, no field name
		// Check if this field has no name (embedded)
		if child.ChildCount() == 1 || (child.ChildCount() > 0 && child.Child(0).Type() != "field_identifier") {
			// This is an embedded field
			var parentType string
			typeChild := child.Child(0)

			switch typeChild.Type() {
			case "type_identifier":
				parentType = string(content[typeChild.StartByte():typeChild.EndByte()])
			case "qualified_type":
				// pkg.Type - extract just the type name
				if selector := typeChild.ChildByFieldName("name"); selector != nil {
					parentType = string(content[selector.StartByte():selector.EndByte()])
				}
			case "pointer_type":
				// *Type - extract the type name
				if elem := typeChild.ChildByFieldName("element"); elem != nil && elem.Type() == "type_identifier" {
					parentType = string(content[elem.StartByte():elem.EndByte()])
				}
			}

			if parentType != "" {
				*rels = append(*rels, TypeRelationship{
					ChildType:  childType,
					ParentType: parentType,
					Relation:   "embeds",
					Path:       path,
					Line:       int(child.StartPoint().Row) + 1,
				})
			}
		}
	}
}

// extractGoInterfaceEmbedding extracts embedded interfaces from an interface definition.
func extractGoInterfaceEmbedding(interfaceNode *sitter.Node, content []byte, path string, childType string, rels *[]TypeRelationship) {
	// Find method_spec_list (which contains both methods and embedded interfaces)
	methodList := interfaceNode.ChildByFieldName("methods")
	if methodList == nil {
		return
	}

	for i := 0; i < int(methodList.ChildCount()); i++ {
		child := methodList.Child(i)

		// Embedded interface: type_identifier or qualified_type
		var parentType string
		switch child.Type() {
		case "type_identifier":
			parentType = string(content[child.StartByte():child.EndByte()])
		case "qualified_type":
			// pkg.Interface - extract just the type name
			if selector := child.ChildByFieldName("name"); selector != nil {
				parentType = string(content[selector.StartByte():selector.EndByte()])
			}
		}

		if parentType != "" {
			*rels = append(*rels, TypeRelationship{
				ChildType:  childType,
				ParentType: parentType,
				Relation:   "embeds",
				Path:       path,
				Line:       int(child.StartPoint().Row) + 1,
			})
		}
	}
}

// extractTypeScriptRefs extracts TypeScript-specific references and type relationships.
func extractTypeScriptRefs(node *sitter.Node, content []byte, path string, config *LanguageConfig, stack *scopeStack, refs *[]SymbolReference, rels *[]TypeRelationship) {
	nodeType := node.Type()
	line := int(node.StartPoint().Row) + 1

	switch nodeType {
	case "call_expression":
		// Extract function/method calls
		ref := extractTypeScriptCallExpression(node, content, path, stack, line)
		if ref != nil {
			*refs = append(*refs, *ref)
		}

	case "class_declaration":
		// Extract implements/extends clauses
		extractTypeScriptTypeRelations(node, content, path, rels)

	case "interface_declaration":
		// Extract extends clauses
		extractTypeScriptInterfaceRelations(node, content, path, rels)
	}
}

// extractTypeScriptCallExpression extracts a TypeScript call expression.
func extractTypeScriptCallExpression(node *sitter.Node, content []byte, path string, stack *scopeStack, line int) *SymbolReference {
	// call_expression → function (identifier or member_expression)
	funcNode := node.ChildByFieldName("function")
	if funcNode == nil {
		return nil
	}

	var name, qualifiedName string

	switch funcNode.Type() {
	case "identifier":
		// Direct function call: foo()
		name = string(content[funcNode.StartByte():funcNode.EndByte()])
		qualifiedName = name

	case "member_expression":
		// Method call: obj.method()
		if property := funcNode.ChildByFieldName("property"); property != nil {
			name = string(content[property.StartByte():property.EndByte()])

			// Try to get object name
			if object := funcNode.ChildByFieldName("object"); object != nil {
				receiver := string(content[object.StartByte():object.EndByte()])
				qualifiedName = receiver + "." + name
			} else {
				qualifiedName = name
			}
		}

	default:
		// Complex expression
		return nil
	}

	if name == "" {
		return nil
	}

	return &SymbolReference{
		Name:          name,
		QualifiedName: qualifiedName,
		Kind:          "call",
		SourcePath:    path,
		SourceLine:    line,
		SourceScope:   stack.current(),
	}
}

// extractTypeScriptTypeRelations extracts implements/extends clauses from class declarations.
func extractTypeScriptTypeRelations(node *sitter.Node, content []byte, path string, rels *[]TypeRelationship) {
	// Get class name
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	childType := string(content[nameNode.StartByte():nameNode.EndByte()])
	line := int(node.StartPoint().Row) + 1

	// Extract implements clause
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "implements_clause" {
			extractTypeScriptImplementsClause(child, content, path, childType, line, rels)
		} else if child.Type() == "class_heritage" {
			// Look for extends within class_heritage
			for j := 0; j < int(child.ChildCount()); j++ {
				heritage := child.Child(j)
				if heritage.Type() == "extends_clause" {
					extractTypeScriptExtendsClause(heritage, content, path, childType, line, rels, "extends")
				} else if heritage.Type() == "implements_clause" {
					extractTypeScriptImplementsClause(heritage, content, path, childType, line, rels)
				}
			}
		}
	}
}

// extractTypeScriptInterfaceRelations extracts extends clauses from interface declarations.
func extractTypeScriptInterfaceRelations(node *sitter.Node, content []byte, path string, rels *[]TypeRelationship) {
	// Get interface name
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	childType := string(content[nameNode.StartByte():nameNode.EndByte()])
	line := int(node.StartPoint().Row) + 1

	// Extract extends clause
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "extends_clause" || child.Type() == "extends_type_clause" {
			extractTypeScriptExtendsClause(child, content, path, childType, line, rels, "extends")
		}
	}
}

// extractTypeScriptImplementsClause extracts parent types from an implements clause.
func extractTypeScriptImplementsClause(node *sitter.Node, content []byte, path string, childType string, line int, rels *[]TypeRelationship) {
	// implements_clause contains type references
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		parentType := extractTypeName(child, content)
		if parentType != "" {
			*rels = append(*rels, TypeRelationship{
				ChildType:  childType,
				ParentType: parentType,
				Relation:   "implements",
				Path:       path,
				Line:       line,
			})
		}
	}
}

// extractTypeScriptExtendsClause extracts parent types from an extends clause.
func extractTypeScriptExtendsClause(node *sitter.Node, content []byte, path string, childType string, line int, rels *[]TypeRelationship, relation string) {
	// extends_clause contains type references
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		parentType := extractTypeName(child, content)
		if parentType != "" {
			*rels = append(*rels, TypeRelationship{
				ChildType:  childType,
				ParentType: parentType,
				Relation:   relation,
				Path:       path,
				Line:       line,
			})
		}
	}
}

// extractTypeName extracts a type name from a type node (handles type_identifier, generic_type, etc.)
func extractTypeName(node *sitter.Node, content []byte) string {
	switch node.Type() {
	case "type_identifier":
		return string(content[node.StartByte():node.EndByte()])
	case "identifier":
		return string(content[node.StartByte():node.EndByte()])
	case "generic_type":
		// Extract base type name (before <...>)
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			return string(content[nameNode.StartByte():nameNode.EndByte()])
		}
	case "nested_type_identifier":
		// module.Type - extract just Type
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			return string(content[nameNode.StartByte():nameNode.EndByte()])
		}
	}
	return ""
}

// Helper functions (wrappers around existing chunker methods for reuse)

func extractNodeName(node *sitter.Node, content []byte, config *LanguageConfig) string {
	// First try configured field names
	for _, field := range config.NameFields {
		if nameNode := node.ChildByFieldName(field); nameNode != nil {
			return string(content[nameNode.StartByte():nameNode.EndByte()])
		}
	}

	// For some languages, the name might be nested deeper
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" || child.Type() == "property_identifier" {
			return string(content[child.StartByte():child.EndByte()])
		}
	}

	return ""
}

func mapNodeTypeToKind(nodeType, lang string) string {
	// Language-agnostic kind mapping
	switch {
	case strings.Contains(nodeType, "function"):
		return "function"
	case strings.Contains(nodeType, "method"):
		return "method"
	case strings.Contains(nodeType, "class"):
		return "class"
	case strings.Contains(nodeType, "struct"):
		return "struct"
	case strings.Contains(nodeType, "interface"):
		return "interface"
	case strings.Contains(nodeType, "trait"):
		return "trait"
	default:
		return nodeType
	}
}

func extractReceiverType(node *sitter.Node, content []byte, lang string) string {
	switch lang {
	case "go":
		// For Go methods: method_declaration → receiver → parameter_list → parameter_declaration → type
		if receiverNode := node.ChildByFieldName("receiver"); receiverNode != nil {
			// receiver is a parameter_list with one parameter
			for i := 0; i < int(receiverNode.ChildCount()); i++ {
				param := receiverNode.Child(i)
				if param.Type() == "parameter_declaration" {
					// Get the type (could be identifier, pointer_type, etc.)
					if typeNode := param.ChildByFieldName("type"); typeNode != nil {
						typeStr := string(content[typeNode.StartByte():typeNode.EndByte()])
						// Remove pointer indicator if present
						return strings.TrimPrefix(typeStr, "*")
					}
				}
			}
		}

	case "typescript", "tsx":
		// For TS methods: look up the tree to find the parent class
		parent := node.Parent()
		for parent != nil {
			if parent.Type() == "class_declaration" {
				if nameNode := parent.ChildByFieldName("name"); nameNode != nil {
					return string(content[nameNode.StartByte():nameNode.EndByte()])
				}
			}
			parent = parent.Parent()
		}

	case "python":
		// For Python methods: look for class definition up the tree
		parent := node.Parent()
		for parent != nil {
			if parent.Type() == "class_definition" {
				if nameNode := parent.ChildByFieldName("name"); nameNode != nil {
					return string(content[nameNode.StartByte():nameNode.EndByte()])
				}
			}
			parent = parent.Parent()
		}

	case "rust":
		// For Rust: impl blocks define methods for types
		parent := node.Parent()
		for parent != nil {
			if parent.Type() == "impl_item" {
				if typeNode := parent.ChildByFieldName("type"); typeNode != nil {
					return string(content[typeNode.StartByte():typeNode.EndByte()])
				}
			}
			parent = parent.Parent()
		}
	}

	return ""
}
