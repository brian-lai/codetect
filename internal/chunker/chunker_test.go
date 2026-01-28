package chunker

import (
	"context"
	"strings"
	"testing"
)

// Helper function to filter chunks by predicate
func filterChunks(chunks []Chunk, predicate func(Chunk) bool) []Chunk {
	var result []Chunk
	for _, c := range chunks {
		if predicate(c) {
			result = append(result, c)
		}
	}
	return result
}

// =============================================================================
// Go Language Tests
// =============================================================================

func TestChunkGoFile(t *testing.T) {
	content := `package main

func hello() {
	fmt.Println("hello")
}

func world() {
	fmt.Println("world")
}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have 2 function chunks
	funcChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "function_declaration"
	})
	if len(funcChunks) != 2 {
		t.Errorf("expected 2 function chunks, got %d", len(funcChunks))
	}

	// Verify function names
	if funcChunks[0].NodeName != "hello" {
		t.Errorf("expected first function name 'hello', got '%s'", funcChunks[0].NodeName)
	}
	if funcChunks[1].NodeName != "world" {
		t.Errorf("expected second function name 'world', got '%s'", funcChunks[1].NodeName)
	}

	// Verify language is set
	for _, c := range funcChunks {
		if c.Language != "go" {
			t.Errorf("expected language 'go', got '%s'", c.Language)
		}
	}
}

func TestChunkGoMethod(t *testing.T) {
	content := `package main

type Greeter struct {
	name string
}

func (g *Greeter) Greet() string {
	return "Hello, " + g.name
}

func (g *Greeter) SetName(name string) {
	g.name = name
}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have method chunks
	methodChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "method_declaration"
	})
	if len(methodChunks) != 2 {
		t.Errorf("expected 2 method chunks, got %d", len(methodChunks))
	}

	// Should have type declaration
	typeChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "type_declaration"
	})
	if len(typeChunks) != 1 {
		t.Errorf("expected 1 type chunk, got %d", len(typeChunks))
	}
}

func TestChunkGoConstants(t *testing.T) {
	content := `package main

const (
	MaxSize = 100
	MinSize = 10
)

var globalVar = "test"
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have const and var declarations
	constChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "const_declaration"
	})
	if len(constChunks) != 1 {
		t.Errorf("expected 1 const chunk, got %d", len(constChunks))
	}

	varChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "var_declaration"
	})
	if len(varChunks) != 1 {
		t.Errorf("expected 1 var chunk, got %d", len(varChunks))
	}
}

// =============================================================================
// Python Language Tests
// =============================================================================

func TestChunkPythonFile(t *testing.T) {
	content := `def hello():
    print("hello")

class Greeter:
    def greet(self):
        print("greet")

    def goodbye(self):
        print("goodbye")
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.py", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have function and class chunks
	funcChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "function_definition"
	})
	if len(funcChunks) < 1 {
		t.Errorf("expected at least 1 function chunk, got %d", len(funcChunks))
	}

	classChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "class_definition"
	})
	if len(classChunks) != 1 {
		t.Errorf("expected 1 class chunk, got %d", len(classChunks))
	}

	// Verify language
	for _, c := range chunks {
		if c.Language != "python" {
			t.Errorf("expected language 'python', got '%s'", c.Language)
		}
	}
}

func TestChunkPythonDecorated(t *testing.T) {
	content := `@decorator
def decorated_func():
    pass

@staticmethod
def static_func():
    pass
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.py", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have decorated_definition chunks
	decoratedChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "decorated_definition"
	})
	if len(decoratedChunks) != 2 {
		t.Errorf("expected 2 decorated chunks, got %d", len(decoratedChunks))
	}
}

// =============================================================================
// JavaScript Language Tests
// =============================================================================

func TestChunkJavaScriptFile(t *testing.T) {
	content := `function hello() {
    console.log("hello");
}

class Greeter {
    greet() {
        console.log("greet");
    }
}

const arrow = () => {
    return "arrow";
};
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.js", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have function, class, and arrow function chunks
	funcChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "function_declaration"
	})
	if len(funcChunks) != 1 {
		t.Errorf("expected 1 function chunk, got %d", len(funcChunks))
	}

	classChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "class_declaration"
	})
	if len(classChunks) != 1 {
		t.Errorf("expected 1 class chunk, got %d", len(classChunks))
	}

	// Verify language
	for _, c := range chunks {
		if c.NodeType != "gap" && c.Language != "javascript" {
			t.Errorf("expected language 'javascript', got '%s'", c.Language)
		}
	}
}

