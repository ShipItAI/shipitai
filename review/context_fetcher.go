package review

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/shipitai/shipitai/config"
	"github.com/shipitai/shipitai/github"
)

const (
	// MaxFileSize is the maximum size for a single file (50KB).
	MaxFileSize = 50 * 1024

	// TotalContextBudget is the total budget for all context (100KB).
	TotalContextBudget = 100 * 1024

	// FullFilesBudgetRatio is the portion of budget for full files (50%).
	FullFilesBudgetRatio = 0.50

	// TestFilesBudgetRatio is the portion of budget for test files (25%).
	TestFilesBudgetRatio = 0.25

	// ImportsBudgetRatio is the portion of budget for imported files (15%).
	ImportsBudgetRatio = 0.15

	// HistoryBudgetRatio is the portion of budget for commit history (10%).
	// Note: History is text-only so this is more of a placeholder.
	HistoryBudgetRatio = 0.10

	// CommitsPerFile is the maximum number of commits to fetch per file.
	CommitsPerFile = 5

	// ContextFetchTimeout is the timeout for fetching all context.
	ContextFetchTimeout = 30 * time.Second
)

// ContextFetcher fetches enriched context for code reviews.
type ContextFetcher struct {
	client     *github.Client
	logger     *slog.Logger
	modulePath string // Go module path for import resolution
}

// NewContextFetcher creates a new context fetcher.
func NewContextFetcher(client *github.Client, logger *slog.Logger) *ContextFetcher {
	return &ContextFetcher{
		client: client,
		logger: logger,
	}
}

// ContextInput contains the parameters for fetching context.
type ContextInput struct {
	InstallationID int64
	Owner          string
	Repo           string
	HeadRef        string           // The branch/SHA to fetch files from
	ChangedFiles   []string         // List of file paths from the diff
	Config         *config.Config   // Repository config (for context settings)
	Budget         int              // Total size budget in bytes (0 = default)
}

// FetchContext fetches all available context within the size budget.
// Failures are non-fatal - partial context is returned rather than failing.
func (f *ContextFetcher) FetchContext(ctx context.Context, input *ContextInput) *ReviewContext {
	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, ContextFetchTimeout)
	defer cancel()

	result := &ReviewContext{
		FullFiles:     make([]FileContext, 0),
		RelatedFiles:  make([]RelatedFile, 0),
		FileHistories: make([]FileHistory, 0),
	}

	// Check if context is disabled in config
	if input.Config != nil && input.Config.Context != nil {
		if input.Config.Context.Enabled != nil && !*input.Config.Context.Enabled {
			f.logger.Info("context fetching disabled by config")
			return result
		}
	}

	if len(input.ChangedFiles) == 0 {
		return result
	}

	budget := input.Budget
	if budget <= 0 {
		budget = TotalContextBudget
	}

	// Calculate budget allocations
	fullFilesBudget := int(float64(budget) * FullFilesBudgetRatio)
	testFilesBudget := int(float64(budget) * TestFilesBudgetRatio)
	importsBudget := int(float64(budget) * ImportsBudgetRatio)

	var budgetUsed int

	// Check config for which context types are enabled
	fetchFullFiles := true
	fetchRelatedFiles := true
	fetchHistory := true

	if input.Config != nil && input.Config.Context != nil {
		if input.Config.Context.FullFiles != nil {
			fetchFullFiles = *input.Config.Context.FullFiles
		}
		if input.Config.Context.RelatedFiles != nil {
			fetchRelatedFiles = *input.Config.Context.RelatedFiles
		}
		if input.Config.Context.History != nil {
			fetchHistory = *input.Config.Context.History
		}
	}

	// Priority 1: Fetch full files for changed files
	if fetchFullFiles {
		files, used := f.fetchFullFiles(ctx, input, fullFilesBudget)
		result.FullFiles = files
		budgetUsed += used
		f.logger.Info("fetched full files",
			"count", len(files),
			"budget_used", used,
			"budget_allocated", fullFilesBudget,
		)
	}

	// Priority 2: Fetch test files
	if fetchRelatedFiles && budgetUsed < budget {
		testFiles, used := f.fetchTestFiles(ctx, input, result.FullFiles, testFilesBudget)
		result.RelatedFiles = append(result.RelatedFiles, testFiles...)
		budgetUsed += used
		f.logger.Info("fetched test files",
			"count", len(testFiles),
			"budget_used", used,
		)

		// Priority 3: Fetch imported files (if budget allows)
		remainingBudget := importsBudget
		if budgetUsed < budget {
			importedFiles, used := f.fetchImportedFiles(ctx, input, result.FullFiles, remainingBudget)
			result.RelatedFiles = append(result.RelatedFiles, importedFiles...)
			f.logger.Info("fetched imported files",
				"count", len(importedFiles),
				"budget_used", used,
			)
		}
	}

	// Priority 4: Fetch commit history
	if fetchHistory {
		histories := f.fetchFileHistories(ctx, input)
		result.FileHistories = histories
		f.logger.Info("fetched file histories",
			"count", len(histories),
		)
	}

	f.logger.Info("context fetch complete",
		"full_files", len(result.FullFiles),
		"related_files", len(result.RelatedFiles),
		"histories", len(result.FileHistories),
		"total_size", result.TotalSize(),
	)

	return result
}

