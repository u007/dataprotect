package ner

import (
	"os"
	"path/filepath"
	"testing"
)

func writeModelConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadID2LabelOrdersByID(t *testing.T) {
	path := writeModelConfig(t, `{"id2label":{"2":"I-PER","0":"O","1":"B-PER"}}`)
	got, err := loadID2Label(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"O", "B-PER", "I-PER"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestLoadID2LabelRejectsBadConfigs(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "no id2label", body: `{}`},
		{name: "empty id2label", body: `{"id2label":{}}`},
		{name: "not contiguous", body: `{"id2label":{"0":"O","2":"B-PER"}}`},
		{name: "does not start at zero", body: `{"id2label":{"1":"O","2":"B-PER"}}`},
		{name: "non-numeric key", body: `{"id2label":{"zero":"O"}}`},
		{name: "negative key", body: `{"id2label":{"-1":"O"}}`},
		{name: "empty label", body: `{"id2label":{"0":"","1":"B-PER"}}`},
		{name: "malformed json", body: `{"id2label":`},
		{name: "generic LABEL names", body: `{"id2label":{"0":"LABEL_0","1":"LABEL_1"}}`},
		{name: "no O label", body: `{"id2label":{"0":"B-PER","1":"I-PER"}}`},
		{name: "bare entity type without tag", body: `{"id2label":{"0":"O","1":"PER"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeModelConfig(t, tt.body)
			if _, err := loadID2Label(path); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestLoadID2LabelMissingFile(t *testing.T) {
	if _, err := loadID2Label(filepath.Join(t.TempDir(), "config.json")); err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}

func TestLoadID2LabelAcceptsBIOES(t *testing.T) {
	path := writeModelConfig(t, `{"id2label":{"0":"O","1":"B-PER","2":"I-PER","3":"E-PER","4":"S-LOC","5":"B-MISC"}}`)
	got, err := loadID2Label(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 || got[0] != "O" || got[4] != "S-LOC" {
		t.Fatalf("got %v", got)
	}
}