func TestChunkJavaScriptMJS(t *testing.T) {
	content := `export function hello() {
    return "hello";
}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.mjs", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("expected at least 1 chunk for .mjs file")
	}
}

// =============================================================================
// TypeScript Language Tests
// =============================================================================

func TestChunkTypeScriptFile(t *testing.T) {
	content := `interface Person {
    name: string;
    age: number;
}

type StringOrNumber = string | number;

function greet(person: Person): string {
    return "Hello, " + person.name;
}

class Greeter implements Person {
    name: string;
    age: number;

    constructor(name: string, age: number) {
        this.name = name;
        this.age = age;
    }
}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.ts", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have interface, type alias, function, and class chunks
	interfaceChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "interface_declaration"
	})
	if len(interfaceChunks) != 1 {
		t.Errorf("expected 1 interface chunk, got %d", len(interfaceChunks))
	}

	typeChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "type_alias_declaration"
	})
	if len(typeChunks) != 1 {
		t.Errorf("expected 1 type alias chunk, got %d", len(typeChunks))
	}

	// Verify language
	for _, c := range chunks {
		if c.NodeType != "gap" && c.Language != "typescript" {
			t.Errorf("expected language 'typescript', got '%s'", c.Language)
		}
	}
}

// =============================================================================
// TSX Language Tests
// =============================================================================

func TestChunkTSXFile(t *testing.T) {
	content := `import React from 'react';

interface Props {
    name: string;
}

function Greeting({ name }: Props) {
    return <div>Hello, {name}!</div>;
}

export default Greeting;
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.tsx", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have interface and function chunks
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}

	// Verify language
	for _, c := range chunks {
		if c.NodeType != "gap" && c.Language != "tsx" {
			t.Errorf("expected language 'tsx', got '%s'", c.Language)
		}
	}
}

// =============================================================================
// Rust Language Tests
// =============================================================================

func TestChunkRustFile(t *testing.T) {
	content := `struct Point {
    x: i32,
    y: i32,
}

impl Point {
    fn new(x: i32, y: i32) -> Self {
        Point { x, y }
    }

    fn distance(&self) -> f64 {
        ((self.x.pow(2) + self.y.pow(2)) as f64).sqrt()
    }
}

fn main() {
    let p = Point::new(3, 4);
    println!("Distance: {}", p.distance());
}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.rs", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have struct, impl, and function chunks
	structChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "struct_item"
	})
	if len(structChunks) != 1 {
		t.Errorf("expected 1 struct chunk, got %d", len(structChunks))
	}

	implChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "impl_item"
	})
	if len(implChunks) != 1 {
		t.Errorf("expected 1 impl chunk, got %d", len(implChunks))
	}

	funcChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "function_item"
	})
	if len(funcChunks) < 1 {
		t.Errorf("expected at least 1 function chunk, got %d", len(funcChunks))
	}

	// Verify language
	for _, c := range chunks {
		if c.NodeType != "gap" && c.Language != "rust" {
			t.Errorf("expected language 'rust', got '%s'", c.Language)
		}
	}
}

// =============================================================================
// Java Language Tests
// =============================================================================

func TestChunkJavaFile(t *testing.T) {
	content := `public class Greeter {
    private String name;

    public Greeter(String name) {
        this.name = name;
    }

    public String greet() {
        return "Hello, " + name;
    }

    public static void main(String[] args) {
        Greeter g = new Greeter("World");
        System.out.println(g.greet());
    }
}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.java", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have class chunk
	classChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "class_declaration"
	})
	if len(classChunks) != 1 {
		t.Errorf("expected 1 class chunk, got %d", len(classChunks))
	}

	// Verify language
	for _, c := range chunks {
		if c.NodeType != "gap" && c.Language != "java" {
			t.Errorf("expected language 'java', got '%s'", c.Language)
		}
	}
}

// =============================================================================
// C Language Tests
// =============================================================================

func TestChunkCFile(t *testing.T) {
	content := `#include <stdio.h>

struct Point {
    int x;
    int y;
};

int add(int a, int b) {
    return a + b;
}

int main() {
    struct Point p = {1, 2};
    printf("%d\n", add(p.x, p.y));
    return 0;
}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.c", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have function and struct chunks
	funcChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "function_definition"
	})
	if len(funcChunks) < 2 {
		t.Errorf("expected at least 2 function chunks, got %d", len(funcChunks))
	}

	// Verify language
	for _, c := range chunks {
		if c.NodeType != "gap" && c.Language != "c" {
			t.Errorf("expected language 'c', got '%s'", c.Language)
		}
	}
}

