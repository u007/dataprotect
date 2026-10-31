package golden

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLanguageFieldValidAndEmpty(t *testing.T) {
	dir := t.TempDir()
	for _, lang := range []string{"", "de", "nl", "fr", "it", "es", "sv"} {
		body := `[{"id":"c1","text":"hello","entities":[]}]`
		if lang != "" {
			body = `[{"id":"c1","text":"hello","entities":[],"language":"` + lang + `"}]`
		}
		if err := os.WriteFile(filepath.Join(dir, "cases.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		cases, err := Load(dir)
		if err != nil {
			t.Fatalf("lang=%q: %v", lang, err)
		}
		if len(cases) != 1 || cases[0].Language != lang {
			t.Fatalf("lang=%q: got %q", lang, cases[0].Language)
		}
	}
}

func TestLoadLanguageFieldRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	for _, lang := range []string{"xx", "DE", "en", "svx"} {
		body := `[{"id":"c1","text":"hello","entities":[],"language":"` + lang + `"}]`
		if err := os.WriteFile(filepath.Join(dir, "cases.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir); err == nil {
			t.Fatalf("lang=%q: expected error", lang)
		}
	}
}

func TestLanguageFieldOmittedWhenEmpty(t *testing.T) {
	c := Case{ID: "c1", Text: "hi", Entities: []Label{}}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) == "" {
		t.Fatal("empty marshal")
	}
	// Ensure the key is absent for an empty language.
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["language"]; ok {
		t.Fatalf("language key present for empty value: %s", b)
	}
}
