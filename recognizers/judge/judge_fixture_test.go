package judge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

type judgeCase struct {
	Text      string `json:"text"`
	Candidate string `json:"candidate"`
	Expected  bool   `json:"expected"`
	Note      string `json:"note"`
}

func loadJudgeFixture(t *testing.T) []judgeCase {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "judge", "mykad.json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var cases []judgeCase
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return cases
}

// TestJudgeFixtureParses is ungated: it runs in CI without any endpoint and
// guards the fixture's shape and count.
func TestJudgeFixtureParses(t *testing.T) {
	cases := loadJudgeFixture(t)
	if len(cases) != 10 {
		t.Fatalf("fixture case count: got %d, want 10", len(cases))
	}
	for i, c := range cases {
		if c.Text == "" || c.Candidate == "" {
			t.Errorf("case %d: empty text/candidate", i)
		}
	}
}

// TestJudgeScoresMyKadFixture is gated on PII_SCRUB_NLP_JUDGE_URL. It skips
// cleanly when no endpoint is configured (the default). When an endpoint is
// available it reports per-case verdicts and accuracy; prediction mismatches
// are logged, not fatal, so a small model's noise does not break CI.
func TestJudgeScoresMyKadFixture(t *testing.T) {
	j, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if j == nil {
		t.Skip("PII_SCRUB_NLP_JUDGE_URL unset; judge disabled")
	}

	cases := loadJudgeFixture(t)
	ctx := t.Context()
	matches := 0
	for i, c := range cases {
		got, err := j.Judge(ctx, recognizers.EntityMYIC, c.Candidate, c.Text)
		if err != nil {
			t.Errorf("case %d (%s): judge error: %v", i, c.Note, err)
			continue
		}
		status := "OK"
		if got != c.Expected {
			status = "MISMATCH"
		} else {
			matches++
		}
		t.Logf("case %d [%s] expected=%v got=%v note=%s", i, status, c.Expected, got, c.Note)
	}
	t.Logf("accuracy: %d/%d", matches, len(cases))
}
