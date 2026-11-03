package main

import "testing"

func TestRunRejectsLanguageForNemotron(t *testing.T) {
	if _, _, err := run("nemotron-pii", "", 0, "Italian"); err == nil {
		t.Fatal("expected an error when lang is set for nemotron-pii")
	}
}

func TestRunRejectsUnknownCorpus(t *testing.T) {
	if _, _, err := run("bogus", "", 0, ""); err == nil {
		t.Fatal("expected an error for an unknown corpus")
	}
}

func TestNormalizeLangTrimsWhitespace(t *testing.T) {
	got, err := normalizeLang("  Italian ")
	if err != nil {
		t.Fatalf("normalizeLang returned error: %v", err)
	}
	if got != "Italian" {
		t.Errorf("normalizeLang = %q, want %q", got, "Italian")
	}
}

func TestNormalizeLangRejectsBlank(t *testing.T) {
	if _, err := normalizeLang("   "); err == nil {
		t.Fatal("expected an error for a blank lang")
	}
}

func TestOutputNameUsesTrimmedLang(t *testing.T) {
	lang, err := normalizeLang(" Italian")
	if err != nil {
		t.Fatalf("normalizeLang returned error: %v", err)
	}
	if got, want := outputName("gretel-pii-finance", lang), "gretel-pii-finance-italian"; got != want {
		t.Errorf("outputName = %q, want %q", got, want)
	}
}

func TestOutputNameCombinedSplit(t *testing.T) {
	if got, want := outputName("nemotron-pii", ""), "nemotron-pii"; got != want {
		t.Errorf("outputName = %q, want %q", got, want)
	}
}