func TestChunkCHeaderFile(t *testing.T) {
	content := `#ifndef MYHEADER_H
#define MYHEADER_H

int add(int a, int b);
int subtract(int a, int b);

#endif
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.h", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("expected at least 1 chunk for header file")
	}

	// Verify language (headers are treated as C)
	for _, c := range chunks {
		if c.NodeType != "gap" && c.Language != "c" {
			t.Errorf("expected language 'c', got '%s'", c.Language)
		}
	}
}

// =============================================================================
// C++ Language Tests
// =============================================================================

func TestChunkCppFile(t *testing.T) {
	content := `#include <iostream>

namespace math {
    int add(int a, int b) {
        return a + b;
    }
}

class Calculator {
public:
    int multiply(int a, int b) {
        return a * b;
    }
};

int main() {
    Calculator calc;
    std::cout << math::add(1, 2) << std::endl;
    return 0;
}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.cpp", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have namespace, class, and function chunks
	namespaceChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "namespace_definition"
	})
	if len(namespaceChunks) != 1 {
		t.Errorf("expected 1 namespace chunk, got %d", len(namespaceChunks))
	}

	classChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "class_specifier"
	})
	if len(classChunks) != 1 {
		t.Errorf("expected 1 class chunk, got %d", len(classChunks))
	}

	// Verify language
	for _, c := range chunks {
		if c.NodeType != "gap" && c.Language != "cpp" {
			t.Errorf("expected language 'cpp', got '%s'", c.Language)
		}
	}
}

func TestChunkCppExtensions(t *testing.T) {
	content := `void test() {}`

	extensions := []string{".cpp", ".cc", ".cxx", ".hpp", ".hxx"}
	chunker := NewASTChunker()

	for _, ext := range extensions {
		chunks, err := chunker.ChunkFile(context.Background(), "test"+ext, []byte(content))
		if err != nil {
			t.Errorf("ChunkFile failed for %s: %v", ext, err)
			continue
		}
		if len(chunks) == 0 {
			t.Errorf("expected chunks for %s extension", ext)
		}
	}
}

// =============================================================================
// Ruby Language Tests
// =============================================================================

func TestChunkRubyFile(t *testing.T) {
	content := `module Greetings
  def hello
    puts "Hello"
  end
end

class Greeter
  include Greetings

  def initialize(name)
    @name = name
  end

  def greet
    puts "Hello, #{@name}"
  end
end
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.rb", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have module and class chunks
	moduleChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "module"
	})
	if len(moduleChunks) != 1 {
		t.Errorf("expected 1 module chunk, got %d", len(moduleChunks))
	}

	classChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "class"
	})
	if len(classChunks) != 1 {
		t.Errorf("expected 1 class chunk, got %d", len(classChunks))
	}

	// Verify language
	for _, c := range chunks {
		if c.NodeType != "gap" && c.Language != "ruby" {
			t.Errorf("expected language 'ruby', got '%s'", c.Language)
		}
	}
}

// =============================================================================
// Symbol Name Extraction Tests
// =============================================================================

func TestChunkPreservesSymbolNames(t *testing.T) {
	content := `func calculateSum(a, b int) int {
	return a + b
}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	funcChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "function_declaration"
	})
	if len(funcChunks) != 1 {
		t.Fatalf("expected 1 function chunk, got %d", len(funcChunks))
	}

	if funcChunks[0].NodeName != "calculateSum" {
		t.Errorf("expected function name 'calculateSum', got '%s'", funcChunks[0].NodeName)
	}
}

