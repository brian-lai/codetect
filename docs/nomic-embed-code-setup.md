# Using Nomic-Embed-Code with codetect

Nomic-Embed-Code is a code-optimized embedding model trained specifically for code retrieval tasks. It significantly outperforms general-purpose embedding models on code search benchmarks.

## Performance Comparison

| Model | CodeSearchNet MRR | Dimensions | Best For |
|-------|-------------------|------------|----------|
| **nomic-embed-code** | **77.9** | 3584 | Code search & retrieval |
| nomic-embed-text | ~57 | 768 | General text |
| OpenAI Ada-002 | 71.3 | 1536 | General embeddings |

Nomic-Embed-Code shows **~36% better performance** on code search tasks compared to nomic-embed-text.

## Quick Start

### 1. Pull the Model

```bash
# Quantized version (Recommended - 4.4 GB)
ollama pull manutic/nomic-embed-code:7b-Q4_K_M

# Or full precision (14 GB)
ollama pull manutic/nomic-embed-code:7b
```

**Recommendation:** Use the quantized `:7b-Q4_K_M` version for:
- 41% smaller size (4.4 GB vs 7.5 GB)
- Faster inference
- Lower memory usage
- Minimal quality loss

### 2. Configure codetect

Set environment variables:

```bash
export CODETECT_EMBEDDING_PROVIDER=ollama
export CODETECT_EMBEDDING_MODEL=manutic/nomic-embed-code:7b-Q4_K_M
```

Or use `.codetect.yaml`:

```yaml
embedding:
  provider: ollama
  model: manutic/nomic-embed-code:7b-Q4_K_M
```

### 3. Index Your Project

```bash
cd /path/to/your/project

# Index with tree-sitter AST chunking
codetect index

# Generate embeddings with nomic-embed-code
codetect embed

# Verify it worked
codetect doctor
```

### 4. Use in Claude Code

```bash
# Start Claude Code - codetect will use nomic-embed-code automatically
claude
```

The MCP integration will now use code-optimized embeddings for semantic search.

## Supported Languages

Nomic-Embed-Code is trained on:
- Python
- JavaScript/TypeScript
- Java
- Go
- Ruby
- PHP

But works well on other languages due to its code understanding.

## Model Details

- **Architecture:** Qwen2-based transformer
- **Parameters:** 7B (7.07 billion)
- **Dimensions:** 3584
- **Context Length:** 32K tokens
- **Quantization:** Q4_K_M (4-bit with K-quants medium strategy)
- **Training:** 21M code-to-natural-language pairs

## Troubleshooting

### Model not found

```bash
# List installed models
ollama list

# Pull if missing
ollama pull manutic/nomic-embed-code:7b-Q4_K_M
```

### Wrong dimensions

If you get dimension mismatch errors after switching models:

```bash
# Re-embed with new model
codetect embed --force

# This will regenerate all embeddings with 3584 dimensions
```

### Ollama not responding

```bash
# Check if Ollama is running
curl http://localhost:11434/api/version

# Test model directly
curl http://localhost:11434/api/embed \
  -d '{"model":"manutic/nomic-embed-code:7b-Q4_K_M","input":"def hello(): pass"}'
```

## Performance Tips

1. **First run is slow:** Model loads into memory on first request (~30s)
2. **Keep Ollama running:** Model stays in memory for subsequent requests
3. **RAM requirements:** ~5-6 GB RAM for the quantized model
4. **GPU acceleration:** Ollama automatically uses Metal on Apple Silicon for faster inference

## Comparison with Other Models

### vs nomic-embed-text (current default)

| Aspect | nomic-embed-code | nomic-embed-text |
|--------|------------------|------------------|
| Code search accuracy | ⭐⭐⭐⭐⭐ (77.9 MRR) | ⭐⭐⭐ (~57 MRR) |
| Model size | 4.4 GB (quantized) | 274 MB |
| Dimensions | 3584 | 768 |
| Speed | Slower | Faster |
| Best for | Code projects | General docs/text |

### When to use each

**Use nomic-embed-code if:**
- Your project is primarily code
- You want the best code search accuracy
- You have sufficient RAM (~6GB)
- Search quality > speed

**Use nomic-embed-text if:**
- Mixed code + documentation
- Limited RAM (<4GB)
- Speed > accuracy
- Default choice works well enough

## Migration from nomic-embed-text

If you're switching from nomic-embed-text:

```bash
# 1. Pull new model
ollama pull manutic/nomic-embed-code:7b-Q4_K_M

# 2. Update config
export CODETECT_EMBEDDING_MODEL=manutic/nomic-embed-code:7b-Q4_K_M

# 3. Re-embed (required due to dimension change: 768 → 3584)
codetect embed --force

# 4. Verify
codetect doctor
```

## References

- [Nomic-Embed-Code Model Card](https://ollama.com/manutic/nomic-embed-code)
- [Ollama Embedding Models](https://ollama.com/blog/embedding-models)
- [Nomic AI Research](https://www.nomic.ai/)
