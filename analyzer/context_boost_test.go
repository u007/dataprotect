package analyzer

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// fixedConfidenceRecognizer emits entities at a set confidence so context
// boosting can be observed in isolation from any regex's scoring.
type fixedConfidenceRecognizer struct {
	name       string
	entityType recognizers.EntityType
	text       string
	confidence float64
}

func (r *fixedConfidenceRecognizer) Name() string { return r.name }

func (r *fixedConfidenceRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	start := indexOf(text, r.text)
	if start < 0 {
		return nil, nil
	}
	return []recognizers.Entity{{
		Type:       r.entityType,
		Start:      start,
		End:        start + len(r.text),
		Text:       r.text,
		Confidence: r.confidence,
	}}, nil
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Boosting must be OFF unless explicitly requested. If it were on by default it
// would silently change overlap-resolution outcomes for every existing caller.
func TestContextBoostIsOptIn(t *testing.T) {
	rec := &fixedConfidenceRecognizer{
		name: "fixed", entityType: recognizers.EntityEmail,
		text: "a@b.com", confidence: 0.5,
	}

	off := New()
	off.AddRecognizer(rec)
	got, err := off.Analyze(context.Background(), "your email is a@b.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entity, got %d", len(got))
	}
	if got[0].Confidence != 0.5 {
		t.Errorf("confidence %f, want the unboosted 0.5", got[0].Confidence)
	}

	on := New(WithContextBoost())
	on.AddRecognizer(rec)
	got, err = on.Analyze(context.Background(), "your email is a@b.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Confidence <= 0.5 {
		t.Errorf("confidence %f, want a boost above 0.5", got[0].Confidence)
	}
}

// The ordering constraint: boosting must run BEFORE the confidence filter, or it
// is dead code for every borderline entity. This entity sits below the threshold
// and is rescued only by the boost.
func TestContextBoostRunsBeforeConfidenceFilter(t *testing.T) {
	rec := &fixedConfidenceRecognizer{
		name: "fixed", entityType: recognizers.EntityEmail,
		text: "a@b.com", confidence: 0.7,
	}
	a := New(WithMinConfidence(0.9), WithContextBoost())
	a.AddRecognizer(rec)

	got, err := a.Analyze(context.Background(), "your email contact is a@b.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("boosted entity was filtered out; want 1 entity, got %d: %+v", len(got), got)
	}
	if got[0].Confidence < 0.9 {
		t.Errorf("surviving entity has confidence %f, want >= 0.9", got[0].Confidence)
	}
}

// Without boosting the same entity is correctly filtered out, which proves the
// previous test passes because of the boost and not for an unrelated reason.
func TestContextBoostIsWhatRescuesTheEntity(t *testing.T) {
	rec := &fixedConfidenceRecognizer{
		name: "fixed", entityType: recognizers.EntityEmail,
		text: "a@b.com", confidence: 0.7,
	}
	a := New(WithMinConfidence(0.9))
	a.AddRecognizer(rec)

	got, err := a.Analyze(context.Background(), "your email address is a@b.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 entities without boosting, got %d: %+v", len(got), got)
	}
}

// The second ordering constraint: boosting must run BEFORE overlap resolution so
// it can change which of two colliding recognizers wins. Here the SSN loses on
// raw confidence but wins once "card" in the surrounding text boosts it.
//
// The colliding entity is deliberately not a PHONE. CREDIT_CARD now outranks
// PHONE outright in winsOverlap, so that pair can no longer be decided by
// confidence or by a boost, and using it here would prove nothing about
// ordering. The scores are also kept clear of a tie (0.6 + 0.2 = 0.8 against
// 0.75, not 0.8 against 0.8) so the boosted win is a real win rather than the
// tie resolving to whichever span sorted first.
func TestContextBoostRunsBeforeOverlapResolution(t *testing.T) {
	card := &fixedConfidenceRecognizer{
		name: "card", entityType: recognizers.EntityCreditCard,
		text: "4111111111111111", confidence: 0.6,
	}
	ssn := &fixedConfidenceRecognizer{
		name: "ssn", entityType: recognizers.EntitySSN,
		text: "411111111", confidence: 0.75,
	}

	// Without boosting: the SSN has higher confidence, so it wins the overlap.
	off := New()
	off.AddRecognizer(card)
	off.AddRecognizer(ssn)
	got, err := off.Analyze(context.Background(), "credit card 4111111111111111 here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entity after overlap resolution, got %d: %+v", len(got), got)
	}
	if got[0].Type != recognizers.EntitySSN {
		t.Fatalf("without boosting the SSN should win on confidence, got %q", got[0].Type)
	}

	// With boosting: "card" boosts the credit card past the SSN.
	on := New(WithContextBoost())
	on.AddRecognizer(card)
	on.AddRecognizer(ssn)
	got, err = on.Analyze(context.Background(), "credit card 4111111111111111 here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entity after overlap resolution, got %d: %+v", len(got), got)
	}
	if got[0].Type != recognizers.EntityCreditCard {
		t.Errorf("with context boosting the card should win, got %q", got[0].Type)
	}
}

