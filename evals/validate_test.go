package evals

import (
	"math"
	"testing"
)

func TestValidate(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name           string
		tc             TestCase
		result         RunResult
		wantRecall     float64
		wantFilesFound int
		wantSymFound   int
		wantContFound  int
		wantContMissed int
	}{
		{
			name: "backward compat: files+symbols only, no content",
			tc: TestCase{
				ID: "compat-001",
				GroundTruth: GroundTruth{
					Files:   []string{"cmd/main.go"},
					Symbols: []string{"RunServer"},
				},
			},
			result: RunResult{
				Success: true,
				Output:  "The function RunServer is defined in cmd/main.go",
			},
			wantRecall:     1.0,
			wantFilesFound: 1,
			wantSymFound:   1,
			wantContFound:  0,
			wantContMissed: 0,
		},
		{
			name: "content scoring: symbols missed but content matches",
			tc: TestCase{
				ID: "content-001",
				GroundTruth: GroundTruth{
					Files:   []string{"auth/handler.go"},
					Symbols: []string{"validateJWTToken"},
					Content: []string{"JWT verification", "Okta provider"},
				},
			},
			result: RunResult{
				Success: true,
				Output:  "The auth/handler.go file handles JWT verification using the Okta provider for SSO",
			},
			// file found (1) + symbol missed (0) + content found (2) = 3/4
			wantRecall:     0.75,
			wantFilesFound: 1,
			wantSymFound:   0,
			wantContFound:  2,
			wantContMissed: 0,
		},
		{
			name: "content + symbols both contribute",
			tc: TestCase{
				ID: "both-001",
				GroundTruth: GroundTruth{
					Files:   []string{"api/routes.go"},
					Symbols: []string{"RegisterRoutes"},
					Content: []string{"REST endpoints", "middleware chain"},
				},
			},
			result: RunResult{
				Success: true,
				Output:  "In api/routes.go, RegisterRoutes sets up REST endpoints with a middleware chain",
			},
			wantRecall:     1.0,
			wantFilesFound: 1,
			wantSymFound:   1,
			wantContFound:  2,
			wantContMissed: 0,
		},
		{
			name: "empty content field: no effect on scoring",
			tc: TestCase{
				ID: "empty-001",
				GroundTruth: GroundTruth{
					Files:   []string{"pkg/util.go"},
					Symbols: []string{"FormatDate"},
					Content: []string{},
				},
			},
			result: RunResult{
				Success: true,
				Output:  "FormatDate in pkg/util.go formats timestamps",
			},
			wantRecall:     1.0,
			wantFilesFound: 1,
			wantSymFound:   1,
			wantContFound:  0,
			wantContMissed: 0,
		},
		{
			name: "nil content field: same as empty",
			tc: TestCase{
				ID: "nil-001",
				GroundTruth: GroundTruth{
					Files:   []string{"pkg/util.go"},
					Symbols: []string{"FormatDate"},
				},
			},
			result: RunResult{
				Success: true,
				Output:  "FormatDate in pkg/util.go formats timestamps",
			},
			wantRecall:     1.0,
			wantFilesFound: 1,
			wantSymFound:   1,
			wantContFound:  0,
			wantContMissed: 0,
		},
		{
			name: "search-008 reproduction: both valid file locations listed",
			tc: TestCase{
				ID:       "search-008",
				Category: "search",
				GroundTruth: GroundTruth{
					Files:   []string{"types.go", "store.go"},
					Symbols: []string{"MaxRetries"},
				},
			},
			result: RunResult{
				Success: true,
				Output:  "The constant MaxRetries is defined in store.go",
			},
			// file: store.go found (1/2) + symbol found (1/1) = 2/3
			wantRecall:     2.0 / 3.0,
			wantFilesFound: 1,
			wantSymFound:   1,
			wantContFound:  0,
			wantContMissed: 0,
		},
		{
			name: "understand-009 reproduction: content compensates for missed symbols",
			tc: TestCase{
				ID:       "understand-009",
				Category: "understand",
				GroundTruth: GroundTruth{
					Files:   []string{"internal/auth/oauth.go"},
					Symbols: []string{"newOAuthClient", "refreshToken", "validateScopes"},
					Content: []string{"OAuth 2.0 flow", "token refresh", "scope validation"},
				},
			},
			result: RunResult{
				Success: true,
				// Model described concepts (content matches) but summarized away private function names
				Output: "The internal/auth/oauth.go file implements the OAuth 2.0 flow with token refresh and scope validation for third-party integrations",
			},
			// files: 1/1, symbols: 0/3, content: 3/3 = 4/7
			wantRecall:     4.0 / 7.0,
			wantFilesFound: 1,
			wantSymFound:   0,
			wantContFound:  3,
			wantContMissed: 0,
		},
		{
			name: "content matching is case-insensitive",
			tc: TestCase{
				ID: "case-001",
				GroundTruth: GroundTruth{
					Content: []string{"JWT Verification"},
				},
			},
			result: RunResult{
				Success: true,
				Output:  "The system uses jwt verification for auth",
			},
			wantRecall:    1.0,
			wantContFound: 1,
		},
		{
			name: "failed run gets zero scores",
			tc: TestCase{
				ID: "fail-001",
				GroundTruth: GroundTruth{
					Files:   []string{"a.go"},
					Symbols: []string{"Foo"},
					Content: []string{"bar"},
				},
			},
			result: RunResult{
				Success: false,
				Output:  "a.go Foo bar",
			},
			wantRecall:     0.0,
			wantFilesFound: 0,
			wantSymFound:   0,
			wantContFound:  0,
			wantContMissed: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vr := v.Validate(tt.tc, tt.result)

			if !almostEqual(vr.Recall, tt.wantRecall) {
				t.Errorf("Recall = %.4f, want %.4f", vr.Recall, tt.wantRecall)
			}
			if len(vr.FilesFound) != tt.wantFilesFound {
				t.Errorf("FilesFound = %d %v, want %d", len(vr.FilesFound), vr.FilesFound, tt.wantFilesFound)
			}
			if len(vr.SymbolsFound) != tt.wantSymFound {
				t.Errorf("SymbolsFound = %d %v, want %d", len(vr.SymbolsFound), vr.SymbolsFound, tt.wantSymFound)
			}
			if len(vr.ContentFound) != tt.wantContFound {
				t.Errorf("ContentFound = %d %v, want %d", len(vr.ContentFound), vr.ContentFound, tt.wantContFound)
			}
			if len(vr.ContentMissed) != tt.wantContMissed {
				t.Errorf("ContentMissed = %d %v, want %d", len(vr.ContentMissed), vr.ContentMissed, tt.wantContMissed)
			}
		})
	}
}

