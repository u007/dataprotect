package ner

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

func TestProseBackendRecognizes(t *testing.T) {
	b := NewProseBackend()
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "person",
			text: "Marcus Feld signed the agreement on Tuesday.",
			want: []string{"PERSON:Marcus Feld"},
		},
		{
			name: "organisation with legal form",
			text: "Fernwood Logistics Inc handles the freight.",
			want: []string{"ORGANIZATION:Fernwood Logistics Inc"},
		},
		{
			name: "organisation with institutional head noun",
			text: "Northgate University admitted a larger cohort.",
			want: []string{"ORGANIZATION:Northgate University"},
		},
		{
			name: "location",
			text: "The conference was held in Lisbon.",
			want: []string{"LOCATION:Lisbon"},
		},
		{
			name: "month is not a location",
			text: "The invoice was issued in January and settled in March.",
			want: nil,
		},
		{
			name: "language is not a location",
			text: "The contract is written in Swedish and English.",
			want: nil,
		},
		{
			name: "season is not a location",
			text: "The audit runs in Autumn each year.",
			want: nil,
		},
		{
			name: "greeting is not part of the name",
			text: "Hello Fatima, your request has been approved.",
			want: []string{"PERSON:Fatima"},
		},
		{
			name: "contact verb is not part of the name",
			text: "Contact Amara Boateng for the shipment paperwork.",
			want: []string{"PERSON:Amara Boateng"},
		},
		{
			name: "street is a location, not an organisation",
			text: "The office is at 14 Elm Street.",
			want: []string{"LOCATION:Elm Street"},
		},
		{
			name: "no entity",
			text: "The quarterly report was filed on time.",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := b.Recognize(context.Background(), tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotStrs := make([]string, 0, len(got))
			for _, e := range got {
				gotStrs = append(gotStrs, string(e.Type)+":"+e.Text)
				// The span must cover exactly the text it reports.
				if e.Text != tt.text[e.Start:e.End] {
					t.Errorf("span %d..%d covers %q, want %q", e.Start, e.End, tt.text[e.Start:e.End], e.Text)
				}
				if e.Confidence != DefaultConfidence {
					t.Errorf("confidence %v, want %v", e.Confidence, DefaultConfidence)
				}
			}
			if len(gotStrs) != len(tt.want) {
				t.Fatalf("got %v, want %v", gotStrs, tt.want)
			}
			for i := range tt.want {
				if gotStrs[i] != tt.want[i] {
					t.Errorf("got %v, want %v", gotStrs, tt.want)
					break
				}
			}
		})
	}
}

func TestProseBackendName(t *testing.T) {
	if got := NewProseBackend().Name(); got != "prose" {
		t.Fatalf("name %q, want prose", got)
	}
}

func TestProseBackendRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A cancelled context must return an error rather than silently
	// pretending to have found nothing.
	if _, err := NewProseBackend().Recognize(ctx, "Marcus Feld signed it."); err == nil {
		t.Fatal("expected an error on a cancelled context")
	}
}

func TestProseBackendEmptyText(t *testing.T) {
	got, err := NewProseBackend().Recognize(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want no entities", got)
	}
}

func TestMatchesSuffix(t *testing.T) {
	yes := []string{"Inc", "inc", "LTD", "GmbH", "Co", "University", "Committee", "Corporation"}
	for _, w := range yes {
		if !matchesSuffix(w) {
			t.Errorf("matchesSuffix(%q) = false, want true", w)
		}
	}
	no := []string{"Vince", "Anderson", "Grant", "Plain", "Cooper", "Singh"}
	for _, w := range no {
		if matchesSuffix(w) {
			t.Errorf("matchesSuffix(%q) = true, want false", w)
		}
	}
}

func TestNotAnEntity(t *testing.T) {
	if !notAnEntity("January") || !notAnEntity("Swedish") {
		t.Error("a month or a language must not be an entity")
	}
	if notAnEntity("Lisbon") || notAnEntity("Marcus") {
		t.Error("a place or a person must be an entity candidate")
	}
}

func TestPlaceAndBusinessHeadNounsDoNotCollide(t *testing.T) {
	// The two lists must not both claim a word, because the classifier
	// reads the PLACE list first and a word in both would silently be a
	// place. They were separate lists precisely so that "Cape Town" is a
	// place and "Cedarline Systems" is an organisation.
	//
	// The lists are kept disjoint on purpose. A word that could be either
	// ("group", "press", "partners", "media", "capital") is a PLACE head
	// noun here, because a name ending in one of those is more often a
	// venue or a district than a legal entity, and the legal forms are
	// caught by the legal-form branch instead.
	for w := range businessHeadNouns {
		if placeHeadNouns[w] {
			t.Errorf("%q is in both the business and the place list", w)
		}
	}
	// The words the two lists used to share must still resolve to a place.
	for _, w := range []string{"group", "press", "partners", "media", "capital"} {
		if !placeHeadNouns[w] {
			t.Errorf("%q must be a place head noun", w)
		}
	}
}

func TestTrimTrailingPunct(t *testing.T) {
	text := "The ferry crosses the Baltic Sea."
	start, end, ok := trimTrailingPunct(text, len(text)-len("Baltic Sea."), len(text))
	if !ok {
		t.Fatal("expected ok")
	}
	if got := text[start:end]; got != "Baltic Sea" {
		t.Errorf("got %q, want %q", got, "Baltic Sea")
	}
}

var _ = recognizers.EntityPerson
