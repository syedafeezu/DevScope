package query

import (
	"bufio"
	"math"
	"os"
	"sort"
	"strings"
)

type SearchResult struct {
	DocID   uint32
	Path    string
	Score   float64
	Snippet string
	LineNum uint32
}

// Search executes a query against the index
func Search(idx *IndexReader, queryString string) ([]SearchResult, error) {
	// 1. Simple Parsing
	// Split by space. Supports "term", "level:ERROR", "ext:.py"
	// Implicit AND.

	parts := strings.Fields(queryString)
	var terms []string
	var levelFilter string // "ERROR", "WARN" or empty
	var extFilter string

	for _, p := range parts {
		if strings.HasPrefix(p, "level:") {
			levelFilter = strings.ToUpper(strings.TrimPrefix(p, "level:"))
		} else if strings.HasPrefix(p, "ext:") {
			extFilter = strings.ToLower(strings.TrimPrefix(p, "ext:"))
		} else {
			terms = append(terms, strings.ToLower(p))
		}
	}

	if len(terms) == 0 {
		return nil, nil
	}

	// 2. Retrieve Postings for each term
	// map[DocID] -> Score
	scores := make(map[uint32]float64)
	docMatches := make(map[uint32]int) // Count of terms matched

	for _, term := range terms {
		postings, err := idx.GetPostings(term)
		if err != nil {
			return nil, err
		}
		if postings == nil {
			continue // Term not found
		}

		// Lexicon contains DocFreq for IDF
		lexEntry := idx.Lexicon[term]
		idf := math.Log(float64(idx.TotalDocs) / (float64(lexEntry.DocFreq) + 1))

		for _, p := range postings {
			doc := idx.Docs[p.DocID]

			if extFilter != "" && !strings.HasSuffix(strings.ToLower(doc.Path), extFilter) {
				continue
			}

			// Level filter logic
			// Req: "error AND timeout level:ERROR".
			// Check if p.Meta has MetaLogLevelError bit OR if doc itself is a log with ERROR lines?
			// For specific file filtering, usually we check if the file matches criteria.
			// But for logs, "level:ERROR" implies we are looking for error lines.
			// p.Meta bits: 0x4=Error, 0x8=Warn (defined in tokenizer.go but copied to models check?)
			// models.Posting Meta: 1=FileName, 2=FuncName, 4=Error, 8=Warn.

			if levelFilter == "ERROR" {
				if (p.Meta & (1 << 2)) == 0 { // 1<<2 is MetaLogLevelError
					continue
				}
			} else if levelFilter == "WARN" {
				if (p.Meta & (1 << 3)) == 0 {
					continue
				}
			}

			// TF-IDF
			tf := float64(p.Frequency)
			score := tf * idf

			// Bonus
			if (p.Meta & (1 << 0)) != 0 {
				score += 5.0
			} // In FileName
			if (p.Meta & (1 << 1)) != 0 {
				score += 3.0
			} // In FuncName

			scores[p.DocID] += score
			docMatches[p.DocID]++
		}
	}

	// 3. Filter for AND logic (must contain all terms)
	var results []SearchResult
	for docID, count := range docMatches {
		if count == len(terms) {
			doc := idx.Docs[docID]
			res := SearchResult{
				DocID: docID,
				Path:  doc.Path,
				Score: scores[docID],
			}
			results = append(results, res)
		}
	}

	// 4. Sort by Score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 5. Generate Snippets for top N
	if len(results) > 10 {
		results = results[:10]
	}

	for i := range results {
		results[i].Snippet, results[i].LineNum = getSnippet(results[i].Path, terms[0])
	}

	return results, nil
}

func getSnippet(path string, term string) (string, uint32) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := uint32(1)
	termLower := strings.ToLower(term)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(strings.ToLower(line), termLower) {
			// Trim if too long
			if len(line) > 200 {
				line = line[:200] + "..."
			}
			return strings.TrimSpace(line), lineNum
		}
		lineNum++
	}
	return "", 0
}