// The rounding must not have made the confidence filter laxer. Scores below the
// threshold must still be dropped, including ones just under it. If rounding
// quietly widened the bar this test would pass a value that should have failed.
func TestConfidenceFilterStillRejectsBelowThreshold(t *testing.T) {
	tests := []struct {
		confidence float64
		wantKept   bool
	}{
		{0.95, true},    // comfortably above
		{0.9, true},     // exactly at the threshold: >= semantics must keep it
		{0.8999, false}, // just below: must still be dropped
		{0.89, false},   // below
		{0.5, false},    // far below
	}

	const threshold = 0.9
	for _, tt := range tests {
		rec := &fixedConfidenceRecognizer{
			name: "fixed", entityType: recognizers.EntityEmail,
			text: "a@b.com", confidence: tt.confidence,
		}
		// No context keywords nearby, so no boost can apply and the only variable
		// is the score itself.
		a := New(WithMinConfidence(threshold), WithContextBoost())
		a.AddRecognizer(rec)

		got, err := a.Analyze(context.Background(), "a@b.com")
		if err != nil {
			t.Fatalf("confidence %v: unexpected error: %v", tt.confidence, err)
		}
		kept := len(got) == 1
		if kept != tt.wantKept {
			t.Errorf("confidence %v: kept=%v, want kept=%v (threshold %v)",
				tt.confidence, kept, tt.wantKept, threshold)
		}
	}
}

// A boost that lands the score just short of the threshold must still fail. This
// is the case the rounding fix must NOT rescue.
func TestBoostJustBelowThresholdIsStillDropped(t *testing.T) {
	rec := &fixedConfidenceRecognizer{
		name: "fixed", entityType: recognizers.EntityEmail,
		text: "a@b.com", confidence: 0.75,
	}
	a := New(WithMinConfidence(0.9), WithContextBoost())
	a.AddRecognizer(rec)

	// One keyword ("mail") matches via the "email" substring; "contact" and
	// "reach" do not appear. Boost is 0.1, so 0.75 + 0.1 = 0.85 < 0.9.
	got, err := a.Analyze(context.Background(), "mail a@b.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("entity boosted only to 0.85 survived a 0.9 threshold: %+v", got)
	}
}

