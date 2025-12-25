# DevScope: Project Description

## Overview

**DevScope** is a high-performance, from-scratch search engine designed specifically for source code and application logs. Built entirely in Go, the system implements a complete inverted index architecture with custom binary file formats, intelligent tokenization, and a TF-IDF based ranking algorithm—all without relying on external search libraries.

We designed DevScope to address a common developer pain point: quickly finding relevant code snippets, function definitions, and log entries across large codebases using natural search queries.

---

## System Architecture & How It Works

DevScope operates in two distinct phases: **Indexing** and **Searching**.

### Indexing Phase

When a user runs `devscope index <path>`, the system performs the following pipeline:

```
Raw Files → Crawler → Tokenizer → Index Builder → Binary Files (docs.bin, lexicon.bin, index.bin)
```

1. **Crawler (`crawler.go`)**: Recursively walks the target directory, identifying supported file types (`.go`, `.py`, `.js`, `.log`, etc.) while intelligently skipping irrelevant directories like `.git`, `node_modules`, and `vendor`. Each discovered file is assigned a unique Document ID and streamed via Go channels for concurrent processing.

2. **Tokenizer (`tokenizer.go`)**: Breaks file content into searchable tokens with context-aware processing:
   - **For Source Code**: Extracts identifiers using regex patterns, detecting function/class definitions and marking tokens with `MetaInFunctionName` flags for relevance boosting.
   - **For Log Files**: Parses ISO-8601 timestamps, extracts log levels (ERROR/WARN), and captures the time range for potential time-based filtering.

3. **Index Builder (`builder.go`)**: Constructs an in-memory inverted index (term → document → posting list) and flushes it to disk in our custom binary format. Additionally, we extract tokens from filenames themselves—so searching for "main" will boost `main.go` in the results.

### Search Phase

When a user runs `devscope search <query>`, the system executes:

```
User Query → Query Parser → Index Reader → Posting List Intersection → Scoring → Ranked Results
```

1. **Query Parser (`searcher.go`)**: Analyzes the query string, extracting:
   - Single search terms
   - Quoted phrases for exact sequence matching (e.g., `"fatal error"`)
   - Metadata filters (`ext:.go`, `level:ERROR`)

2. **Index Reader (`reader.go`)**: Loads the lexicon into memory and uses file offset seeking to efficiently retrieve posting lists from the index file—avoiding the need to load the entire index into RAM.

3. **Search Engine**: 
   - Performs posting list intersection for AND logic (all terms must match)
   - Validates positional adjacency for phrase queries using stored token positions
   - Applies metadata filters (file extension, log level)

4. **Scorer**: Calculates relevance using **TF-IDF** with metadata boosting:
   - **+5.0 bonus** for matches in filenames
   - **+3.0 bonus** for matches in function/class names
   - **2x multiplier** for exact phrase matches

---

## What We Built From Scratch

### 1. Custom Binary Index Format

We designed and implemented a proprietary binary file format optimized for minimal storage and fast retrieval:

| File | Purpose | Binary Structure |
|------|---------|------------------|
| `docs.bin` | Document metadata | `[DocID:4][Type:1][PathLen:2][Path:N][TimeMin:8][TimeMax:8]` |
| `lexicon.bin` | Term dictionary with offsets | `[TermLen:2][Term:N][DocFreq:4][Offset:8][PostingLen:4]` |
| `index.bin` | Inverted index (posting lists) | `[DocID:4][Freq:4][Meta:1][PosCount:4][Positions:4*N]...` |

Each file includes a magic header (`DEVSCOPE_DOCS`, `DEVSCOPE_LEX`, `DEVSCOPE_IDX`) and version byte for format validation.

### 2. Positional Inverted Index

Unlike simple term-to-document mappings, we store the **exact position** of every token occurrence. This enables:
- **Exact phrase matching**: `"database connection"` finds documents where these words appear consecutively
- **Proximity scoring potential**: Foundation for future "near" operator support

### 3. Context-Aware Tokenization

Our tokenizer isn't just splitting on whitespace—it understands code structure:
- Detects function/class definitions using regex patterns (`func`, `def`, `class`, `struct`)
- Marks tokens appearing in definitions with metadata flags
- Parses log timestamps to enable time-range filtering

### 4. Metadata-Boosted Ranking

We implemented a custom scoring algorithm that goes beyond basic TF-IDF:

```go
score := tf * idf
if isInFileName { score += 5.0 }
if isInFunctionName { score += 3.0 }
if isPhrase { score *= 2.0 }
```

### 5. Streaming Architecture

