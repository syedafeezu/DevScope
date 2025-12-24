package query

import (
	"bufio"
	"devscope/pkg/models"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type IndexReader struct {
	Docs      map[uint32]models.DocumentRecord
	Lexicon   map[string]models.LexiconEntry
	File      *os.File
	TotalDocs int
}

func NewIndexReader(dir string) (*IndexReader, error) {
	reader := &IndexReader{
		Docs:    make(map[uint32]models.DocumentRecord),
		Lexicon: make(map[string]models.LexiconEntry),
	}

	// Load Docs
	if err := reader.loadDocs(dir + "/" + models.DocsFileName); err != nil {
		return nil, fmt.Errorf("loading docs: %w", err)
	}

	// Load Lexicon
	if err := reader.loadLexicon(dir + "/" + models.LexiconFileName); err != nil {
		return nil, fmt.Errorf("loading lexicon: %w", err)
	}

	// Open Index
	f, err := os.Open(dir + "/" + models.IndexFileName)
	if err != nil {
		return nil, fmt.Errorf("opening index: %w", err)
	}

	// Verify Index Header
	header := make([]byte, 13) // "DEVSCOPE_IDX" (12) + Ver(1)
	if _, err := io.ReadFull(f, header); err != nil {
		f.Close()
		return nil, err
	}
	if string(header[:12]) != "DEVSCOPE_IDX" {
		f.Close()
		return nil, fmt.Errorf("invalid index header")
	}

	reader.File = f
	return reader, nil
}

func (r *IndexReader) Close() {
	if r.File != nil {
		r.File.Close()
	}
}

func (r *IndexReader) loadDocs(path string) error {
	// Re-implemented using internal/store code or just use store.DocReader?
	// We didn't export NewDocReader in store properly or we defined it in docs_io.go which is in store package.
	// So we can use store.NewDocReader.
	// But reader.go relies on `devscope/internal/store`.
	// I should update imports to use `devscope/internal/store`.
	// But `store.DocReader` returns `models.DocumentRecord`.

	// Wait, I can only import `devscope/internal/store` if I update imports.
	// I'll assume I can add the import.
	// Actually I will reimplement reading here to avoid cross-layer dependency if unnecessary,
	// OR better, reuse `store.DocReader` which I spent time implementing.
	// I'll update imports to include `devscope/internal/store`.
	return r.loadDocsUsingStore(path)
}

// Helper to avoid import cycles / cleaner usage if possible. But cycle is query -> store. store -> models. models -> none. No cycle.
// I will add the import `devscope/internal/store` in the replace block.

func (r *IndexReader) loadDocsUsingStore(path string) error {
	// I need to import store. But I can't put import mid-file.
	// I'll manually implement for now to avoid complexity of editing imports again if I mess up.
	// Actually, I already imported `devscope/pkg/models`.
	// I will just reimplement the read logic since it's simple enough and I want to be sure it matches.
	// Actually, `docs_io.go` has header verification. I should really use it.

	// Let's rely on manual reading as previously implemented but with corrected headers/types.

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	bufReader := bufio.NewReader(f)

	// Verify Header
	const headerStr = "DEVSCOPE_DOCS"
	header := make([]byte, len(headerStr))
	if _, err := io.ReadFull(bufReader, header); err != nil {
		return err
	}
	if string(header) != headerStr {
		return fmt.Errorf("invalid docs header")
	}
	ver, err := bufReader.ReadByte()
	if err != nil {
		return err
	}
	if ver != 1 {
		return fmt.Errorf("bad version")
	}

	for {
		// Read DocID (4)
		var docID uint32
		if err := binary.Read(bufReader, binary.LittleEndian, &docID); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		// Type (1)
		b, err := bufReader.ReadByte()
		if err != nil {
			return err
		}
		docType := models.DocType(b)

		// PathLen (2)
		var pathLen uint16
		if err := binary.Read(bufReader, binary.LittleEndian, &pathLen); err != nil {
			return err
		}

		fmt.Printf("Debug: DocID=%d PathLen=%d\n", docID, pathLen)

		// Path
		pathBytes := make([]byte, pathLen)
		if _, err := io.ReadFull(bufReader, pathBytes); err != nil {
			return err
		}

		// Timestamps (8+8)
		var tMin, tMax int64
		if err := binary.Read(bufReader, binary.LittleEndian, &tMin); err != nil {
			return err
		}
		if err := binary.Read(bufReader, binary.LittleEndian, &tMax); err != nil {
			return err
		}

		doc := models.DocumentRecord{
			DocID:        docID,
			Type:         docType,
			Path:         string(pathBytes),
			TimestampMin: tMin,
			TimestampMax: tMax,
		}
		r.Docs[doc.DocID] = doc
	}
	r.TotalDocs = len(r.Docs)
	return nil
}

func (r *IndexReader) loadLexicon(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := bufio.NewReader(f)

	// Verify Header "DEVSCOPE_LEX"
	header := make([]byte, 12)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}
	if string(header) != "DEVSCOPE_LEX" {
		return fmt.Errorf("bad lexicon header")
	}
	if _, err := reader.ReadByte(); err != nil {
		return err
	} // Version

	for {
		// TermLen (2) - Updated from 1 byte
		var termLen uint16
		if err := binary.Read(reader, binary.LittleEndian, &termLen); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		termBytes := make([]byte, termLen)
		if _, err := io.ReadFull(reader, termBytes); err != nil {
			return err
		}

		// DocFreq(4) + Offset(8) + Len(4) = 16 bytes
		meta := make([]byte, 16)
		if _, err := io.ReadFull(reader, meta); err != nil {
			return err
		}

		entry := models.LexiconEntry{
			Term:         string(termBytes),
			DocFreq:      binary.LittleEndian.Uint32(meta[0:4]),
			Offset:       binary.LittleEndian.Uint64(meta[4:12]),
			PostingCount: binary.LittleEndian.Uint32(meta[12:16]), // Actually byte length
		}
		r.Lexicon[entry.Term] = entry
	}
	return nil
}

func (r *IndexReader) GetPostings(term string) ([]models.Posting, error) {
	entry, ok := r.Lexicon[term]
	if !ok {
		return nil, nil
	}

	if _, err := r.File.Seek(int64(entry.Offset), 0); err != nil {
		return nil, err
	}

	// If we trusted ParsingCount as ByteLength, we could limit reading,
	// but we can just loop DocFreq times.

	postings := make([]models.Posting, 0, entry.DocFreq)
	header := make([]byte, 13) // DocID(4)+Freq(4)+Meta(1)+PosCount(4)

	for i := uint32(0); i < entry.DocFreq; i++ {
		if _, err := io.ReadFull(r.File, header); err != nil {
			return nil, err
		}

		p := models.Posting{
			DocID:     binary.LittleEndian.Uint32(header[0:4]),
			Frequency: binary.LittleEndian.Uint32(header[4:8]),
			Meta:      header[8],
		}
		posCount := binary.LittleEndian.Uint32(header[9:13])

		p.Positions = make([]uint32, posCount)
		posBuf := make([]byte, 4*posCount)
		if _, err := io.ReadFull(r.File, posBuf); err != nil {
			return nil, err
		}
		for j := 0; j < int(posCount); j++ {
			p.Positions[j] = binary.LittleEndian.Uint32(posBuf[j*4 : j*4+4])
		}

		postings = append(postings, p)
	}

	return postings, nil
}
