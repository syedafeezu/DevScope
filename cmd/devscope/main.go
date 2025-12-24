package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"devscope/internal/indexer"
	"devscope/internal/query"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "index":
		runIndex(os.Args[2:])
	case "search":
		runSearch(os.Args[2:])
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func runIndex(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: devscope index <path_to_index>")
		os.Exit(1)
	}

	root := args[0]
	outDir := ".devscope"
	// make output dir if it dont exist
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Indexing %s -> %s\n", root, outDir)

	builder := indexer.NewIndexBuilder(outDir)
	if err := builder.Build(root); err != nil {
		fmt.Printf("Indexing failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Indexing complete.")
}

func runSearch(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: devscope search <query>")
		os.Exit(1)
	}

	queryStr := strings.Join(args, " ")
	outDir := ".devscope"

	// open the index so we can search it
	idxReader, err := query.NewIndexReader(outDir)
	if err != nil {
		fmt.Printf("Failed to open index: %v\n", err)
		os.Exit(1)
	}
	defer idxReader.Close()

	start := time.Now()
	results, err := query.Search(idxReader, queryStr)
	if err != nil {
		fmt.Printf("Search failed: %v\n", err)
		os.Exit(1)
	}

	duration := time.Since(start)

	fmt.Printf("Found %d results in %v:\n", len(results), duration)
	for i, res := range results {
		fmt.Printf("%d. %s (Line: %d, Score: %.2f, Matches: %d)\n", i+1, res.Path, res.LineNum, res.Score, res.MatchCount)
		fmt.Printf("   %s\n\n", res.Snippet)
	}
}

func printUsage() {
	fmt.Println("DevScope - Code & Log Search Engine")
	fmt.Println("Usage:")
	fmt.Println("  devscope index <path>   # recursive index")
	fmt.Println("  devscope search <query> # search indexed data")
}