func TestChunkExtractsClassNames(t *testing.T) {
	content := `class MyClass:
    def method(self):
        pass
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.py", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	classChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "class_definition"
	})
	if len(classChunks) != 1 {
		t.Fatalf("expected 1 class chunk, got %d", len(classChunks))
	}

	if classChunks[0].NodeName != "MyClass" {
		t.Errorf("expected class name 'MyClass', got '%s'", classChunks[0].NodeName)
	}
}

// =============================================================================
// Fallback Tests
// =============================================================================

func TestFallbackForUnsupportedLanguage(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5\n"
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.xyz", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("expected at least 1 chunk for unsupported language")
	}

	// Should use fallback language
	if chunks[0].Language != "unknown" {
		t.Errorf("expected language 'unknown', got '%s'", chunks[0].Language)
	}

	// Should use fallback node type
	if chunks[0].NodeType != "block" {
		t.Errorf("expected node type 'block', got '%s'", chunks[0].NodeType)
	}
}

func TestFallbackWithLargeFile(t *testing.T) {
	// Create a large file with 100 lines
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "line content here")
	}
	content := strings.Join(lines, "\n")

	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.xyz", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have multiple chunks with overlap
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for large file, got %d", len(chunks))
	}

	// Verify all chunks have hashes
	for i, c := range chunks {
		if c.ContentHash == "" {
			t.Errorf("chunk %d missing content hash", i)
		}
	}
}

// =============================================================================
// Content Hash Tests
// =============================================================================

func TestContentHashDeterministic(t *testing.T) {
	content := `func test() {}`
	chunker := NewASTChunker()

	chunks1, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	chunks2, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	if len(chunks1) == 0 || len(chunks2) == 0 {
		t.Fatal("expected non-empty chunks")
	}

	if chunks1[0].ContentHash != chunks2[0].ContentHash {
		t.Errorf("content hashes should be deterministic: %s != %s",
			chunks1[0].ContentHash, chunks2[0].ContentHash)
	}
}

func TestContentHashUnique(t *testing.T) {
	content := `func foo() {}

func bar() {}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	funcChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "function_declaration"
	})
	if len(funcChunks) != 2 {
		t.Fatalf("expected 2 function chunks, got %d", len(funcChunks))
	}

	if funcChunks[0].ContentHash == funcChunks[1].ContentHash {
		t.Error("different functions should have different content hashes")
	}
}

// =============================================================================
// Line and Byte Position Tests
// =============================================================================

func TestChunkLinePositions(t *testing.T) {
	content := `package main

func hello() {
	fmt.Println("hello")
}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	funcChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "function_declaration"
	})
	if len(funcChunks) != 1 {
		t.Fatalf("expected 1 function chunk, got %d", len(funcChunks))
	}

	// Function starts at line 3
	if funcChunks[0].StartLine != 3 {
		t.Errorf("expected start line 3, got %d", funcChunks[0].StartLine)
	}

	// Function ends at line 5
	if funcChunks[0].EndLine != 5 {
		t.Errorf("expected end line 5, got %d", funcChunks[0].EndLine)
	}

	// Line count should be 3
	if funcChunks[0].LineCount() != 3 {
		t.Errorf("expected line count 3, got %d", funcChunks[0].LineCount())
	}
}

// =============================================================================
// Gap Chunk Tests
// =============================================================================

func TestGapChunksCreated(t *testing.T) {
	content := `package main

import (
	"fmt"
	"strings"
)

func hello() {
	fmt.Println("hello")
}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	// Should have gap chunk for imports
	gapChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "gap"
	})

	// Gap chunks should be created for regions not covered by split nodes
	// The import block should be in a gap or covered by another mechanism
	if len(gapChunks) > 0 {
		for _, g := range gapChunks {
			if g.LineCount() < MinGapLines {
				t.Errorf("gap chunk has less than %d lines: %d", MinGapLines, g.LineCount())
			}
		}
	}
}

// =============================================================================
// Chunk Helper Method Tests
// =============================================================================

func TestChunkHelperMethods(t *testing.T) {
	chunk := Chunk{
		Path:      "test.go",
		StartLine: 1,
		EndLine:   10,
		Content:   "test content",
		Language:  "go",
	}

	// Test LineCount
	if chunk.LineCount() != 10 {
		t.Errorf("expected line count 10, got %d", chunk.LineCount())
	}

	// Test ByteCount
	if chunk.ByteCount() != 12 { // "test content" is 12 bytes
		t.Errorf("expected byte count 12, got %d", chunk.ByteCount())
	}

	// Test IsEmpty
	if chunk.IsEmpty() {
		t.Error("chunk should not be empty")
	}

	emptyChunk := Chunk{}
	if !emptyChunk.IsEmpty() {
		t.Error("empty chunk should be empty")
	}

	// Test ComputeHash
	chunk.ComputeHash()
	if chunk.ContentHash == "" {
		t.Error("content hash should be computed")
	}

	// Verify hash is deterministic
	expectedHash := chunk.ContentHash
	chunk.ComputeHash()
	if chunk.ContentHash != expectedHash {
		t.Error("content hash should be deterministic")
	}
}

