# Plan: Cursor Feature Gap Analysis

**Date:** 2026-02-02
**Objective:** Evaluate codetect's functionality against Cursor's indexing capabilities and identify strategic feature gaps for MCP-enabled LLMs

---

## Objective

Analyze codetect's current feature set compared to Cursor's indexing and code intelligence capabilities to:

1. **Identify missing features** that would make codetect competitive with Cursor
2. **Prioritize features** that align with our goal of bringing indexing to any MCP-capable LLM
3. **Define an open-source core** + premium hosted tier strategy
4. **Create a strategic roadmap** for closing the most valuable gaps

**Success Criteria:**
- Comprehensive feature comparison matrix (Cursor vs codetect)
- Prioritized list of features by strategic value
- Clear recommendation on which gaps to address first
- Open-source vs premium tier feature allocation

---

## Approach

### Phase 1: Feature Comparison (Research)

**1.1 Catalog Cursor's Capabilities**
- [x] Research Cursor's indexing architecture (Merkle trees, Turbopuffer, etc.)
- [x] Document semantic search implementation (custom embeddings, AST chunking)
- [x] Identify code intelligence features (@codebase, hybrid search, etc.)
- [x] Note privacy/security features (encryption, obfuscation)

**1.2 Catalog codetect's Current Capabilities**
- [x] Document existing MCP tools (search_keyword, find_symbol, etc.)
- [x] Review architecture (SQLite/PostgreSQL, AST chunking, RRF fusion)
- [x] Note performance optimizations (parallel embedding, HNSW, caching)
- [x] Identify current limitations

**1.3 Create Comparison Matrix**
- [ ] Map features side-by-side
- [ ] Identify gaps (what Cursor has that codetect lacks)
- [ ] Identify advantages (what codetect has that Cursor lacks)
- [ ] Note architectural differences

### Phase 2: Gap Analysis (Analysis)

**2.1 Categorize Missing Features**
Group gaps into categories:
- **Core Indexing Infrastructure** (Merkle trees, change detection, etc.)
- **Search Quality** (custom embeddings, reranking, query expansion)
- **Code Intelligence** (LSP integration, call graphs, type hierarchies)
- **User Experience** (automatic re-indexing, progress indicators, etc.)
- **Privacy/Security** (encryption, obfuscation, cloud sync)
- **Integration** (HTTP API, non-MCP tool support, etc.)

**2.2 Assess Strategic Value**
For each missing feature, evaluate:
- **MCP Alignment**: Does this benefit any MCP-capable LLM?
- **Open Source Viability**: Can this be built with OSS tools?
- **Competitive Differentiation**: Does this close a critical gap vs Cursor?
- **Implementation Complexity**: How hard is this to build?
- **User Impact**: How much does this improve the user experience?

**2.3 Define Tier Strategy**
Allocate features to:
- **Open Source Core**: Features that maximize MCP ecosystem value
- **Premium Hosted Tier**: Features that require infrastructure/hosting
- **Enterprise Tier** (future): Features for organizations at scale

### Phase 3: Prioritization (Decision)

**3.1 Score Features**
Use weighted scoring:
- MCP Alignment: 30%
- User Impact: 25%
- Open Source Viability: 20%
- Competitive Differentiation: 15%
- Implementation Complexity: 10% (inverse)

**3.2 Create Roadmap**
Group features into:
- **Quick Wins** (high impact, low complexity)
- **Strategic Investments** (high impact, high complexity)
- **Nice-to-Haves** (medium impact, any complexity)
- **Premium Only** (requires hosting/infrastructure)

**3.3 Recommendations**
Provide clear recommendations on:
- Top 3-5 features to implement first
- Which features belong in open source vs premium
- Resource allocation guidance (time/effort estimates)
- Risk assessment for each major feature

---

## Data Sources

### Primary Sources
- **Cursor Documentation**
  - https://cursor.com/docs/context/codebase-indexing
  - https://cursor.com/docs/context/semantic-search
  - https://cursor.com/blog/semsearch

- **codetect Documentation**
  - README.md
  - docs/architecture.md
  - docs/benchmarks.md
  - docs/evaluation.md

