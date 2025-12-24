package indexer

import (
	"io/fs"
	"path/filepath"
	"strings"

	"devscope/pkg/models"
)

type Crawler struct {
	Root string
}

func NewCrawler(root string) *Crawler {
	return &Crawler{Root: root}
}

// Crawl walks the directory and sends DocumentRecords to the channel.
// It assigns DocID incremently starting from 1.
func (c *Crawler) Crawl(out chan<- models.DocumentRecord) error {
	defer close(out)
	
	docIDCounter := uint32(1)

	return filepath.WalkDir(c.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip unreadable files
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		docType := determineType(path)
		if docType == -1 {
			return nil // Skip unknown types
		}

		rec := models.DocumentRecord{
			DocID: docIDCounter,
			Type:  models.DocType(docType),
			Path:  path,
		}
		
		out <- rec
		docIDCounter++
		return nil
	})
}

func determineType(path string) int {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	// Code
	case ".go", ".py", ".js", ".ts", ".c", ".cpp", ".h", ".hpp", ".java", ".rs", ".md", ".txt", ".json", ".yaml", ".yml":
		return int(models.DocTypeCode)
	// Log
	case ".log":
		return int(models.DocTypeLog)
	}
	return -1
}
