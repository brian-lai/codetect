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
		TestCaseID: tc.ID,
		Mode:       result.Mode,
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

	// Calculate precision, recall, F1
	totalExpected := len(expectedFiles) + len(expectedSymbols)
	totalFound := len(vr.FilesFound) + len(vr.SymbolsFound)

	if totalExpected > 0 {
		vr.Recall = float64(totalFound) / float64(totalExpected)
	}

	// For precision, we'd need to know total items returned
	// Approximation: assume output quality correlates with finding expected items
	if totalFound > 0 {
		vr.Precision = vr.Recall // Simplified: same as recall when we can't count false positives
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