func TestValidate_ToolCallVerification(t *testing.T) {
	v := NewValidator()
	tc := TestCase{ID: "tool-001", GroundTruth: GroundTruth{Files: []string{"a.go"}}}

	t.Run("MCP with tools: no warning", func(t *testing.T) {
		result := RunResult{
			Mode: ModeWithMCP, Success: true, ToolCallCount: 3,
			Output: "a.go does the thing",
		}
		vr := v.Validate(tc, result)
		if vr.NoToolsWarning {
			t.Error("expected NoToolsWarning=false when tools were called")
		}
		if vr.ToolCallsMade != 3 {
			t.Errorf("ToolCallsMade = %d, want 3", vr.ToolCallsMade)
		}
	})

	t.Run("MCP with zero tools: warning set", func(t *testing.T) {
		result := RunResult{
			Mode: ModeWithMCP, Success: true, ToolCallCount: 0,
			Output: "a.go does the thing",
		}
		vr := v.Validate(tc, result)
		if !vr.NoToolsWarning {
			t.Error("expected NoToolsWarning=true when MCP run called no tools")
		}
		if vr.ToolCallsMade != 0 {
			t.Errorf("ToolCallsMade = %d, want 0", vr.ToolCallsMade)
		}
	})

	t.Run("without_mcp zero tools: no warning", func(t *testing.T) {
		result := RunResult{
			Mode: ModeWithoutMCP, Success: true, ToolCallCount: 0,
			Output: "a.go does the thing",
		}
		vr := v.Validate(tc, result)
		if vr.NoToolsWarning {
			t.Error("expected NoToolsWarning=false for without_mcp mode")
		}
	})

	t.Run("failed run: no warning even with zero tools", func(t *testing.T) {
		result := RunResult{
			Mode: ModeWithMCP, Success: false, ToolCallCount: 0,
		}
		vr := v.Validate(tc, result)
		if vr.NoToolsWarning {
			t.Error("expected NoToolsWarning=false for failed run")
		}
	})
}

