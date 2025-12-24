package query

import (
	"bufio"
	"devscope/pkg/models"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// helping us read the index files
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

	// first we load docs meta data
	if err := reader.loadDocs(dir + "/" + models.DocsFileName); err != nil {
		return nil, fmt.Errorf("loading docs failed: %w", err)
	}

	// then we load the dictionary (lexicon)
	if err := reader.loadLexicon(dir + "/" + models.LexiconFileName); err != nil {
		return nil, fmt.Errorf("loading lexicon failed: %w", err)
	}

	// open the big index file
	f, err := os.Open(dir + "/" + models.IndexFileName)
	if err != nil {
		return nil, fmt.Errorf("opening index failed: %w", err)
	}

	// make sure header is valid
	header := make([]byte, 13)
	if _, err := io.ReadFull(f, header); err != nil {
		f.Close()
		return nil, err
	}
	if string(header[:12]) != "DEVSCOPE_IDX" {
		f.Close()
		return nil, fmt.Errorf("invalid header index")
	}

	reader.File = f
	return reader, nil
}

func (r *IndexReader) Close() {
	if r.File != nil {
		r.File.Close() // dont forget to close file handle
	}
}

func (r *IndexReader) loadDocs(path string) error {
	return r.loadDocsUsingStore(path)
}

func (r *IndexReader) loadDocsUsingStore(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	bufReader := bufio.NewReader(f)

	const headerStr = "DEVSCOPE_DOCS"
	header := make([]byte, len(headerStr))
	if _, err := io.ReadFull(bufReader, header); err != nil {
		return err
	}
	if string(header) != headerStr {
		return fmt.Errorf("bad headers docs")
	}
	ver, err := bufReader.ReadByte()
	if err != nil {
		return err
	}
	if ver != 1 {
		return fmt.Errorf("bad ver")
	}

	// looping thru all docs
	for {
		var docID uint32
		if err := binary.Read(bufReader, binary.LittleEndian, &docID); err != nil {
			if err == io.EOF {
				break // done reading
			}
			return err
		}

		b, err := bufReader.ReadByte()
		if err != nil {
			return err
		}
		docType := models.DocType(b)

		var pathLen uint16
		if err := binary.Read(bufReader, binary.LittleEndian, &pathLen); err != nil {
			return err
		}

		pathBytes := make([]byte, pathLen)
		if _, err := io.ReadFull(bufReader, pathBytes); err != nil {
			return err
		}

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

	header := make([]byte, 12)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}
	if string(header) != "DEVSCOPE_LEX" {
		return fmt.Errorf("bad lex header")
	}
	if _, err := reader.ReadByte(); err != nil {
		return err
	}

	for {
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

		meta := make([]byte, 16)
		if _, err := io.ReadFull(reader, meta); err != nil {
			return err
		}

		entry := models.LexiconEntry{
			Term:         string(termBytes),
			DocFreq:      binary.LittleEndian.Uint32(meta[0:4]),
			Offset:       binary.LittleEndian.Uint64(meta[4:12]),
			PostingCount: binary.LittleEndian.Uint32(meta[12:16]),
		}
		r.Lexicon[entry.Term] = entry
	}
	return nil
}

// retrieves the list of docs that contain the term
func (r *IndexReader) GetPostings(term string) ([]models.Posting, error) {
	entry, ok := r.Lexicon[term]
	if !ok {
		return nil, nil // word not found
	}

	// jump to location in file
	if _, err := r.File.Seek(int64(entry.Offset), 0); err != nil {
		return nil, err
	}

	postings := make([]models.Posting, 0, entry.DocFreq)
	header := make([]byte, 13)

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
