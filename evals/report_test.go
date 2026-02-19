package evals

import (
	"strings"
	"testing"
	"time"
)

func TestPrintReport_CostBreakdown(t *testing.T) {
	reporter := NewReporter()

	report := &EvalReport{
		Timestamp: time.Now(),
		Config: EvalConfig{
			RepoPath: "/test/repo",
			Model:    "sonnet",
		},
		Summary: ReportSummary{
			TotalCases: 2,
			WithMCP: ModeStats{
				AvgInputTokens:       10000,
				AvgOutputTokens:      500,
				AvgCacheReadTokens:   5000,
				AvgCacheCreateTokens: 8000,
				AvgTotalTokens:       23500,
				AvgCostUSD:           0.05,
				TotalCostUSD:         0.10,
			},
			WithoutMCP: ModeStats{
				AvgInputTokens:       8000,
				AvgOutputTokens:      600,
				AvgCacheReadTokens:   17000,
				AvgCacheCreateTokens: 2000,
				AvgTotalTokens:       27600,
				AvgCostUSD:           0.04,
				TotalCostUSD:         0.08,
			},
		},
	}

	var sb strings.Builder
	reporter.PrintReport(report, &sb)
	out := sb.String()

	// Label fix: "Total Tokens" row should be "Avg Total Tokens"
	if strings.Contains(out, "| Total Tokens") {
		t.Error("report should not contain 'Total Tokens'; expected 'Avg Total Tokens'")
	}
	if !strings.Contains(out, "Avg Total Tokens") {
		t.Error("report should contain 'Avg Total Tokens'")
	}

	// Cost breakdown rows
	for _, label := range []string{"Est. Input Cost", "Est. Output Cost", "Est. Cache Rd Cost", "Est. Cache Cr Cost"} {
		if !strings.Contains(out, label) {
			t.Errorf("report should contain cost breakdown row %q", label)
		}
	}

	// Estimated values use ~$ prefix
	if !strings.Contains(out, "~$") {
		t.Error("estimated cost rows should use '~$' prefix")
	}

	// Summary note is present
	if !strings.Contains(out, "Cache create tokens cost ~12.5x more") {
		t.Error("report should contain cache create cost explanation note")
	}
	if !strings.Contains(out, "tool result overhead") {
		t.Error("report should explain MCP tool result overhead")
	}
}