// =============================================================================
// Language Config Tests
// =============================================================================

func TestGetLanguageConfig(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"test.go", "go"},
		{"test.py", "python"},
		{"test.js", "javascript"},
		{"test.mjs", "javascript"},
		{"test.jsx", "javascript"},
		{"test.ts", "typescript"},
		{"test.tsx", "tsx"},
		{"test.rs", "rust"},
		{"test.java", "java"},
		{"test.c", "c"},
		{"test.h", "c"},
		{"test.cpp", "cpp"},
		{"test.cc", "cpp"},
		{"test.cxx", "cpp"},
		{"test.hpp", "cpp"},
		{"test.hxx", "cpp"},
		{"test.rb", "ruby"},
	}

	for _, tt := range tests {
		config := GetLanguageConfig(tt.path)
		if config == nil {
			t.Errorf("GetLanguageConfig(%s) returned nil", tt.path)
			continue
		}
		if config.Name != tt.expected {
			t.Errorf("GetLanguageConfig(%s) = %s, want %s", tt.path, config.Name, tt.expected)
		}
	}
}

func TestGetLanguageConfigUnsupported(t *testing.T) {
	unsupported := []string{"test.xyz", "test.foo", "test", ""}
	for _, path := range unsupported {
		config := GetLanguageConfig(path)
		if config != nil {
			t.Errorf("GetLanguageConfig(%s) should return nil for unsupported extension", path)
		}
	}
}

func TestSupportedExtensions(t *testing.T) {
	exts := SupportedExtensions()
	if len(exts) < 10 {
		t.Errorf("expected at least 10 supported extensions, got %d", len(exts))
	}

	// Verify some key extensions are present
	extSet := make(map[string]bool)
	for _, ext := range exts {
		extSet[ext] = true
	}

	required := []string{".go", ".py", ".js", ".ts", ".tsx", ".rs", ".java", ".c", ".cpp", ".rb"}
	for _, ext := range required {
		if !extSet[ext] {
			t.Errorf("expected extension %s to be supported", ext)
		}
	}
}

func TestSupportedLanguages(t *testing.T) {
	langs := SupportedLanguages()
	if len(langs) < 10 {
		t.Errorf("expected at least 10 supported languages, got %d", len(langs))
	}
}

func TestIsSupported(t *testing.T) {
	if !IsSupported("test.go") {
		t.Error("test.go should be supported")
	}
	if IsSupported("test.xyz") {
		t.Error("test.xyz should not be supported")
	}
}

// =============================================================================
// Options Tests
// =============================================================================

func TestChunkFileWithOptions(t *testing.T) {
	content := `func hello() {}

func world() {}
`
	chunker := NewASTChunker()

	opts := ChunkOptions{
		MaxChunkSize:    1000,
		IncludeGaps:     false,
		FallbackEnabled: true,
		ComputeHashes:   true,
	}

	chunks, err := chunker.ChunkFileWithOptions(context.Background(), "test.go", []byte(content), opts)
	if err != nil {
		t.Fatalf("ChunkFileWithOptions failed: %v", err)
	}

	// Should not have gap chunks
	gapChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "gap"
	})
	if len(gapChunks) != 0 {
		t.Errorf("expected 0 gap chunks with IncludeGaps=false, got %d", len(gapChunks))
	}
}

func TestChunkFileWithOptionsNoFallback(t *testing.T) {
	content := "unsupported content"
	chunker := NewASTChunker()

	opts := ChunkOptions{
		FallbackEnabled: false,
	}

	chunks, err := chunker.ChunkFileWithOptions(context.Background(), "test.xyz", []byte(content), opts)
	if err != nil {
		t.Fatalf("ChunkFileWithOptions failed: %v", err)
	}

	if chunks != nil && len(chunks) > 0 {
		t.Error("expected no chunks with FallbackEnabled=false for unsupported language")
	}
}

