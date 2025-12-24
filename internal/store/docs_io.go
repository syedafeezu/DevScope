package store

import (
	"bufio"
	"devscope/pkg/models"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const (
	DocsHeader  = "DEVSCOPE_DOCS"
	DocsVersion = 1
)

// writes docs to the disk
type DocWriter struct {
	file *os.File
	w    *bufio.Writer
}

func NewDocWriter(path string) (*DocWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	dw := &DocWriter{file: f, w: bufio.NewWriter(f)}

	// write the header so we know its our file
	if _, err := dw.w.WriteString(DocsHeader); err != nil {
		return nil, err
	}
	if err := dw.w.WriteByte(DocsVersion); err != nil {
		return nil, err
	}

	return dw, nil
}

func (w *DocWriter) Write(rec models.DocumentRecord) error {
	// calculate size needed for buffer
	buf := make([]byte, 4+1+2+len(rec.Path)+8+8)
	offset := 0

	// put the id first
	binary.LittleEndian.PutUint32(buf[offset:], rec.DocID)
	offset += 4

	buf[offset] = byte(rec.Type)
	offset += 1

	// store path len then the path string
	binary.LittleEndian.PutUint16(buf[offset:], uint16(len(rec.Path)))
	offset += 2

	copy(buf[offset:], rec.Path)
	offset += len(rec.Path)

	// save timestamps for time filtering later
	binary.LittleEndian.PutUint64(buf[offset:], uint64(rec.TimestampMin))
	offset += 8

	binary.LittleEndian.PutUint64(buf[offset:], uint64(rec.TimestampMax))
	offset += 8

	_, err := w.w.Write(buf)
	return err
}

func (w *DocWriter) Close() error {
	// make sure we flush everything before closing
	if err := w.w.Flush(); err != nil {
		return err
	}
	return w.file.Close()
}

// reads docs back from disk
type DocReader struct {
	file *os.File
	r    *bufio.Reader
}

func NewDocReader(path string) (*DocReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	dr := &DocReader{file: f, r: bufio.NewReader(f)}

	// check if header match content
	header := make([]byte, len(DocsHeader))
	if _, err := io.ReadFull(dr.r, header); err != nil {
		return nil, fmt.Errorf("bad headers: %v", err)
	}
	if string(header) != DocsHeader {
		return nil, fmt.Errorf("invalid header: expected %s, got %s", DocsHeader, string(header))
	}
	version, err := dr.r.ReadByte()
	if err != nil {
		return nil, err
	}
	if version != DocsVersion {
		return nil, fmt.Errorf("unsupported version: %d", version)
	}

	return dr, nil
}

func (r *DocReader) ReadNext() (models.DocumentRecord, error) {
	var rec models.DocumentRecord

	// read doc id
	var docID uint32
	if err := binary.Read(r.r, binary.LittleEndian, &docID); err != nil {
		return rec, err
	}
	rec.DocID = docID

	t, err := r.r.ReadByte()
	if err != nil {
		return rec, err
	}
	rec.Type = models.DocType(t)

	// read len of path
	var pathLen uint16
	if err := binary.Read(r.r, binary.LittleEndian, &pathLen); err != nil {
		return rec, err
	}

	pathBuf := make([]byte, pathLen)
	if _, err := io.ReadFull(r.r, pathBuf); err != nil {
		return rec, err
	}
	rec.Path = string(pathBuf)

	// get the timestamps
	if err := binary.Read(r.r, binary.LittleEndian, &rec.TimestampMin); err != nil {
		return rec, err
	}

	if err := binary.Read(r.r, binary.LittleEndian, &rec.TimestampMax); err != nil {
		return rec, err
	}

	return rec, nil
}

func (r *DocReader) Close() error {
	return r.file.Close()
}