func TestValidate_PrecisionScoring(t *testing.T) {
	v := NewValidator()

	t.Run("focused response: precision close to recall", func(t *testing.T) {
		tc := TestCase{
			ID: "prec-001",
			GroundTruth: GroundTruth{
				Files: []string{"internal/auth/handler.go"},
			},
		}
		result := RunResult{
			Mode: ModeWithMCP, Success: true,
			// Only mentions the expected file — focused response
			Output: "The internal/auth/handler.go handles authentication",
		}
		vr := v.Validate(tc, result)
		if vr.Recall != 1.0 {
			t.Errorf("Recall = %.2f, want 1.0", vr.Recall)
		}
		if vr.Precision < 0.5 {
			t.Errorf("Precision = %.2f, want >= 0.5 for focused response", vr.Precision)
		}
	})

	t.Run("dump-everything response: precision lower than recall", func(t *testing.T) {
		tc := TestCase{
			ID: "prec-002",
			GroundTruth: GroundTruth{
				Files:   []string{"internal/auth/handler.go"},
				Symbols: []string{"ValidateToken"},
			},
		}
		result := RunResult{
			Mode: ModeWithMCP, Success: true,
			// Mentions expected items but also dumps many extra symbols
			Output: `The ValidateToken function is in internal/auth/handler.go.
Other functions: NewServer, RunServer, HandleRequest, ProcessPayload, BuildResponse,
ExtractHeaders, ParseBody, ValidateSchema, WriteResponse, CloseConnection,
CreateSession, RefreshToken, RevokeAccess, CheckPermissions, LogAuditEvent`,
		}
		vr := v.Validate(tc, result)
		if vr.Recall != 1.0 {
			t.Errorf("Recall = %.2f, want 1.0 (expected items found)", vr.Recall)
		}
		// Precision should be less than 1.0 due to extra symbols in output
		if vr.Precision >= 1.0 {
			t.Errorf("Precision = %.2f, want < 1.0 for dump-everything response", vr.Precision)
		}
		// F1 should be less than Recall
		if vr.F1Score >= vr.Recall {
			t.Errorf("F1 = %.2f should be less than Recall = %.2f", vr.F1Score, vr.Recall)
		}
	})
}

func TestNormalizedContains(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		snippet string
		want    bool
	}{
		{"exact match", "JWT verification is used", "JWT verification", true},
		{"case insensitive", "jwt verification is used", "JWT Verification", true},
		{"hyphen variant", "uses jwt-verification for auth", "JWT verification", true},
		{"word window match", "JWT token verification is performed", "JWT verification", true},
		{"word order preserved", "verification JWT", "JWT verification", false},
		{"too far apart", "JWT one two three four five six seven eight nine ten eleven verification", "JWT verification", false},
		{"single word", "hello world", "hello", true},
		{"not present", "something else entirely", "JWT verification", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizedContains(tt.output, tt.snippet)
			if got != tt.want {
				t.Errorf("normalizedContains(%q, %q) = %v, want %v", tt.output, tt.snippet, got, tt.want)
			}
		})
	}
}

func TestValidate_FuzzyContentMatching(t *testing.T) {
	v := NewValidator()

	tc := TestCase{
		ID: "fuzzy-001",
		GroundTruth: GroundTruth{
			Content: []string{"exponential backoff", "max retries"},
		},
	}

	t.Run("hyphenated variant matches", func(t *testing.T) {
		result := RunResult{
			Mode: ModeWithMCP, Success: true,
			Output: "Uses exponential-backoff with max-retries of 3",
		}
		vr := v.Validate(tc, result)
		if len(vr.ContentFound) != 2 {
			t.Errorf("ContentFound = %v, want both snippets matched", vr.ContentFound)
		}
	})

	t.Run("word window matches paraphrase", func(t *testing.T) {
		result := RunResult{
			Mode: ModeWithMCP, Success: true,
			Output: "Implements exponential delay backoff strategy with configurable max retries limit",
		}
		vr := v.Validate(tc, result)
		if len(vr.ContentFound) != 2 {
			t.Errorf("ContentFound = %v, want both snippets matched via word window", vr.ContentFound)
		}
	})
}

func TestValidate_ToolCallsRequired(t *testing.T) {
	v := NewValidator()

	t.Run("meets required tool calls: no warning", func(t *testing.T) {
		tc := TestCase{
			ID: "req-001", ToolCallsRequired: 2,
			GroundTruth: GroundTruth{Files: []string{"a.go"}},
		}
		result := RunResult{Mode: ModeWithMCP, Success: true, ToolCallCount: 2, Output: "a.go"}
		vr := v.Validate(tc, result)
		if vr.NoToolsWarning {
			t.Error("should not warn when tool calls meet requirement")
		}
	})

	t.Run("below required tool calls: warning", func(t *testing.T) {
		tc := TestCase{
			ID: "req-002", ToolCallsRequired: 2,
			GroundTruth: GroundTruth{Files: []string{"a.go"}},
		}
		result := RunResult{Mode: ModeWithMCP, Success: true, ToolCallCount: 1, Output: "a.go"}
		vr := v.Validate(tc, result)
		if !vr.NoToolsWarning {
			t.Error("should warn when tool calls below requirement")
		}
	})
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}