### Secondary Sources
- Cursor technical deep-dives (Engineer's Codex, Pragmatic Engineer)
- MCP ecosystem analysis (which tools support MCP?)
- Competitor analysis (GitHub Copilot, Sourcegraph, etc.)
- Open-source alternatives (ChromaDB, Qdrant, etc.)

---

## Risks

### Technical Risks
1. **Custom Embedding Models** - Cursor's competitive advantage comes from training custom models on agent behavior. This requires:
   - Large dataset of LLM coding sessions
   - GPU infrastructure for training
   - Ongoing model iteration
   - **Mitigation:** Focus on better fusion/reranking instead of custom embeddings initially

2. **Merkle Tree Change Detection** - Sub-second indexing requires sophisticated change detection:
   - Complex implementation (hash tree management)
   - Requires persistent state tracking
   - **Mitigation:** Implement incremental indexing first, optimize later

3. **Turbopuffer or Similar Vector DB** - Cursor uses a specialized vector database:
   - May need to build or adopt a high-performance vector store
   - PostgreSQL + pgvector may not scale to Cursor's level
   - **Mitigation:** Benchmark current solution, identify scaling limits

### Strategic Risks
1. **Feature Parity vs Innovation** - Trying to match Cursor feature-for-feature may miss opportunities for differentiation
   - **Mitigation:** Focus on MCP-first features that enable any LLM, not just Claude

2. **Open Source vs Revenue** - Giving away too much in OSS may hurt premium tier adoption
   - **Mitigation:** Clear tier boundaries (local = OSS, hosted = premium)

3. **Scope Creep** - Too many features dilute focus and delay shipping
   - **Mitigation:** Strict prioritization, ship iteratively

### Market Risks
1. **Cursor Moat** - Cursor may have advantages (data, distribution, brand) that features alone can't overcome
   - **Mitigation:** Compete on openness (MCP), not closed ecosystems

2. **MCP Adoption** - If MCP doesn't gain traction, our strategy may fail
   - **Mitigation:** Hedge with HTTP API for non-MCP tools

---

## MCP Tools & Preprocessing

This analysis will use:

### MCP Tools (codetect)
- **search_keyword** - Verify current keyword search capabilities
- **find_symbol** - Test symbol navigation quality
- **search_semantic** - Benchmark semantic search accuracy
- **hybrid_search** - Evaluate RRF fusion effectiveness

### External Research
- Web search for Cursor technical details
- Document analysis of Cursor blog posts
- Competitor feature comparison (GitHub Copilot, Sourcegraph)

### No Custom MCP Wrappers Needed
This is primarily a research and analysis task. All required data can be gathered through:
- Reading documentation
- Web research
- Manual feature comparison

---

## Deliverables

### 1. Feature Comparison Matrix
**File:** `context/data/2026-02-02-cursor-vs-codetect-matrix.md`

Table format:
| Feature Category | Cursor | codetect | Gap Priority | Notes |
|------------------|--------|----------|--------------|-------|
| ... | ... | ... | ... | ... |

### 2. Gap Analysis Report
**File:** `context/summaries/2026-02-02-cursor-feature-gaps.md`

Sections:
- Executive Summary
- Critical Gaps (must-have features)
- Strategic Gaps (high-value differentiation)
- Nice-to-Have Gaps (incremental improvements)
- Advantages (what codetect does better)

### 3. Prioritized Roadmap
**File:** `context/summaries/2026-02-02-feature-roadmap.md`

Sections:
- Quick Wins (0-3 months)
- Strategic Investments (3-6 months)
- Long-term Vision (6-12 months)
- Premium Tier Features

### 4. Tier Strategy Document
**File:** `context/summaries/2026-02-02-tier-strategy.md`

Sections:
- Open Source Core (feature list + rationale)
- Premium Hosted Tier (feature list + rationale)
- Pricing considerations
- Competitive positioning

---

## Review Checklist

Before proceeding with implementation:
- [ ] Feature comparison matrix is comprehensive
- [ ] All major Cursor features are accounted for
- [ ] Gap priorities are clearly justified
- [ ] Roadmap is realistic given resources
- [ ] Tier strategy balances OSS value with revenue potential
- [ ] Recommendations are actionable and specific

---

## Timeline

**Research Phase:** 1-2 hours
- Comprehensive documentation review
- Feature comparison matrix creation

**Analysis Phase:** 2-3 hours
- Gap analysis and categorization
- Strategic scoring and prioritization

**Deliverables:** 1-2 hours
- Write summary reports
- Create roadmap documents
- Finalize tier strategy

**Total Estimated Time:** 4-7 hours

---

## Success Metrics

This plan is successful if:
1. ✅ We have a clear understanding of Cursor's competitive advantages
2. ✅ We can articulate which features matter most for MCP-enabled LLMs
3. ✅ We have a prioritized roadmap with clear next steps
4. ✅ We can confidently decide which features belong in OSS vs premium
5. ✅ The team agrees on the strategic direction

---

## Notes

**Why This Matters:**
- Cursor is the market leader in AI-native code editors
- Understanding their indexing advantage helps us compete effectively
- MCP creates an opportunity to bring these capabilities to any LLM
- Open-source approach can outflank Cursor's closed ecosystem

**Key Questions to Answer:**
1. What does Cursor do that we absolutely cannot skip?
2. What can we do better by being MCP-first and open-source?
3. Which features require hosting and justify a premium tier?
4. How do we balance feature development with time-to-market?

**Philosophical Stance:**
- **Don't copy Cursor** - Build for the MCP ecosystem, not just Claude
- **Embrace open source** - Maximum value in the open, premium for convenience/scale
- **Ship iteratively** - Don't wait for feature parity, ship value incrementally
- **Differentiate on openness** - Any LLM, any tool, any hosting setup

---

**End of Plan**