The indexer uses Go channels to stream documents from the crawler to the tokenizer, enabling memory-efficient processing of large codebases without loading everything into RAM.

---

## Core Logic Deep Dive

### Inverted Index Construction

```go
memIndex map[string]map[uint32]*models.Posting
```

For each term, we maintain a map of Document IDs to Posting objects. Each Posting stores:
- **Frequency**: How many times the term appears
- **Positions**: Array of token indices for phrase matching
- **Meta**: Bitflags for context (filename match, function name, log level)

### Phrase Matching Algorithm

Our phrase matching uses a **position-chain intersection** approach:

1. Retrieve posting lists for each word in the phrase
2. Start with positions from the first word as candidates
3. For each subsequent word, filter candidates where the new word's position = previous position + 1
4. Documents surviving all intersections contain the exact phrase

```go
for _, prevPos := range prevPositions {
    for _, currPos := range p.Positions {
        if prevPos+1 == currPos {
            validNewPositions = append(validNewPositions, currPos)
        }
    }
}
```

### Binary I/O Layer

We implemented custom binary serialization using `encoding/binary` with Little Endian byte order:

```go
binary.LittleEndian.PutUint32(buf, p.DocID)
binary.LittleEndian.PutUint64(buf, startOffset)
```

This gives us byte-level control over the index format, enabling efficient disk seeks during search.

---

## Unique Engineering Challenges Solved

### 1. Efficient Disk-Based Searching

**Challenge**: Loading the entire inverted index into memory doesn't scale for large codebases.

**Solution**: We store terms alphabetically in the lexicon with file offsets pointing into the index file. At search time, we:
1. Look up the term in the in-memory lexicon
2. Seek directly to the offset in `index.bin`
3. Read only the relevant posting list

This enables sub-millisecond lookups regardless of index size.

### 2. Accurate Phrase Matching Without Storing Raw Text

**Challenge**: Supporting exact phrase queries like `"fatal error"` without storing the original document text in the index.

**Solution**: We assign each token a monotonically increasing position counter within each document. During search, we verify that matched tokens have consecutive positions, proving they appeared adjacently in the source.

### 3. Handling Heterogeneous Document Types

**Challenge**: Source code and log files have fundamentally different structures—code has functions and identifiers; logs have timestamps and severity levels.

**Solution**: We implemented a **document type flag** (`DocType`) and **dual tokenization paths**:
- `tokenizeCode()`: Extracts identifiers, detects function definitions
- `tokenizeLog()`: Parses timestamps, extracts log levels, tracks time ranges

The unified posting format stores type-specific metadata in a single byte using bitflags.

### 4. Relevance Beyond Frequency

**Challenge**: A term appearing 100 times in a verbose log file shouldn't necessarily outrank a term appearing in a relevant filename.

**Solution**: Our ranking algorithm combines:
- **TF-IDF**: Standard information retrieval scoring
- **Metadata Boosts**: Structural importance (filename, function name)
- **Phrase Multipliers**: Exact matches are worth more than scattered terms

### 5. Memory-Efficient Streaming

**Challenge**: Indexing thousands of files without running out of memory.

**Solution**: We use Go's channel-based concurrency to stream documents one at a time from the crawler to the builder. The crawler runs in a goroutine, sending `DocumentRecord` objects through a channel, while the main loop processes and discards them after indexing.

---

## Performance Validation

We prototyped the logic in Python before building the production Go engine. Benchmark results on our test dataset:

| Implementation | Indexing Time |
|----------------|---------------|
| Python Prototype | ~100ms |
| DevScope (Go) | ~36ms |

**Result**: The Go implementation achieved **~3x speedup** thanks to static typing, compiled binaries, and low-level binary I/O.

## Technology Choices

- **Go**: Chosen for its performance characteristics, excellent concurrency primitives (channels, goroutines), and straightforward binary I/O
- **No External Dependencies**: The only import is the Go standard library—no Elasticsearch, no Lucene, no SQLite
- **Custom Binary Format**: Hand-crafted for our specific use case rather than using generic serialization formats like Protocol Buffers or MessagePack

---

## Conclusion

DevScope demonstrates that building a functional search engine from first principles is achievable with careful attention to data structures and algorithms. Through this project, we gained deep insights into:

- Inverted index architecture and posting list design
- Binary file format design and efficient serialization
- TF-IDF scoring and relevance ranking
- Phrase matching using positional indexing
- Streaming architectures for memory efficiency

The result is a fast, lightweight tool that developers can use to search their codebases without external dependencies or cloud services.
