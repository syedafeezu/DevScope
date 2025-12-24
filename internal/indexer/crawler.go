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

// this function walks thru all files recursively
func (c *Crawler) Crawl(out chan<- models.DocumentRecord) error {
	defer close(out)

	docIDCounter := uint32(1)

	// start walkin directory
	return filepath.WalkDir(c.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // just skip bad files
		}
		if d.IsDir() {
			// ignore useless directores
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		docType := determineType(path)
		if docType == -1 {
			return nil // we dont know this file type
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

// checks extension to see what kind of file it is
func determineType(path string) int {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".py", ".js", ".ts", ".c", ".cpp", ".h", ".hpp", ".java", ".rs", ".md", ".txt", ".json", ".yaml", ".yml":
		return int(models.DocTypeCode)
	case ".log":
		return int(models.DocTypeLog)
	}
	return -1
}