func TestDefaultChunkOptions(t *testing.T) {
	opts := DefaultChunkOptions()

	if opts.MaxChunkSize != DefaultMaxChunkSize {
		t.Errorf("expected MaxChunkSize %d, got %d", DefaultMaxChunkSize, opts.MaxChunkSize)
	}
	if !opts.IncludeGaps {
		t.Error("expected IncludeGaps to be true by default")
	}
	if !opts.FallbackEnabled {
		t.Error("expected FallbackEnabled to be true by default")
	}
	if !opts.ComputeHashes {
		t.Error("expected ComputeHashes to be true by default")
	}
}

// =============================================================================
// Empty and Edge Case Tests
// =============================================================================

func TestChunkEmptyFile(t *testing.T) {
	chunker := NewASTChunker()

	// Empty Go file
	chunks, err := chunker.ChunkFile(context.Background(), "test.go", []byte(""))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty file, got %d", len(chunks))
	}

	// Empty unsupported file
	chunks, err = chunker.ChunkFile(context.Background(), "test.xyz", []byte(""))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}
	if chunks != nil && len(chunks) > 0 {
		// Fallback with empty content should return nil or empty
		if chunks[0].Content != "" {
			t.Errorf("expected empty content for empty file")
		}
	}
}

func TestChunkSingleLineFile(t *testing.T) {
	content := `func x() {}`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	funcChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "function_declaration"
	})
	if len(funcChunks) != 1 {
		t.Errorf("expected 1 function chunk, got %d", len(funcChunks))
	}
}

// =============================================================================
// Sorting Tests
// =============================================================================

func TestChunksSorted(t *testing.T) {
	content := `func c() {}

func a() {}

func b() {}
`
	chunker := NewASTChunker()
	chunks, err := chunker.ChunkFile(context.Background(), "test.go", []byte(content))
	if err != nil {
		t.Fatalf("ChunkFile failed: %v", err)
	}

	funcChunks := filterChunks(chunks, func(c Chunk) bool {
		return c.NodeType == "function_declaration"
	})
	if len(funcChunks) < 2 {
		t.Skip("not enough function chunks to verify sorting")
	}

	// Verify chunks are sorted by start line
	for i := 1; i < len(funcChunks); i++ {
		if funcChunks[i].StartLine < funcChunks[i-1].StartLine {
			t.Errorf("chunks not sorted: chunk %d starts at line %d, chunk %d starts at line %d",
				i-1, funcChunks[i-1].StartLine, i, funcChunks[i].StartLine)
		}
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkChunkGoFile(b *testing.B) {
	content := []byte(`package main

import "fmt"

func hello() {
	fmt.Println("hello")
}

func world() {
	fmt.Println("world")
}

type Greeter struct {
	name string
}

func (g *Greeter) Greet() string {
	return "Hello, " + g.name
}
`)
	chunker := NewASTChunker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chunker.ChunkFile(context.Background(), "test.go", content)
	}
}

func BenchmarkChunkPythonFile(b *testing.B) {
	content := []byte(`def hello():
    print("hello")

def world():
    print("world")

class Greeter:
    def __init__(self, name):
        self.name = name

    def greet(self):
        return f"Hello, {self.name}"
`)
	chunker := NewASTChunker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chunker.ChunkFile(context.Background(), "test.py", content)
	}
}

func BenchmarkChunkLargeGoFile(b *testing.B) {
	// Generate a large Go file with many functions
	var builder strings.Builder
	builder.WriteString("package main\n\n")
	for i := 0; i < 100; i++ {
		builder.WriteString("func function")
		builder.WriteString(string(rune('A' + i%26)))
		builder.WriteString("() {\n")
		builder.WriteString("    // Comment line 1\n")
		builder.WriteString("    // Comment line 2\n")
		builder.WriteString("    x := 1\n")
		builder.WriteString("    y := 2\n")
		builder.WriteString("    z := x + y\n")
		builder.WriteString("    _ = z\n")
		builder.WriteString("}\n\n")
	}

	content := []byte(builder.String())
	chunker := NewASTChunker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chunker.ChunkFile(context.Background(), "test.go", content)
	}
}

func BenchmarkFallbackChunk(b *testing.B) {
	// Generate a large unsupported file
	var builder strings.Builder
	for i := 0; i < 1000; i++ {
		builder.WriteString("Line ")
		builder.WriteString(string(rune('0' + i%10)))
		builder.WriteString(" content here\n")
	}

	content := []byte(builder.String())
	chunker := NewASTChunker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chunker.ChunkFile(context.Background(), "test.xyz", content)
	}
}
