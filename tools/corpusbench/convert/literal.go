package convert

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// NemotronSpans parses the value of nemotron's `spans` column.
//
// The column is a Python repr(), not JSON: keys are single-quoted, values pick
// their quote style per content, and non-printable characters arrive escaped.
// json.Unmarshal rejects it outright, so the shapes the converter must handle
// are parsed here.
//
// # Supported grammar
//
//	spans    := '[' [ element (',' element)* ] ']'
//	element  := dict
//	dict     := '{' [ pair (',' pair)* ] '}'
//	pair     := string ':' value
//	value    := string | int
//	string   := "'" ... "'" | '"' ... '"'      (see escapes)
//	int      := '-'? DIGIT+
//
// Strings recognise exactly the escapes repr() emits: \\ \' \" \n \r \t
// \xNN \uNNNN \UNNNNNNNN. Printable non-ASCII characters arrive raw, which is
// the common case — measured over 7179 printable code points in 0x80..0x2000,
// repr escaped none of them.
//
// Anything else — None, True, False, floats, an unknown escape, an unterminated
// string, trailing input — is an error rather than a guess. repr never emits
// those for this column, so their presence means the data is not what this
// parser claims to understand, and a guess here would silently drop gold labels.
func NemotronSpans(repr string) ([]Span, error) {
	p := &reprParser{src: repr}
	p.skipSpace()
	if p.eof() || p.peek() != '[' {
		return nil, fmt.Errorf("spans: want a list, found %s at byte %d", p.describe(), p.i)
	}
	p.i++
	p.skipSpace()
	if !p.eof() && p.peek() == ']' {
		p.i++
		if p.rest() != "" {
			return nil, fmt.Errorf("spans: trailing input after the list: %s", p.describe())
		}
		return nil, nil
	}

	var out []Span
	for {
		dict, err := p.parseDict()
		if err != nil {
			return nil, fmt.Errorf("spans: %w", err)
		}
		span, err := spanFromDict(dict)
		if err != nil {
			return nil, fmt.Errorf("spans: %w", err)
		}
		out = append(out, span)

		p.skipSpace()
		if p.eof() {
			return nil, fmt.Errorf("spans: unexpected end of input inside the list")
		}
		switch p.peek() {
		case ',':
			p.i++
			p.skipSpace()
		case ']':
			p.i++
			if p.rest() != "" {
				return nil, fmt.Errorf("spans: trailing input after the list: %s", p.describe())
			}
			return out, nil
		default:
			return nil, fmt.Errorf("spans: want ',' or ']', found %s", p.describe())
		}
	}
}

// spanFromDict pulls the three fields the converter needs and validates them.
// A dict missing any of them is a malformed annotation: reporting a span with
// zero offsets would later be indistinguishable from a real annotation at the
// start of the record.
func spanFromDict(dict map[string]reprValue) (Span, error) {
	start, ok := dict["start"]
	if !ok {
		return Span{}, fmt.Errorf("element has no 'start': %s", describeKeys(dict))
	}
	end, ok := dict["end"]
	if !ok {
		return Span{}, fmt.Errorf("element has no 'end': %s", describeKeys(dict))
	}
	label, ok := dict["label"]
	if !ok {
		return Span{}, fmt.Errorf("element has no 'label': %s", describeKeys(dict))
	}
	if start.kind != reprInt || end.kind != reprInt {
		return Span{}, fmt.Errorf("'start' and 'end' must be integers, got %s and %s",
			start.describe(), end.describe())
	}
	if label.kind != reprString {
		return Span{}, fmt.Errorf("'label' must be a string, got %s", label.describe())
	}
	return Span{Start: start.num, End: end.num, Label: label.text}, nil
}

func describeKeys(dict map[string]reprValue) string {
	keys := make([]string, 0, len(dict))
	for k := range dict {
		keys = append(keys, strconv.Quote(k))
	}
	// Sorted for a stable error message across runs.
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return "[" + strings.Join(keys, ", ") + "]"
}

type reprKind int

const (
	reprString reprKind = iota
	reprInt
)

// reprValue is a decoded value. Only the two kinds repr() emits for this
// column are representable; anything else is rejected before reaching here.
type reprValue struct {
	kind reprKind
	text string
	num  int
}

func (v reprValue) describe() string {
	if v.kind == reprInt {
		return "an integer"
	}
	return "a string"
}

type reprParser struct {
	src string
	i   int
}

func (p *reprParser) eof() bool  { return p.i >= len(p.src) }
func (p *reprParser) peek() byte { return p.src[p.i] }
func (p *reprParser) rest() string {
	if p.i >= len(p.src) {
		return ""
	}
	return p.src[p.i:]
}

// describe renders the parser's position for an error message, quoting up to
// 24 bytes so the message stays readable and never dumps a whole record.
func (p *reprParser) describe() string {
	end := p.i + 24
	if end > len(p.src) {
		end = len(p.src)
	}
	return strconv.Quote(p.src[p.i:end])
}