// fetchFullFiles fetches the complete content of modified files.
func (f *ContextFetcher) fetchFullFiles(ctx context.Context, input *ContextInput, budget int) ([]FileContext, int) {
	var result []FileContext
	var totalSize int

	// Fetch files in parallel (FetchMultipleFiles handles concurrency internally)
	contents, err := f.client.FetchMultipleFiles(ctx, input.InstallationID, input.Owner, input.Repo, input.ChangedFiles, input.HeadRef)
	if err != nil {
		f.logger.Warn("failed to fetch files", "error", err)
		return result, 0
	}

	// Process fetched files sequentially to respect budget ordering
	for _, path := range input.ChangedFiles {
		content, ok := contents[path]
		if !ok || content == "" {
			continue
		}

		// Check per-file limit
		truncated := false
		if len(content) > MaxFileSize {
			content = content[:MaxFileSize]
			truncated = true
		}

		// Check budget
		if totalSize+len(content) > budget {
			f.logger.Debug("budget exhausted for full files", "path", path)
			break
		}
		totalSize += len(content)

		result = append(result, FileContext{
			Path:      path,
			Content:   content,
			Language:  DetectLanguage(path),
			Truncated: truncated,
		})
	}

	return result, totalSize
}

// fetchTestFiles finds and fetches test files for the modified files.
func (f *ContextFetcher) fetchTestFiles(ctx context.Context, input *ContextInput, fullFiles []FileContext, budget int) ([]RelatedFile, int) {
	var result []RelatedFile
	var totalSize int

	// Collect potential test paths
	var testPaths []string
	pathToSource := make(map[string]string)

	for _, file := range fullFiles {
		paths := GetRelatedTestPaths(file.Path)
		for _, tp := range paths {
			testPaths = append(testPaths, tp)
			pathToSource[tp] = file.Path
		}
	}

	if len(testPaths) == 0 {
		return result, 0
	}

	// Deduplicate
	seen := make(map[string]bool)
	var uniquePaths []string
	for _, p := range testPaths {
		if !seen[p] {
			seen[p] = true
			uniquePaths = append(uniquePaths, p)
		}
	}

	// Fetch test files
	contents, err := f.client.FetchMultipleFiles(ctx, input.InstallationID, input.Owner, input.Repo, uniquePaths, input.HeadRef)
	if err != nil {
		f.logger.Warn("failed to fetch test files", "error", err)
		return result, 0
	}

	for _, path := range uniquePaths {
		content, ok := contents[path]
		if !ok || content == "" {
			continue
		}

		// Check per-file limit
		if len(content) > MaxFileSize {
			content = content[:MaxFileSize]
		}

		// Check budget
		if totalSize+len(content) > budget {
			f.logger.Debug("budget exhausted for test files", "path", path)
			break
		}
		totalSize += len(content)

		result = append(result, RelatedFile{
			Path:         path,
			Relationship: "test",
			Content:      content,
			SourceFile:   pathToSource[path],
		})
	}

	return result, totalSize
}

