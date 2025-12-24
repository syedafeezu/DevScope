package indexer

import (
	"bufio"
	"io"
	"regexp"
	"strings"
	"time"

	"devscope/pkg/models"
)

// RawToken is an intermediate token before indexing
type RawToken struct {
	Term     string
	Position uint32 // Line number for now, or token index
	Meta     uint8
}

const (
	MetaNone           = 0
	MetaInFileName     = 1 << 0
	MetaInFunctionName = 1 << 1
	MetaLogLevelError  = 1 << 2
	MetaLogLevelWarn   = 1 << 3
)

// Tokenize processes a file and returns a slice of tokens.
// It also returns min/max timestamps for logs.
func Tokenize(reader io.Reader, docType models.DocType) ([]RawToken, int64, int64) {
	if docType == models.DocTypeLog {
		return tokenizeLog(reader)
	}
	return tokenizeCode(reader), 0, 0
}

var (
	reIdentifier = regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*`)
	reFuncDef    = regexp.MustCompile(`(func|def|function|class|struct)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
)

func tokenizeCode(reader io.Reader) []RawToken {
	scanner := bufio.NewScanner(reader)
	var tokens []RawToken
	lineNum := uint32(1)

	for scanner.Scan() {
		line := scanner.Text()

		// Check for function definitions
		matches := reFuncDef.FindStringSubmatch(line)
		funcName := ""
		if len(matches) > 2 {
			funcName = matches[2]
		}

		// Find all identifiers
		ids := reIdentifier.FindAllStringIndex(line, -1)
		for _, loc := range ids {
			term := line[loc[0]:loc[1]]
			meta := uint8(MetaNone)
			if term == funcName {
				meta |= MetaInFunctionName
			}

			tokens = append(tokens, RawToken{
				Term:     strings.ToLower(term),
				Position: lineNum, // Using line number as position for snippet retrieval
				Meta:     meta,
			})
		}

		lineNum++
	}
	return tokens
}

func tokenizeLog(reader io.Reader) ([]RawToken, int64, int64) {
	scanner := bufio.NewScanner(reader)
	var tokens []RawToken
	lineNum := uint32(1)
	var minTime, maxTime int64

	// Regex for basic ISO-ish timestamps: 2025-12-20T10:00:00 or 2025-12-20 10:00:00
	// Crude but fast approach: Look for digits and dashes/colons at start

	for scanner.Scan() {
		line := scanner.Text()

		// Parse timestamp
		ts := parseTimestamp(line)
		if ts > 0 {
			if minTime == 0 || ts < minTime {
				minTime = ts
			}
			if ts > maxTime {
				maxTime = ts
			}
		}

		// Parse Level
		meta := uint8(MetaNone)
		upperLine := strings.ToUpper(line)
		if strings.Contains(upperLine, "ERROR") {
			meta |= MetaLogLevelError
		} else if strings.Contains(upperLine, "WARN") {
			meta |= MetaLogLevelWarn
		}

		// Tokenize message
		ids := reIdentifier.FindAllString(line, -1)
		for _, term := range ids {
			tokens = append(tokens, RawToken{
				Term:     strings.ToLower(term),
				Position: lineNum,
				Meta:     meta,
			})
		}

		lineNum++
	}
	return tokens, minTime, maxTime
}

func parseTimestamp(line string) int64 {
	// Try to find something that looks like a date
	// Very simple heuristic: first 19 chars
	if len(line) < 19 {
		return 0
	}
	// "2006-01-02T15:04:05"
	chunk := line[:19]
	// Replace space with T for standard parsing if needed
	chunkT := strings.Replace(chunk, " ", "T", 1)

	t, err := time.Parse("2006-01-02T15:04:05", chunkT)
	if err == nil {
		return t.Unix()
	}
	return 0
}
