package evals

import (
	"regexp"
	"strings"
)

// Validator validates run results against ground truth.
type Validator struct{}

// NewValidator creates a new validator.
func NewValidator() *Validator {
	return &Validator{}
}

// Validate checks a run result against the test case ground truth.
func (v *Validator) Validate(tc TestCase, result RunResult) ValidationResult {
	vr := ValidationResult{
		TestCaseID:    tc.ID,
		Mode:          result.Mode,
		ToolCallsMade: result.ToolCallCount,
	}

	// Flag MCP runs where Claude answered without enough tool calls.
	// This indicates Claude may have answered from prior knowledge, making
	// the accuracy improvement metric unreliable for this case.
	minCalls := tc.ToolCallsRequired
	if minCalls <= 0 {
		minCalls = 1 // default: at least one tool call expected for MCP mode
	}
	if result.Mode == ModeWithMCP && result.Success && result.ToolCallCount < minCalls {
		vr.NoToolsWarning = true
	}

	if !result.Success {
		// Failed run gets zero scores
		return vr
	}

	output := strings.ToLower(result.Output)

	// Extract files mentioned in output
	foundFiles := v.extractFiles(result.Output)
	expectedFiles := tc.GroundTruth.Files

	// Calculate file metrics
	if len(expectedFiles) > 0 {
		for _, f := range expectedFiles {
			fLower := strings.ToLower(f)
			if containsPath(foundFiles, fLower) || strings.Contains(output, fLower) {
				vr.FilesFound = append(vr.FilesFound, f)
			} else {
				vr.FilesMissed = append(vr.FilesMissed, f)
			}
		}
	}

	// Extract symbols mentioned in output
	expectedSymbols := tc.GroundTruth.Symbols
	if len(expectedSymbols) > 0 {
		for _, sym := range expectedSymbols {
			symLower := strings.ToLower(sym)
			if strings.Contains(output, symLower) {
				vr.SymbolsFound = append(vr.SymbolsFound, sym)
			} else {
				vr.SymbolsMissed = append(vr.SymbolsMissed, sym)
			}
		}
	}

	// Check content snippets with fuzzy matching
	expectedContent := tc.GroundTruth.Content
	if len(expectedContent) > 0 {
		for _, snippet := range expectedContent {
			if normalizedContains(output, snippet) {
				vr.ContentFound = append(vr.ContentFound, snippet)
			} else {
				vr.ContentMissed = append(vr.ContentMissed, snippet)
			}
		}
	}

	// Count items extracted from output (for precision denominator)
	extractedFiles := v.extractFiles(result.Output)
	extractedSymbols := v.extractSymbols(result.Output)

	// Calculate precision, recall, F1
	totalExpected := len(expectedFiles) + len(expectedSymbols) + len(expectedContent)
	totalFound := len(vr.FilesFound) + len(vr.SymbolsFound) + len(vr.ContentFound)
	totalExtracted := len(extractedFiles) + len(extractedSymbols)

	if totalExpected > 0 {
		vr.Recall = float64(totalFound) / float64(totalExpected)
	}

	// Real precision: correct items / max(correct items, all extracted items).
	// This penalises responses that dump many extra files/symbols while still
	// rewarding focused answers that mention only what's relevant.
	if totalFound > 0 {
		denominator := totalExtracted
		if totalFound > denominator {
			denominator = totalFound
		}
		vr.Precision = float64(totalFound) / float64(denominator)
	}

	// F1 score
	if vr.Precision+vr.Recall > 0 {
		vr.F1Score = 2 * (vr.Precision * vr.Recall) / (vr.Precision + vr.Recall)
	}

	return vr
}

