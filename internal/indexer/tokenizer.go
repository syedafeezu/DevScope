package indexer

import (
	"bufio"
	"io"
	"regexp"
	"strings"
	"time"

	"devscope/pkg/models"
)

// raw token before we process it fully
type RawToken struct {
	Term     string
	Position uint32
	Meta     uint8
}

const (
	MetaNone           = 0
	MetaInFileName     = 1 << 0
	MetaInFunctionName = 1 << 1
	MetaLogLevelError  = 1 << 2
	MetaLogLevelWarn   = 1 << 3
)

// helper to decide which tokenizer function to use
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

// We Tokenize code files here
func tokenizeCode(reader io.Reader) []RawToken {
	scanner := bufio.NewScanner(reader)
	var tokens []RawToken
	tokenCounter := uint32(0)

	for scanner.Scan() {
		line := scanner.Text()

		matches := reFuncDef.FindStringSubmatch(line)
		funcName := ""
		if len(matches) > 2 {
			funcName = matches[2]
		}

		ids := reIdentifier.FindAllStringIndex(line, -1)
		for _, loc := range ids {
			term := line[loc[0]:loc[1]]
			meta := uint8(MetaNone)
			if term == funcName {
				meta |= MetaInFunctionName
			}

			tokenCounter++
			tokens = append(tokens, RawToken{
				Term:     strings.ToLower(term),
				Position: tokenCounter,
				Meta:     meta,
			})
		}
	}
	return tokens
}

// We Tokenize Log files here
func tokenizeLog(reader io.Reader) ([]RawToken, int64, int64) {
	scanner := bufio.NewScanner(reader)
	var tokens []RawToken
	tokenCounter := uint32(0)
	var minTime, maxTime int64

	for scanner.Scan() {
		line := scanner.Text()

		ts := parseTimestamp(line)
		if ts > 0 {
			if minTime == 0 || ts < minTime {
				minTime = ts
			}
			if ts > maxTime {
				maxTime = ts
			}
		}

		meta := uint8(MetaNone)
		upperLine := strings.ToUpper(line)
		if strings.Contains(upperLine, "ERROR") {
			meta |= MetaLogLevelError
		} else if strings.Contains(upperLine, "WARN") {
			meta |= MetaLogLevelWarn
		}

		ids := reIdentifier.FindAllString(line, -1)
		for _, term := range ids {
			tokenCounter++
			tokens = append(tokens, RawToken{
				Term:     strings.ToLower(term),
				Position: tokenCounter,
				Meta:     meta,
			})
		}
	}
	return tokens, minTime, maxTime
}

func parseTimestamp(line string) int64 {
	// we need at least some chars to make a date
	if len(line) < 19 {
		return 0
	}
	chunk := line[:19]
	chunkT := strings.Replace(chunk, " ", "T", 1)

	// parsing iso format date
	t, err := time.Parse("2006-01-02T15:04:05", chunkT)
	if err == nil {
		return t.Unix()
	}
	return 0
}
