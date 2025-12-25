package indexer

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"devscope/internal/store"
	"devscope/pkg/models"
)

var reFileNameToken = regexp.MustCompile(`[a-zA-Z0-9_]+`)

type IndexBuilder struct {
	DocsPath    string
	IndexPath   string
	LexiconPath string

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

// this does everything: crawl, tokenize, save
func (b *IndexBuilder) Build(root string) error {
	start := time.Now()

	docWriter, err := store.NewDocWriter(b.DocsPath)
	if err != nil {
		return fmt.Errorf("failed to open docs file: %w", err)
	}
	defer docWriter.Close()

	crawler := NewCrawler(root)
	docsChan := make(chan models.DocumentRecord)

	go crawler.Crawl(docsChan)

	count := 0
	for doc := range docsChan {
		count++

		file, err := os.Open(doc.Path)
		if err != nil {
			fmt.Printf("Warn: could not open %s: %v\n", doc.Path, err)
			continue
		}

		tokens, minT, maxT := Tokenize(file, doc.Type)
		file.Close()

		doc.TimestampMin = minT
		doc.TimestampMax = maxT

		if err := docWriter.Write(doc); err != nil {
			return fmt.Errorf("failed to write doc: %w", err)
		}

		// put tokens in memory map for now
		for _, tok := range tokens {
			b.addToken(tok, doc.DocID)
		}

		// ALSO we index the filename itself for the +5.0 bonus!
		baseName := filepath.Base(doc.Path)
		// remove extension for cleaner tokens? "main.cpp" -> "main", "cpp"
		// simple regex find all works nicely
		fnTokens := reFileNameToken.FindAllString(baseName, -1)
		for _, term := range fnTokens {
			// add with MetaInFileName, position 0 (header)
			b.addToken(RawToken{
				Term:     strings.ToLower(term),
				Position: 0,
				Meta:     MetaInFileName, // Same package constant
			}, doc.DocID)
		}

		if count%100 == 0 {
			fmt.Printf("\rIndexed %d files...", count)
		}
	}
	fmt.Printf("\nFinished core indexing of %d files in %v. Sorting and writing index...\n", count, time.Since(start))

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

// save everything to disk in binary format
func (b *IndexBuilder) save() error {
	idxFile, err := os.Create(b.IndexPath)
	if err != nil {
		return err
	}
	defer idxFile.Close()

	idxFile.WriteString("DEVSCOPE_IDX")
	idxFile.Write([]byte{1})

	lexFile, err := os.Create(b.LexiconPath)
	if err != nil {
		return err
	}
	defer lexFile.Close()

	lexFile.WriteString("DEVSCOPE_LEX")
	lexFile.Write([]byte{1})

	// sort terms so we can search faster later maybe?
	terms := make([]string, 0, len(b.memIndex))
	for t := range b.memIndex {
		terms = append(terms, t)
	}
	sort.Strings(terms)

	var indexOffset uint64 = 13 // start after idx header
	buf := make([]byte, 8)

	for _, term := range terms {
		docMap := b.memIndex[term]

		// sort by docID for delta encoding later if we want
		postings := make([]*models.Posting, 0, len(docMap))
		for _, p := range docMap {
			postings = append(postings, p)
		}
		sort.Slice(postings, func(i, j int) bool {
			return postings[i].DocID < postings[j].DocID
		})

		startOffset := indexOffset

		// write each posting
		for _, p := range postings {
			binary.LittleEndian.PutUint32(buf, p.DocID)
			idxFile.Write(buf[:4])

			binary.LittleEndian.PutUint32(buf, p.Frequency)
			idxFile.Write(buf[:4])

			idxFile.Write([]byte{p.Meta})

			binary.LittleEndian.PutUint32(buf, uint32(len(p.Positions)))
			idxFile.Write(buf[:4])

			for _, pos := range p.Positions {
				binary.LittleEndian.PutUint32(buf, pos)
				idxFile.Write(buf[:4])
			}

			indexOffset += uint64(13 + 4*len(p.Positions))
		}

		postingListLen := indexOffset - startOffset

		// write lexicon entry
		termBytes := []byte(term)
		if len(termBytes) > 65535 {
			termBytes = termBytes[:65535]
		}

		binary.LittleEndian.PutUint16(buf, uint16(len(termBytes)))
		lexFile.Write(buf[:2])

		lexFile.Write(termBytes)

		binary.LittleEndian.PutUint32(buf, uint32(len(postings)))
		lexFile.Write(buf[:4])

		binary.LittleEndian.PutUint64(buf, startOffset)
		lexFile.Write(buf[:8])

		binary.LittleEndian.PutUint32(buf, uint32(postingListLen))
		lexFile.Write(buf[:4])
	}

	return nil
}
