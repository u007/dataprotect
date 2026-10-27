// Package analyzer provides the PII analysis engine.
package analyzer

import (
	"context"
	"math"
	"sort"

	ctxconf "github.com/u007/dataprotect/internal/context"
	"github.com/u007/dataprotect/recognizers"
)

// Analyzer orchestrates multiple recognizers to detect PII in text.
type Analyzer struct {
	recognizers      []recognizers.Recognizer
	minConfidence    float64
	contextBoost     bool
	contextBoostLang string
}

// Option configures an Analyzer.
type Option func(*Analyzer)

// WithMinConfidence sets the minimum confidence threshold.
//
// An entity is KEPT when its confidence is greater than or equal to this
// value. The threshold is quantised through roundConfidence, the same helper
// applied to each entity's score, so the comparison cannot fail on
// representation noise alone.
func WithMinConfidence(confidence float64) Option {
	return func(a *Analyzer) {
		a.SetMinConfidence(confidence)
	}
}

// SetMinConfidence changes the minimum confidence threshold.
//
// It is intended for construction-time configuration, before the analyzer is
// used. Analyze reads the field without a lock, so calling SetMinConfidence
// concurrently with Analyze is a data race.
//
// The value goes through the same roundConfidence quantisation as
// WithMinConfidence, so the two cannot disagree about where the threshold sits.
func (a *Analyzer) SetMinConfidence(confidence float64) {
	a.minConfidence = roundConfidence(confidence)
}

// WithContextBoost enables raising an entity's confidence when keywords for its
// type appear in the surrounding text (e.g. the word "email" next to an address).
// It is off by default because boosting changes overlap-resolution outcomes, so
// it is an explicit opt-in rather than a silent behaviour change. A span inside a
// URL that survives the threshold is not boosted (see applyContextBoost).
func WithContextBoost() Option {
	return func(a *Analyzer) {
		a.contextBoost = true
	}
}

// WithContextBoostLanguage enables context boosting with the keyword set for
// lang ("de", "nl", "fr", "it", "es"). English is the default and the
// fallback; a language without a set behaves like WithContextBoost. It is off
// by default for the same reason as WithContextBoost, and a local-language
// keyword cannot boost an English entity unless that language is requested.
func WithContextBoostLanguage(lang string) Option {
	return func(a *Analyzer) {
		a.contextBoost = true
		a.contextBoostLang = lang
	}
}

