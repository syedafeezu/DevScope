package indexer

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"time"

	"devscope/internal/store"
	"devscope/pkg/models"
)

// IndexBuilder orchestrates the indexing process
type IndexBuilder struct {
	DocsPath    string
	IndexPath   string
	LexiconPath string

	// term -> docID -> Pointer to Posting
	memIndex map[string]map[uint32]*models.Posting
}

func NewIndexBuilder(outDir string) *IndexBuilder {
	return &IndexBuilder{
		DocsPath:    outDir + "/" + models.DocsFileName,
		IndexPath:   outDir + "/" + models.IndexFileName,
		LexiconPath: outDir + "/" + models.LexiconFileName,
		memIndex:    make(map[string]map[uint32]*models.Posting),
	}
}

// Build runs the full indexing pipeline
func (b *IndexBuilder) Build(root string) error {
	start := time.Now()

	// 1. Setup Docs Writer
	docWriter, err := store.NewDocWriter(b.DocsPath)
	if err != nil {
		return fmt.Errorf("failed to open docs file: %w", err)
	}
	defer docWriter.Close()

	// 2. Crawl
	crawler := NewCrawler(root)
	docsChan := make(chan models.DocumentRecord)

	go crawler.Crawl(docsChan)

	count := 0
	for doc := range docsChan {
		count++

		// 3. Tokenize
		file, err := os.Open(doc.Path)
		if err != nil {
			fmt.Printf("Warn: could not open %s: %v\n", doc.Path, err)
			continue
		}

		tokens, minT, maxT := Tokenize(file, doc.Type)
		file.Close()

		// Update doc metadata with timestamps if needed
		doc.TimestampMin = minT
		doc.TimestampMax = maxT

		// Write to docs.bin
		if err := docWriter.Write(doc); err != nil {
			return fmt.Errorf("failed to write doc: %w", err)
		}

		// 4. Update In-Memory Index
		for _, tok := range tokens {
			b.addToken(tok, doc.DocID)
		}

		if count%100 == 0 {
			fmt.Printf("\rIndexed %d files...", count)
		}
	}
	fmt.Printf("\nFinished core indexing of %d files in %v. Sorting and writing index...\n", count, time.Since(start))

	// 5. Write Index & Lexicon
	return b.save()
}

func (b *IndexBuilder) addToken(tok RawToken, docID uint32) {
	docMap, ok := b.memIndex[tok.Term]
	if !ok {
		docMap = make(map[uint32]*models.Posting)
		b.memIndex[tok.Term] = docMap
	}

	post, ok := docMap[docID]
	if !ok {
		post = &models.Posting{
			DocID: docID,
			Meta:  0,
		}
		docMap[docID] = post
	}

	post.Frequency++
	post.Positions = append(post.Positions, tok.Position)
	post.Meta |= tok.Meta
}

func (b *IndexBuilder) save() error {
	idxFile, err := os.Create(b.IndexPath)
	if err != nil {
		return err
	}
	defer idxFile.Close()

	// Prepare Header (DEVSCOPE_IDX / LEX) - Keeping it simple for now or matching plan?
	// Plan said Header: DEVSCOPE_IDX (8 bytes) + Version (1 byte)
	// I'll add headers here to match plan strictly.
	idxFile.WriteString("DEVSCOPE_IDX")
	idxFile.Write([]byte{1})

	lexFile, err := os.Create(b.LexiconPath)
	if err != nil {
		return err
	}
	defer lexFile.Close()

	lexFile.WriteString("DEVSCOPE_LEX")
	lexFile.Write([]byte{1})

	// Sort terms
	terms := make([]string, 0, len(b.memIndex))
	for t := range b.memIndex {
		terms = append(terms, t)
	}
	sort.Strings(terms)

	var indexOffset uint64 = 0 // Relative to postings start (after header)?
	// Plan said: "Offset -> Points to start of postings in index.bin"
	// So it should include header size.
	// Header size = 12 ("DEVSCOPE_IDX") + 1 (Version) = 13 bytes.
	indexOffset = 13

	// Buffer for writing integers
	buf := make([]byte, 8)

	for _, term := range terms {
		docMap := b.memIndex[term]

		// Sort postings by DocID
		postings := make([]*models.Posting, 0, len(docMap))
		for _, p := range docMap {
			postings = append(postings, p)
		}
		sort.Slice(postings, func(i, j int) bool {
			return postings[i].DocID < postings[j].DocID
		})

		startOffset := indexOffset

		// Write postings to index.bin
		for _, p := range postings {
			// Write DocID (4)
			binary.LittleEndian.PutUint32(buf, p.DocID)
			idxFile.Write(buf[:4])

			// Write Freq (4)
			binary.LittleEndian.PutUint32(buf, p.Frequency)
			idxFile.Write(buf[:4])

			// Write Meta (1)
			idxFile.Write([]byte{p.Meta})

			// Write Positions Count (4)
			binary.LittleEndian.PutUint32(buf, uint32(len(p.Positions)))
			idxFile.Write(buf[:4])

			// Write Positions (4 * Count)
			for _, pos := range p.Positions {
				binary.LittleEndian.PutUint32(buf, pos)
				idxFile.Write(buf[:4])
			}

			// Update offset: 4+4+1+4 + 4*len
			indexOffset += uint64(13 + 4*len(p.Positions))
		}

		// Calculate chunk length
		postingListLen := indexOffset - startOffset

		// Write Lexicon Entry
		// Record: TermLen (2) + Term + DocFreq (4) + Offset (8) + Length (4)
		// Updated to match implementation plan.
		// NOTE: implementation plan said TermLen is uint16. My prev code had uint8. I'll use uint16.

		termBytes := []byte(term)
		if len(termBytes) > 65535 {
			termBytes = termBytes[:65535]
		}

		binary.LittleEndian.PutUint16(buf, uint16(len(termBytes)))
		lexFile.Write(buf[:2])

		lexFile.Write(termBytes)

		binary.LittleEndian.PutUint32(buf, uint32(len(postings))) // DocFreq
		lexFile.Write(buf[:4])

		binary.LittleEndian.PutUint64(buf, startOffset)
		lexFile.Write(buf[:8])

		binary.LittleEndian.PutUint32(buf, uint32(postingListLen)) // Length of posting list logic
		lexFile.Write(buf[:4])
	}

	return nil
}