// ValidateAll validates all results in a report.
func (v *Validator) ValidateAll(cases []TestCase, report *EvalReport) {
	caseMap := make(map[string]TestCase)
	for _, tc := range cases {
		caseMap[tc.ID] = tc
	}

	for _, result := range report.RawResults {
		tc, ok := caseMap[result.TestCaseID]
		if !ok {
			continue
		}

		vr := v.Validate(tc, result)

		// Find or create comparison result
		found := false
		for i, cr := range report.Results {
			if cr.TestCaseID == result.TestCaseID {
				if result.Mode == ModeWithMCP {
					report.Results[i].WithMCP = vr
				} else {
					report.Results[i].WithoutMCP = vr
				}
				found = true
				break
			}
		}

		if !found {
			cr := ComparisonResult{
				TestCaseID:  tc.ID,
				Category:    tc.Category,
				Description: tc.Description,
			}
			if result.Mode == ModeWithMCP {
				cr.WithMCP = vr
			} else {
				cr.WithoutMCP = vr
			}
			report.Results = append(report.Results, cr)
		}
	}

	// Calculate comparison metrics
	for i := range report.Results {
		cr := &report.Results[i]
		cr.AccuracyDiff = cr.WithMCP.F1Score - cr.WithoutMCP.F1Score

		// Determine winner
		if cr.WithMCP.F1Score > cr.WithoutMCP.F1Score {
			cr.Winner = ModeWithMCP
		} else if cr.WithoutMCP.F1Score > cr.WithMCP.F1Score {
			cr.Winner = ModeWithoutMCP
		}
	}
}

// extractFiles extracts file paths from output text.
func (v *Validator) extractFiles(output string) []string {
	var files []string

	// Match common file path patterns
	patterns := []string{
		`[\w\-./]+\.(go|py|js|ts|tsx|jsx|java|rb|rs|c|cpp|h|hpp|sql|sh)`, // Extension-based
		`[\w\-]+/[\w\-./]+`, // Path with directory
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(output, -1)
		files = append(files, matches...)
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, f := range files {
		if !seen[f] {
			seen[f] = true
			unique = append(unique, f)
		}
	}

	return unique
}

// normalizedContains checks if output contains snippet using two-level matching:
// 1. Exact normalized substring (case-folded, punctuation-normalized)
// 2. Word-window: all words in snippet appear in output within a 10-word window
// This catches paraphrasing (e.g. "jwt-verification" matches "JWT verification")
// while preserving word order (not bag-of-words).
func normalizedContains(output, snippet string) bool {
	normOutput := normalize(output)
	normSnippet := normalize(snippet)

	// Level 1: exact normalized substring
	if strings.Contains(normOutput, normSnippet) {
		return true
	}

	// Level 2: all words appear within a window (order preserved)
	words := strings.Fields(normSnippet)
	if len(words) <= 1 {
		return false // single word already checked above
	}
	outputWords := strings.Fields(normOutput)
	return wordsInWindow(outputWords, words, 10)
}

// normalize lowercases and collapses punctuation/whitespace.
func normalize(s string) string {
	s = strings.ToLower(s)
	r := strings.NewReplacer("-", " ", "_", " ", ".", " ", "/", " ")
	s = r.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// wordsInWindow checks if all words appear in outputWords in order within windowSize.
func wordsInWindow(outputWords, words []string, windowSize int) bool {
	for i, ow := range outputWords {
		if ow != words[0] {
			continue
		}
		// Found first word — check rest within window
		matched := 1
		pos := i
		for _, w := range words[1:] {
			end := pos + windowSize
			if end > len(outputWords) {
				end = len(outputWords)
			}
			found := false
			for j := pos + 1; j < end; j++ {
				if outputWords[j] == w {
					matched++
					pos = j
					found = true
					break
				}
			}
			if !found {
				break
			}
		}
		if matched == len(words) {
			return true
		}
	}
	return false
}

// extractSymbols extracts code identifier tokens from output text.
// It looks for CamelCase and snake_case identifiers of 4+ chars to estimate
// how many distinct symbols the response mentioned (used for precision calculation).
func (v *Validator) extractSymbols(output string) []string {
	// Match CamelCase identifiers (e.g. RunServer, NewOllamaClient)
	// and snake_case identifiers with uppercase (e.g. handleToolsCall)
	re := regexp.MustCompile(`\b[A-Z][a-zA-Z0-9]{3,}|[a-z][a-zA-Z0-9_]{2,}[A-Z][a-zA-Z0-9]*\b`)
	matches := re.FindAllString(output, -1)

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	return unique
}

// containsPath checks if a path is in the list (case-insensitive, partial match).
func containsPath(paths []string, target string) bool {
	for _, p := range paths {
		pLower := strings.ToLower(p)
		if strings.Contains(pLower, target) || strings.Contains(target, pLower) {
			return true
		}
	}
	return false
}