// New creates a new Analyzer.
func New(opts ...Option) *Analyzer {
	a := &Analyzer{
		minConfidence: 0.0,
		contextBoost:  false,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// AddRecognizer adds a recognizer to the analyzer.
func (a *Analyzer) AddRecognizer(r recognizers.Recognizer) {
	a.recognizers = append(a.recognizers, r)
}

// Recognizers returns the registered recognizers in the order they were added.
//
// This exists so a caller can verify which pipeline it wired — notably whether
// the checksum validators are present, which is not visible from Analyze's
// results alone: a Luhn-failing card is correctly absent whether the validator
// rejected it or was never registered.
func (a *Analyzer) Recognizers() []recognizers.Recognizer {
	out := make([]recognizers.Recognizer, len(a.recognizers))
	copy(out, a.recognizers)
	return out
}

// Analyze runs all recognizers and returns detected entities.
//
// Order matters here. Context boosting is applied FIRST, before the confidence
// filter and before overlap resolution:
//
//   - Filtering first would make boosting dead for every borderline entity: a
//     candidate below minConfidence would already be discarded before the boost
//     that would have rescued it was ever computed.
//   - Resolving overlaps first would make boosting cosmetic: it could no longer
//     change which of two colliding recognizers wins, which is precisely the
//     decision context is supposed to inform (a card number near the word "card"
//     beats a bare digit run of another type that merely looks like a number).
//     Note that CREDIT_CARD against PHONE is decided by winsOverlap before
//     confidence is consulted, so a boost cannot flip that one pair; see the
//     comment on winsOverlap for why.
func (a *Analyzer) Analyze(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var allEntities []recognizers.Entity

	for _, r := range a.recognizers {
		entities, err := r.Recognize(ctx, text)
		if err != nil {
			return nil, err
		}
		allEntities = append(allEntities, entities...)
	}

	// Quantise every score on the way in, whatever recognizer produced it. The
	// built-in recognizers use exact two-decimal literals so this is a no-op for
	// them, but a third-party recognizer returning a computed score such as
	// 0.1+0.2 would otherwise reach the threshold comparison unrounded and hit
	// the same float bug this function exists to prevent.
	quantiseEntities(allEntities)

	if a.contextBoost {
		allEntities = applyContextBoost(text, allEntities, a.minConfidence, a.contextBoostLang)
	}

	// Filter by confidence
	if a.minConfidence > 0 {
		filtered := make([]recognizers.Entity, 0, len(allEntities))
		for _, e := range allEntities {
			if e.Confidence >= a.minConfidence {
				filtered = append(filtered, e)
			}
		}
		allEntities = filtered
	}

	// Sort by start position
	sort.Slice(allEntities, func(i, j int) bool {
		return allEntities[i].Start < allEntities[j].Start
	})

	// Remove overlapping entities (type precedence, then higher confidence)
	allEntities = removeOverlapping(allEntities)

	return allEntities, nil
}

// confidenceScale is the reciprocal of confidenceResolution, the quantum used to
// quantise a confidence value.
//
// Without quantisation, a boosted confidence is silently lossy at the
// threshold. BoostConfidence accumulates 0.1 per keyword and IEEE 754 addition
// does not land where the decimal arithmetic suggests: a 0.7 entity boosted by
// two keywords computes 0.7 + 0.2 as 0.8999999999999999, which fails the `>=
// minConfidence` test. An entity carrying exactly enough evidence to clear the
// bar was being dropped.
//
// The resolution is deliberately far finer than any real score step. Built-in
// recognizers score in two decimals and context boosts move by 0.1, so 1e-9 is
// roughly seven orders of magnitude below the smallest meaningful difference. A
// coarser quantum would be actively wrong here: rounding to four places would
// promote a 0.89996 to 0.9 and let it past a 0.9 threshold, which is a real
// loosening. At 1e-9 only float representation noise is absorbed.
const confidenceResolution = 1e-9

const confidenceScale = 1 / confidenceResolution

// roundConfidence quantises c and clamps it to [0, 1].
//
// This is the single place confidence is quantised. Both sides of the `>=`
// threshold comparison in Analyze go through it (the score via applyContextBoost,
// the threshold via WithMinConfidence), so the two can never disagree because
// one was snapped and the other was not.
func roundConfidence(c float64) float64 {
	return math.Round(math.Min(math.Max(c, 0), 1.0)*confidenceScale) / confidenceScale
}

// quantiseEntities snaps every entity's confidence onto the 1e-9 grid.
func quantiseEntities(entities []recognizers.Entity) {
	for i := range entities {
		entities[i].Confidence = roundConfidence(entities[i].Confidence)
	}
}

// applyContextBoost raises each entity's confidence using keywords in the
// surrounding text. Confidence is clamped to 1.0 so a boost can never push an
// entity above the maximum of the scale.
//
// URLs are boosted first. A span that overlaps a URL surviving minConfidence is
// not boosted: the boost could lift it above that URL, and overlap resolution
// would then keep the inner span, leaving the URL's path and query visible. A URL
// below minConfidence is dropped by the filter anyway, so it protects nothing and
// the spans it overlaps are boosted as usual.
func applyContextBoost(text string, entities []recognizers.Entity, minConfidence float64, lang string) []recognizers.Entity {
	for i := range entities {
		if entities[i].Type == recognizers.EntityURL {
			boostEntity(text, &entities[i], lang)
		}
	}
	for i := range entities {
		if entities[i].Type == recognizers.EntityURL || overlapsSurvivingURL(entities[i], entities, minConfidence) {
			continue
		}
		boostEntity(text, &entities[i], lang)
	}
	return entities
}

// boostEntity adds the context boost for e's type, if any, to its confidence.
func boostEntity(text string, e *recognizers.Entity, lang string) {
	boost := ctxconf.BoostConfidence(text, e.Type, e.Start, e.End)
	if lang != "" {
		boost = ctxconf.BoostConfidenceForLanguage(text, e.Type, e.Start, e.End, lang)
	}
	if boost == 0 {
		return
	}
	e.Confidence = roundConfidence(e.Confidence + boost)
}

// overlapsSurvivingURL reports whether e overlaps, at any point, a URL entity in
// entities whose confidence is at least minConfidence. A URL is not guarded
// against itself.
func overlapsSurvivingURL(e recognizers.Entity, entities []recognizers.Entity, minConfidence float64) bool {
	for _, u := range entities {
		if u.Type == recognizers.EntityURL && u.Confidence >= minConfidence && e.Start < u.End && u.Start < e.End {
			return true
		}
	}
	return false
}

func removeOverlapping(entities []recognizers.Entity) []recognizers.Entity {
	if len(entities) <= 1 {
		return entities
	}
	result := make([]recognizers.Entity, 0, len(entities))
	result = append(result, entities[0])
	for i := 1; i < len(entities); i++ {
		last := &result[len(result)-1]
		if entities[i].Start < last.End {
			// Overlap: type precedence first, confidence second.
			if winsOverlap(entities[i], *last) {
				*last = entities[i]
			}
		} else {
			result = append(result, entities[i])
		}
	}
	return result
}

// winsOverlap reports whether candidate should survive a direct overlap against
// current.
//
// Confidence decides every pair except CREDIT_CARD against PHONE, where the
// card always wins. A card window needs 13 or more digits behind a valid
// network prefix, so when the two collide the phone match is a sub-run of
// digits the card has already claimed, and the card is the more specific
// reading of them. Letting confidence decide leaks: a label-gated card scores
// 0.85 against the phone's 0.98, so the phone took the span, masked its own
// sub-run and left the card's first group in the clear ("Credit/Debit Card 3467
// ***-***-8675"). A Luhn-valid card at 0.99 already won this pair, so the rule
// changes only the gated case, and it makes an intent that was previously a
// coincidence of scores explicit.
//
// Dropping the phone here costs no coverage: every digit it matched sits inside
// the card span that replaces it. The phone winning is what loses digits,
// because the card's leading group begins before the phone match does.
func winsOverlap(candidate, current recognizers.Entity) bool {
	cardVsPhone := candidate.Type == recognizers.EntityCreditCard &&
		current.Type == recognizers.EntityPhone
	phoneVsCard := candidate.Type == recognizers.EntityPhone &&
		current.Type == recognizers.EntityCreditCard
	if cardVsPhone || phoneVsCard {
		return cardVsPhone
	}
	return candidate.Confidence > current.Confidence
}