// roundConfidence must clamp on both ends, absorb float representation noise,
// and NOT rescue a score that is genuinely below the threshold. The last two
// cases are the ones a coarser quantum would get wrong.
func TestRoundConfidence(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{"clamps below zero", -1, 0},
		{"zero", 0, 0},
		{"absorbs float noise", 0.8999999999999999, 0.9},
		{"does not round a real score", 0.123456789, 0.123456789},
		{"does not promote 0.89996 to 0.9", 0.89996, 0.89996},
		{"does not promote 0.8999", 0.8999, 0.8999},
		{"keeps 0.999999 below 1", 0.999999, 0.999999},
		{"one", 1, 1},
		{"clamps above one", 1.5, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := roundConfidence(tt.in); got != tt.want {
				t.Errorf("roundConfidence(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// The exact regression: a 0.7 entity with two keywords of context evidence must
// survive a 0.9 threshold, because 0.7 + 0.2 is 0.9.
func TestThresholdBoundaryRegression(t *testing.T) {
	rec := &fixedConfidenceRecognizer{
		name: "fixed", entityType: recognizers.EntityEmail,
		text: "a@b.com", confidence: 0.7,
	}
	a := New(WithMinConfidence(0.9), WithContextBoost())
	a.AddRecognizer(rec)

	// "email" and "contact" are two whole-word email keywords, so two fire:
	// 0.7 + 0.2 == 0.9.
	got, err := a.Analyze(context.Background(), "email contact a@b.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("0.7 + 0.2 == 0.9 must be kept against a 0.9 threshold, got %d entities: %+v",
			len(got), got)
	}
	if got[0].Confidence != 0.9 {
		t.Errorf("confidence = %v, want exactly 0.9", got[0].Confidence)
	}
}

// belowThresholdSum returns 0.7 + 0.2 evaluated with real float64 arithmetic at
// runtime.
//
// It MUST go through variables. Go evaluates untyped CONSTANT arithmetic at
// arbitrary precision, so writing the literal `0.7 + 0.2` in a struct field
// constant-folds to exactly 0.9 and no IEEE 754 rounding happens at all. A test
// written that way is silently vacuous: it passes with or without the fix it
// supposedly guards, because the value it supplies is already exact.
func belowThresholdSum() float64 {
	var a float64 = 0.7
	var b float64 = 0.2
	return a + b
}

// Guard the premise. If this ever folds to exactly 0.9, the regression test
// below is testing nothing, so fail loudly instead.
func TestBelowThresholdSumReallyIsBelow(t *testing.T) {
	if got := belowThresholdSum(); got >= 0.9 {
		t.Fatalf("belowThresholdSum() = %.20f, want strictly below 0.9; the arithmetic "+
			"is being constant-folded rather than done in float64", got)
	}
}

// A third-party recognizer that returns a COMPUTED confidence must not be able to
// defeat the threshold.
//
// The value has to be one that lands BELOW its mathematical result, because only
// then does the bug appear. An accumulation that overshoots instead would pass
// regardless of whether quantisation happens, so it cannot test this.
func TestThirdPartyRecognizerComputedScoreIsQuantised(t *testing.T) {
	rec := &fixedConfidenceRecognizer{
		name: "thirdparty", entityType: recognizers.EntityEmail,
		text: "a@b.com", confidence: belowThresholdSum(),
	}
	a := New(WithMinConfidence(0.9))
	a.AddRecognizer(rec)

	got, err := a.Analyze(context.Background(), "a@b.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("third-party score dropped at its own threshold: got %d entities: %+v", len(got), got)
	}
	if got[0].Confidence != 0.9 {
		t.Errorf("confidence = %v, want exactly 0.9", got[0].Confidence)
	}
}

// The control: a float64 accumulation that overshoots really does clear the
// threshold with no quantisation at all, which is why it is not a valid
// regression case.
func TestThirdPartyOvershootPassesWithoutQuantisation(t *testing.T) {
	var x float64 = 0.1
	var y float64 = 0.2
	if x+y <= 0.3 {
		t.Fatalf("premise broken: float64 0.1+0.2 is no longer above 0.3")
	}
}

// An exactly-equal tie between two overlapping entities must resolve
// deterministically to the earlier span, not randomly.
func TestEqualConfidenceTieIsDeterministic(t *testing.T) {
	first := &fixedConfidenceRecognizer{
		name: "first", entityType: recognizers.EntityCreditCard,
		text: "4111111111111111", confidence: 0.5,
	}
	// A shorter, fully-contained span at the same confidence.
	second := &fixedConfidenceRecognizer{
		name: "second", entityType: recognizers.EntityPhone,
		text: "4111111111111", confidence: 0.5,
	}

	ctx := context.Background()
	var firstType recognizers.EntityType
	for i := 0; i < 50; i++ {
		a := New()
		a.AddRecognizer(first)
		a.AddRecognizer(second)
		got, err := a.Analyze(ctx, "pay 4111111111111111 now")
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if len(got) != 1 {
			t.Fatalf("iteration %d: want 1 entity, got %d: %+v", i, len(got), got)
		}
		if i == 0 {
			firstType = got[0].Type
			continue
		}
		if got[0].Type != firstType {
			t.Fatalf("iteration %d: tie broke to %q, first iteration broke to %q",
				i, got[0].Type, firstType)
		}
	}
	if firstType != recognizers.EntityCreditCard {
		t.Errorf("tie broke to %q, want the earlier span's CREDIT_CARD", firstType)
	}
}

// Confidence is on a 0..1 scale; a generous context must not push it past 1.0.
func TestContextBoostClampsToOne(t *testing.T) {
	rec := &fixedConfidenceRecognizer{
		name: "fixed", entityType: recognizers.EntityEmail,
		text: "a@b.com", confidence: 0.95,
	}
	a := New(WithContextBoost())
	a.AddRecognizer(rec)

	// Many keywords for the same type, so the boost exceeds the headroom.
	text := "email e-mail mail contact reach card visa iban bank bitcoin a@b.com"
	got, err := a.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entity, got %d", len(got))
	}
	if got[0].Confidence > 1.0 {
		t.Errorf("confidence %f exceeds the 1.0 maximum", got[0].Confidence)
	}
}

// Boosting must not move spans or introduce overlaps; it only touches the
// confidence field.
func TestContextBoostDoesNotAlterSpans(t *testing.T) {
	rec := &fixedConfidenceRecognizer{
		name: "fixed", entityType: recognizers.EntityEmail,
		text: "a@b.com", confidence: 0.5,
	}
	off := New()
	off.AddRecognizer(rec)
	on := New(WithContextBoost())
	on.AddRecognizer(rec)

	text := "your email is a@b.com and mail x@y.org"
	ctx := context.Background()

	base, err := off.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	boosted, err := on.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(base) != len(boosted) {
		t.Fatalf("boosting changed entity count: %d -> %d", len(base), len(boosted))
	}
	for i := range base {
		if base[i].Start != boosted[i].Start || base[i].End != boosted[i].End ||
			base[i].Text != boosted[i].Text || base[i].Type != boosted[i].Type {
			t.Errorf("entity %d changed shape: %+v -> %+v", i, base[i], boosted[i])
		}
	}
}

// A boost must not lift a span inside a URL above that URL. Here the IP has two
// context words, so an unguarded boost takes it from 0.95 to 1.0 and it wins
// overlap resolution. The URL must survive whole.
func TestContextBoostDoesNotLiftSpanInsideURL(t *testing.T) {
	text := "ip address: http://192.168.1.100:8080/status"
	a := New(WithContextBoost())
	a.AddRecognizer(&fixedConfidenceRecognizer{
		name: "url", entityType: recognizers.EntityURL,
		text: "http://192.168.1.100:8080/status", confidence: 0.99,
	})
	a.AddRecognizer(&fixedConfidenceRecognizer{
		name: "ip", entityType: recognizers.EntityIP,
		text: "192.168.1.100", confidence: 0.95,
	})

	got, err := a.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Type != recognizers.EntityURL {
		t.Fatalf("got %+v, want the URL alone", got)
	}
}

// A boost must not lift a span that only partly overlaps a URL either. The IP
// starts inside the "ip address:" context, before the URL, and runs into it.
// Unguarded, its boost takes it above the URL, and overlap resolution keeps the
// IP and drops the URL.
func TestContextBoostDoesNotLiftSpanOverlappingURL(t *testing.T) {
	text := "ip address: http://192.168.1.100:8080/status"
	a := New(WithContextBoost())
	a.AddRecognizer(&fixedConfidenceRecognizer{
		name: "url", entityType: recognizers.EntityURL,
		text: "http://192.168.1.100:8080/status", confidence: 0.99,
	})
	a.AddRecognizer(&fixedConfidenceRecognizer{
		name: "ip", entityType: recognizers.EntityIP,
		text: "address: http://192.168", confidence: 0.95,
	})

	got, err := a.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Type != recognizers.EntityURL {
		t.Fatalf("got %+v, want the URL alone", got)
	}
}

// A URL below minConfidence is dropped by the filter, so it protects nothing. The
// IP inside it must still be boosted, or the threshold removes an IP that the
// context would have kept.
func TestContextBoostIgnoresURLDroppedByThreshold(t *testing.T) {
	text := "ip address: http://192.168.1.100:8080/status"
	a := New(WithContextBoost(), WithMinConfidence(0.5))
	a.AddRecognizer(&fixedConfidenceRecognizer{
		name: "url", entityType: recognizers.EntityURL,
		text: "http://192.168.1.100:8080/status", confidence: 0.30,
	})
	a.AddRecognizer(&fixedConfidenceRecognizer{
		name: "ip", entityType: recognizers.EntityIP,
		text: "192.168.1.100", confidence: 0.45,
	})

	got, err := a.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Type != recognizers.EntityIP {
		t.Fatalf("got %+v, want the boosted IP alone", got)
	}
}
