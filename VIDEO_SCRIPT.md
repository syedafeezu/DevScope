# DevScope: Competition Video Script 🎬

**Target Duration**: 5 minutes
**Goal**: Maximize scores on Technical Depth, Originality, Clarity, and Working Demo.

---

## SECTION 1: Hook & Introduction (0:00 - 0:30)

### What to Say:
> "Hi, I'm [Your Name]. For this hackathon, I built **DevScope**—a high-performance search engine for source code and logs.
>
> But here's the thing: I didn't use Lucene, Elasticsearch, or SQLite. I wrote everything from scratch—custom binary file formats, my own tokenizer, and a ranking algorithm. Let me show you how it works."

### What to Show:
- Your face (optional, builds trust)
- Then switch to the terminal with VS Code open

---

## SECTION 2: Live Demo (0:30 - 1:30)

### Step 1: Indexing
**Command**:
```powershell
./devscope index .
```

**What to Say**:
> "First, I index my entire project directory. DevScope crawls every file, tokenizes the content, and writes a custom binary index to `.devscope/`.
>
> Notice the speed: **22 files in 20 milliseconds**. That's the power of Go's low-level I/O."

---

### Step 2: Basic Search
**Command**:
```powershell
./devscope search "main"
```

**What to Say**:
> "Now let's search. Notice the ranking: `main.go` appears first even though `PRESENTATION_GUIDE.md` mentions 'main' more times.
>
> Why? Because my ranking algorithm gives a **+5 bonus** if the search term appears in the **filename**. Developers usually want definitions, not documentation."

---

### Step 3: Phrase Search (THE KILLER FEATURE)
**Command**:
```powershell
./devscope search '"hello world"'
```

**What to Say**:
> "This is my most complex feature: **Phrase Search**. When I wrap terms in quotes, the engine doesn't just find files with both words—it verifies they appear **right next to each other**.
>
> `phrase_match.txt` contains 'hello world' in sequence, so it matches. `phrase_fail.txt` has 'hello big world'—the words exist, but they're not adjacent, so it's correctly excluded."

---

## SECTION 3: Technical Deep Dive (1:30 - 3:30)

### Part A: The Binary Index Format

**What to Show**: Open `.devscope/index.bin` in a hex editor (or just show the diagram in README).

**What to Say**:
> "I designed my own binary format. No JSON, no SQLite overhead. Every posting is packed as raw bytes:
> - 4 bytes for DocID
> - 4 bytes for Frequency
> - 1 byte for Metadata flags
> - N positions as 4-byte integers
>
> This compact structure lets me seek directly to any term's data without reading the whole file."

---

### Part B: The Tokenizer (Show Code)

**File**: `internal/indexer/tokenizer.go`
**Lines**: 41-71

**What to Show**: Scroll to `tokenizeCode` function.

**What to Say**:
> "My tokenizer is context-aware. Look at line 49: I use a regex to detect function definitions like `func main` or `def process`.
>
> If a word matches the function name, I set a **metadata flag** (line 60). Later, my ranking algorithm uses this flag to boost the score by +3 points.
>
> This is how DevScope understands that a **definition** is more important than a **reference**."

---

### Part C: Phrase Search Algorithm (Show Code)

**File**: `internal/query/searcher.go`
**Lines**: 168-216

**What to Show**: Scroll to `matchPhraseDocs` function.

**What to Say**:
> "This is the brain of phrase search. Here's the algorithm:
>
> 1. I start with all positions of the first word (line 172-175).
> 2. For each subsequent word, I check: does any position equal the previous position **plus one**? (line 194)
> 3. If yes, it's a valid chain. If no chains survive, the phrase doesn't exist in that document.
>
> This is called **Positional Intersection**, and it's how real search engines like Google handle quote queries."

---

### Part D: TF-IDF Scoring (Show Code)

**File**: `internal/query/searcher.go`
**Lines**: 44-47, 152-160

**What to Say**:
> "My ranking uses the classic TF-IDF formula. Line 45 calculates IDF: the logarithm of total documents divided by documents containing the term.
>
> But I go further. Lines 155-160 add **contextual bonuses**:
> - +5 if the term is in the filename
> - +3 if it's inside a function definition
>
> This hybrid approach combines math with code-specific heuristics."

---

## SECTION 4: Performance & Benchmarks (3:30 - 4:00)

**What to Show**: Terminal with both commands

**Commands**:
```powershell
python pythonproto.py index test_data   # ~1.2 seconds
./devscope index test_data              # ~0.02 seconds
```

**What to Say**:
> "To validate my design, I first prototyped in Python. It worked, but it took **1.2 seconds** to index just 10 files.
>
> The Go version does the same in **20 milliseconds**—that's **50x faster**. This is why I chose Go: static typing, zero garbage collection pressure, and direct binary I/O."

---

## SECTION 5: Closing & Future Work (4:00 - 5:00)

**What to Say**:
> "To summarize: DevScope is a **from-scratch search engine** with:
> - Custom binary index files
> - Context-aware tokenization
> - TF-IDF ranking with metadata bonuses
> - Exact phrase matching using positional intersection
>
> If I had more time, I'd add **delta compression** to shrink the index and **concurrent crawling** for even faster indexing.
>
> Thank you for watching. I hope this demonstrates that you can build production-grade infrastructure without relying on frameworks."

---

## PRO TIPS FOR RECORDING

1. **Use Zoom on Code**: When showing code, zoom in so judges can read it clearly.
2. **Pause After Commands**: Let the output appear before speaking about it.
3. **Confidence**: You built this. Own it.
4. **Time Check**: Practice to hit exactly 5 minutes. Judges respect precision.

---

## QUICK REFERENCE: Key Lines to Show

| Feature | File | Lines |
|---------|------|-------|
| Tokenizer (func detection) | `tokenizer.go` | 49-60 |
| Phrase Intersection | `searcher.go` | 168-216 |
| TF-IDF Calculation | `searcher.go` | 44-47 |
| Metadata Bonuses | `searcher.go` | 155-160 |
| Binary Write | `builder.go` | 130-170 |
| Filename Indexing | `builder.go` | 78-90 |

**Good luck. Go win this. 🏆**
