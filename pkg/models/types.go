package models

// DocType distinguishes between source code and log files.
type DocType uint8

const (
	DocTypeCode DocType = 0
	DocTypeLog  DocType = 1
)

// DocumentRecord represents the metadata stored for each file in docs.bin.
type DocumentRecord struct {
	DocID        uint32
	Type         DocType
	Path         string
	TimestampMin int64 // For logs: epoch start. For code: ModTime.
	TimestampMax int64 // For logs: epoch end. For code: 0 or ModTime.
}

// Posting represents a single hit in the index.
type Posting struct {
	DocID     uint32
	Frequency uint32
	Positions []uint32
	Meta      uint8 // e.g. bitmask: 0x1=in_filename, 0x2=in_function, 0x4=is_error_log
}

// LexiconEntry represents a term in lexicon.bin (in-memory representation)
type LexiconEntry struct {
	Term         string
	DocFreq      uint32
	Offset       uint64 // Offset in index.bin
	PostingCount uint32
}

const (
	DocsFileName    = "docs.bin"
	IndexFileName   = "index.bin"
	LexiconFileName = "lexicon.bin"
)
