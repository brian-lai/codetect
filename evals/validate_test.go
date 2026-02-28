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

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}
