package query

import (
	"bufio"
	"devscope/pkg/models"
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

// main search function that coordinates everything
func Search(idx *IndexReader, queryString string) ([]SearchResult, error) {
	terms, phrases, levelFilter, extFilter := parseQuery(queryString)

	if len(terms) == 0 && len(phrases) == 0 {
		return nil, nil
	}

	scores := make(map[uint32]float64)
	docMatches := make(map[uint32]int) // tracks how many terms/phrases matched per doc
	totalRequirements := len(terms) + len(phrases)

	// 1. Process Single Terms
	for _, term := range terms {
		postings, err := idx.GetPostings(term)
		if err != nil {
			return nil, err
		}
		if postings == nil {
			continue
		}

		lexEntry := idx.Lexicon[term]
		idf := math.Log(float64(idx.TotalDocs) / (float64(lexEntry.DocFreq) + 1))

		processPostings(postings, idx.Docs, idf, scores, docMatches, levelFilter, extFilter)
	}

	// 2. Process Phrases
	for _, phrase := range phrases {
		// a phrase is a list of words that must appear in order
		// first, get postings for all words
		var phrasePostings [][]models.Posting
		for _, word := range phrase {
			p, err := idx.GetPostings(word)
			if err != nil {
				return nil, err
			}
			if p == nil {
				phrasePostings = nil
				break // one word missing means phrase missing
			}
			phrasePostings = append(phrasePostings, p)
		}

		if phrasePostings == nil {
			continue
		}

		// intersection logic
		matchedDocs := matchPhraseDocs(phrasePostings)

		// calculating idf for phrase is complex, lets just sum idfs of words
		var phraseIdf float64
		for _, word := range phrase {
			lexEntry := idx.Lexicon[word]
			phraseIdf += math.Log(float64(idx.TotalDocs) / (float64(lexEntry.DocFreq) + 1))
		}

		// add scores for matched docs
		for docID := range matchedDocs {
			doc := idx.Docs[docID]
			// check filters again (redundant but safe)
			if extFilter != "" && !strings.HasSuffix(strings.ToLower(doc.Path), extFilter) {
				continue
			}
			// level filter mainly for logs, phrase search usually for code but lets support it
			// we assume if phrase exists in doc, it passes unless specific line check needed (skipped for now)

			// tf = 1 for phrase (simplification)
			score := 1.0 * phraseIdf * 2.0 // bonus for phrase

			scores[docID] += score
			docMatches[docID]++
		}
	}

	var results []SearchResult
	// enforce AND logic
	for docID, count := range docMatches {
		if count == totalRequirements {
			doc := idx.Docs[docID]
			res := SearchResult{
				DocID: docID,
				Path:  doc.Path,
				Score: scores[docID],
			}
			results = append(results, res)
		}
	}

	// sort by highest score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > 10 {
		results = results[:10]
	}

	// generate snippets
	displayTerm := ""
	if len(terms) > 0 {
		displayTerm = terms[0]
	} else if len(phrases) > 0 {
		displayTerm = phrases[0][0]
	}

	for i := range results {
		results[i].Snippet, results[i].LineNum = getSnippet(results[i].Path, displayTerm)
	}

	return results, nil
}

func processPostings(postings []models.Posting, docs map[uint32]models.DocumentRecord, idf float64, scores map[uint32]float64, docMatches map[uint32]int, levelFilter, extFilter string) {
	for _, p := range postings {
		doc := docs[p.DocID]

		if extFilter != "" && !strings.HasSuffix(strings.ToLower(doc.Path), extFilter) {
			continue
		}

		if levelFilter == "ERROR" {
			if (p.Meta & (1 << 2)) == 0 {
				continue
			}
		} else if levelFilter == "WARN" {
			if (p.Meta & (1 << 3)) == 0 {
				continue
			}
		}

		tf := float64(p.Frequency)
		score := tf * idf

		if (p.Meta & (1 << 0)) != 0 {
			score += 5.0
		}
		if (p.Meta & (1 << 1)) != 0 {
			score += 3.0
		}

		scores[p.DocID] += score
		docMatches[p.DocID]++
	}
}

func matchPhraseDocs(postingsList [][]models.Posting) map[uint32]bool {
	// Start with docIDs from the first word
	// Using map for quick lookup
	candidates := make(map[uint32][]uint32) // docID -> positions of first word matches

	firstList := postingsList[0]
	for _, p := range firstList {
		candidates[p.DocID] = p.Positions
	}

	// Intersect with subsequent words
	for i := 1; i < len(postingsList); i++ {
		nextCandidates := make(map[uint32][]uint32)
		currentWordPostings := postingsList[i]

		for _, p := range currentWordPostings {
			// check if this doc was in previous candidates
			prevPositions, ok := candidates[p.DocID]
			if !ok {
				continue
			}

			// check positional adjacency
			// for any pos in prevPositions, is there a (pos + 1) in p.Positions?
			var validNewPositions []uint32
			for _, prevPos := range prevPositions {
				for _, currPos := range p.Positions {
					// we now use token indices, so strict adjacency is +1
					if prevPos+1 == currPos {
						validNewPositions = append(validNewPositions, currPos)
					}
				}
			}

			if len(validNewPositions) > 0 {
				nextCandidates[p.DocID] = validNewPositions
			}
		}
		candidates = nextCandidates
		if len(candidates) == 0 {
			break
		}
	}

	finalDocs := make(map[uint32]bool)
	for id := range candidates {
		finalDocs[id] = true
	}
	return finalDocs
}

func parseQuery(q string) (terms []string, phrases [][]string, level, ext string) {
	// manual parsing loop
	var buffer strings.Builder
	inQuote := false

	flush := func() {
		if buffer.Len() > 0 {
			s := buffer.String()
			buffer.Reset()

			if strings.HasPrefix(s, "level:") {
				level = strings.ToUpper(strings.TrimPrefix(s, "level:"))
			} else if strings.HasPrefix(s, "ext:") {
				ext = strings.ToLower(strings.TrimPrefix(s, "ext:"))
			} else {
				terms = append(terms, strings.ToLower(s))
			}
		}
	}

	chars := []rune(q)
	for i := 0; i < len(chars); i++ {
		c := chars[i]
		if c == '"' {
			if inQuote {
				// End of phrase
				phraseStr := buffer.String()
				buffer.Reset()
				// split phrase into words
				parts := strings.Fields(strings.ToLower(phraseStr))
				if len(parts) > 0 {
					phrases = append(phrases, parts)
				}
				inQuote = false
			} else {
				// Start of phrase, flush previous word if any
				flush()
				inQuote = true
			}
		} else if c == ' ' && !inQuote {
			flush()
		} else {
			buffer.WriteRune(c)
		}
	}
	flush()
	return
}

// finds the line with the term to show context
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
			if len(line) > 200 {
				line = line[:200] + "..."
			}
			return strings.TrimSpace(line), lineNum
		}
		lineNum++
	}
	return "", 0
}