func (p *reprParser) skipSpace() {
	for !p.eof() {
		switch p.peek() {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *reprParser) parseDict() (map[string]reprValue, error) {
	if p.eof() || p.peek() != '{' {
		return nil, fmt.Errorf("want a dict, found %s at byte %d", p.describe(), p.i)
	}
	p.i++
	out := map[string]reprValue{}
	p.skipSpace()
	if !p.eof() && p.peek() == '}' {
		p.i++
		return out, nil
	}
	for {
		key, err := p.parseString()
		if err != nil {
			return nil, fmt.Errorf("key: %w", err)
		}
		p.skipSpace()
		if p.eof() || p.peek() != ':' {
			return nil, fmt.Errorf("want ':' after key %q, found %s", key, p.describe())
		}
		p.i++
		p.skipSpace()
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		out[key] = val

		p.skipSpace()
		if p.eof() {
			return nil, fmt.Errorf("unexpected end of input inside a dict")
		}
		switch p.peek() {
		case ',':
			p.i++
			p.skipSpace()
		case '}':
			p.i++
			return out, nil
		default:
			return nil, fmt.Errorf("want ',' or '}', found %s", p.describe())
		}
	}
}

// parseValue reads an int or a string. A bare word such as None, True or False
// reaches readBareWord and is rejected: repr would emit one only if the column
// held such a value, which is not this corpus's shape.
func (p *reprParser) parseValue() (reprValue, error) {
	if p.eof() {
		return reprValue{}, fmt.Errorf("unexpected end of input where a value was expected")
	}
	c := p.peek()
	if c == '\'' || c == '"' {
		text, err := p.parseString()
		if err != nil {
			return reprValue{}, err
		}
		return reprValue{kind: reprString, text: text}, nil
	}
	if c == '-' || (c >= '0' && c <= '9') {
		return p.parseInt()
	}
	return reprValue{}, fmt.Errorf("unsupported literal %s (want a string or an integer)",
		p.describe())
}

func (p *reprParser) parseInt() (reprValue, error) {
	start := p.i
	if p.peek() == '-' {
		p.i++
	}
	digits := 0
	for !p.eof() && p.peek() >= '0' && p.peek() <= '9' {
		p.i++
		digits++
	}
	if digits == 0 {
		return reprValue{}, fmt.Errorf("malformed integer at byte %d: %s", start, p.describe())
	}
	n, err := strconv.Atoi(p.src[start:p.i])
	if err != nil {
		return reprValue{}, fmt.Errorf("malformed integer %q: %w", p.src[start:p.i], err)
	}
	return reprValue{kind: reprInt, num: n}, nil
}

// parseString reads a quoted string and decodes its escapes. The closing quote
// must match the opening one, so a value holding the other quote kind arrives
// escaped rather than terminated early.
func (p *reprParser) parseString() (string, error) {
	// Guard first: parseDict reaches here after consuming a ',' with no
	// lookahead, so truncated input would otherwise index past the end.
	if p.eof() {
		return "", fmt.Errorf("unexpected end of input where a string was expected")
	}
	quote := p.peek()
	p.i++
	var b strings.Builder
	for {
		if p.eof() {
			return "", fmt.Errorf("unterminated string starting at byte %d", p.i)
		}
		c := p.src[p.i]
		switch {
		case c == quote:
			p.i++
			return b.String(), nil
		case c == '\\':
			p.i++
			if p.eof() {
				return "", fmt.Errorf("unterminated escape at end of input")
			}
			decoded, width, err := decodeEscape(p.src[p.i:])
			if err != nil {
				return "", err
			}
			b.WriteString(decoded)
			p.i += width
		case c == '\n' || c == '\r':
			return "", fmt.Errorf("raw newline inside a string at byte %d", p.i)
		default:
			b.WriteByte(c)
			p.i++
		}
	}
}

// decodeEscape decodes one escape sequence and reports how many bytes it
// consumed. It returns an error for any sequence repr() does not emit rather
// than passing the character through, which is what makes a malformed literal
// visible instead of quietly embedded in the record.
func decodeEscape(s string) (string, int, error) {
	switch s[0] {
	case '\\':
		return `\`, 1, nil
	case '\'':
		return "'", 1, nil
	case '"':
		return `"`, 1, nil
	case 'n':
		return "\n", 1, nil
	case 'r':
		return "\r", 1, nil
	case 't':
		return "\t", 1, nil
	case 'x':
		return decodeHexEscape(s, 2, "xNN")
	case 'u':
		return decodeHexEscape(s, 4, "uNNNN")
	case 'U':
		return decodeHexEscape(s, 8, "UNNNNNNNN")
	}
	return "", 0, fmt.Errorf("unknown escape \\%c", s[0])
}

// decodeHexEscape reads exactly `digits` hex digits after the introducer and
// yields the code point they name. A short or non-hex run is an error, so
// 'short\x0' cannot be read as though it were a valid byte.
func decodeHexEscape(s string, digits int, shape string) (string, int, error) {
	if len(s) < 1+digits {
		return "", 0, fmt.Errorf("malformed escape: want \\%s, found %s",
			shape, strconv.Quote(truncate(s, 1+digits)))
	}
	hex := s[1 : 1+digits]
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return "", 0, fmt.Errorf("malformed escape \\%s%s: hex digits expected", s[0:1], hex)
	}
	r := rune(v)
	if !utf8.ValidRune(r) {
		return "", 0, fmt.Errorf("escape \\%s%s names an invalid code point", s[0:1], hex)
	}
	return string(r), 1 + digits, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
