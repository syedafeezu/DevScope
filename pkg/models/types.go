package models

// this helps us know if its a log or just code
type DocType uint8

const (
	DocTypeCode DocType = 0
	DocTypeLog  DocType = 1
)

// this struct holds the info for each file
type DocumentRecord struct {
	DocID        uint32
	Type         DocType
	Path         string
	TimestampMin int64 // start time if its a log
	TimestampMax int64 // end time if its a log
}

// this is one item in the search result list
type Posting struct {
	DocID     uint32
	Frequency uint32
	Positions []uint32
	Meta      uint8 // extra info like if its inside a functon
}

// this is for looking up words in the index
type LexiconEntry struct {
	Term         string
	DocFreq      uint32
	Offset       uint64 // where to jump in the file
	PostingCount uint32
}

const (
	DocsFileName    = "docs.bin"
	IndexFileName   = "index.bin"
	LexiconFileName = "lexicon.bin"
)