// fetchImportedFiles finds and fetches locally imported files.
func (f *ContextFetcher) fetchImportedFiles(ctx context.Context, input *ContextInput, fullFiles []FileContext, budget int) ([]RelatedFile, int) {
	var result []RelatedFile
	var totalSize int

	// Collect import paths
	var importPaths []string
	pathToSource := make(map[string]string)

	for _, file := range fullFiles {
		imports := ParseLocalImports(file.Path, file.Content, f.modulePath)
		for _, imp := range imports {
			// Skip if it's one of the changed files (already have full content)
			isChanged := false
			for _, changed := range input.ChangedFiles {
				if imp == changed || strings.HasPrefix(changed, imp+"/") {
					isChanged = true
					break
				}
			}
			if !isChanged {
				importPaths = append(importPaths, imp)
				pathToSource[imp] = file.Path
			}
		}
	}

	if len(importPaths) == 0 {
		return result, 0
	}

	// Deduplicate
	seen := make(map[string]bool)
	var uniquePaths []string
	for _, p := range importPaths {
		if !seen[p] {
			seen[p] = true
			uniquePaths = append(uniquePaths, p)
		}
	}

	// For imports, we might need to try with different extensions
	var pathsToTry []string
	for _, p := range uniquePaths {
		if filepath.Ext(p) == "" {
			// Try common extensions based on other files in the PR
			pathsToTry = append(pathsToTry, p+".ts", p+".tsx", p+".js", p+".jsx", p+"/index.ts", p+"/index.js")
		} else {
			pathsToTry = append(pathsToTry, p)
		}
	}

	// Fetch imported files
	contents, err := f.client.FetchMultipleFiles(ctx, input.InstallationID, input.Owner, input.Repo, pathsToTry, input.HeadRef)
	if err != nil {
		f.logger.Warn("failed to fetch imported files", "error", err)
		return result, 0
	}

	for _, path := range pathsToTry {
		content, ok := contents[path]
		if !ok || content == "" {
			continue
		}

		// Check per-file limit
		if len(content) > MaxFileSize {
			content = content[:MaxFileSize]
		}

		// Check budget
		if totalSize+len(content) > budget {
			f.logger.Debug("budget exhausted for imported files", "path", path)
			break
		}
		totalSize += len(content)

		// Find the source file for this import
		sourceFile := ""
		for _, p := range uniquePaths {
			if strings.HasPrefix(path, p) {
				sourceFile = pathToSource[p]
				break
			}
		}

		result = append(result, RelatedFile{
			Path:         path,
			Relationship: "import",
			Content:      content,
			SourceFile:   sourceFile,
		})
	}

	return result, totalSize
}

// fetchFileHistories fetches recent commit history for modified files.
func (f *ContextFetcher) fetchFileHistories(ctx context.Context, input *ContextInput) []FileHistory {
	var result []FileHistory
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Limit concurrent requests
	sem := make(chan struct{}, 5)

	for _, path := range input.ChangedFiles {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			commits, err := f.client.FetchFileCommits(ctx, input.InstallationID, input.Owner, input.Repo, p, input.HeadRef, CommitsPerFile)
			if err != nil {
				f.logger.Debug("failed to fetch commits for file", "path", p, "error", err)
				return
			}

			if len(commits) == 0 {
				return
			}

			history := FileHistory{
				Path:    p,
				Commits: make([]CommitInfo, 0, len(commits)),
			}

			for _, c := range commits {
				// Extract first line of commit message
				msg := c.Commit.Message
				if idx := strings.Index(msg, "\n"); idx != -1 {
					msg = msg[:idx]
				}
				// Truncate long messages
				if len(msg) > 80 {
					msg = msg[:77] + "..."
				}

				author := ""
				if c.Author != nil {
					author = c.Author.Login
				} else if c.Commit.Author != nil {
					author = c.Commit.Author.Name
				}

				history.Commits = append(history.Commits, CommitInfo{
					SHA:     c.SHA[:7], // Short SHA
					Message: msg,
					Author:  author,
				})
			}

			mu.Lock()
			result = append(result, history)
			mu.Unlock()
		}(path)
	}

	wg.Wait()
	return result
}

// SetModulePath sets the Go module path for import resolution.
func (f *ContextFetcher) SetModulePath(modulePath string) {
	f.modulePath = modulePath
}

// FetchContextForChunk fetches context for a specific chunk of files.
// The budget is divided by the number of chunks.
func (f *ContextFetcher) FetchContextForChunk(ctx context.Context, input *ContextInput, chunkFiles []string, chunkIndex, totalChunks int) *ReviewContext {
	// Create a modified input with only the chunk's files
	chunkInput := &ContextInput{
		InstallationID: input.InstallationID,
		Owner:          input.Owner,
		Repo:           input.Repo,
		HeadRef:        input.HeadRef,
		ChangedFiles:   chunkFiles,
		Config:         input.Config,
		Budget:         input.Budget / totalChunks, // Divide budget among chunks
	}

	return f.FetchContext(ctx, chunkInput)
}
