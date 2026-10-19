package secret

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// testJWT is the public jwt.io example token: a header, a payload and a signature,
// each base64url. The header is 36 characters, above the token floor.
const testJWT = `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNtPnIxz`

// shortHeaderJWT has a 20-character header, under the 32-character floor that
// applies to a bare token value. noneHeaderJWT has a 19-character header, under
// the 20-character floor of the api key shape. Both are synthetic.
const (
	shortHeaderJWT = `eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNtPnIxz`
	noneHeaderJWT  = `eyJhbGciOiJub25lIn0.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNtPnIxz`
)

// testJWE is a synthetic five-segment JWE: header, encrypted key, IV, ciphertext
// and tag. Only the header is valid base64url of JSON; the rest is filler.
const testJWE = `eyJhbGciOiJSU0EtT0FFUCJ9.Q2VrS2V5RW5j.aXZTZWNyZXQx.Y2lwaGVydGV4dA.dGFnVmFsdWUx`

// testDirJWE is a synthetic five-segment JWE with the `dir` key management form:
// the second segment, the encrypted key, is empty. Its 18-character header is under
// the 32-character floor a bare token needs. testDirLongJWE has the same shape with
// a 31-character header. Both are synthetic.
const (
	testDirJWE     = `eyJhbGciOiJkaXIifQ..aXZTZWNyZXQx.Y2lwaGVydGV4dA.dGFnVmFsdWUx`
	testDirLongJWE = `eyJhbGciOiJBMTI4Q0JDLUhTMjU2In0..aXZTZWNyZXQx.Y2lwaGVydGV4dA.dGFnVmFsdWUx`
)

func TestSecretRecognizerAPIKey(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	tests := []struct {
		name     string
		text     string
		wantHits int
	}{
		{"api key", `api_key=sk_live_51N4mK2eZvKYlo2C5x7yPq9w`, 1},
		{"aws access key", `access_token=AKIAIOSFODNN7EXAMPLE`, 1},
		{"slack token", `token=xoxb-1234567890-1234567890-abcdefghijklmnop`, 1},
		{"not a secret", `just some random text here`, 0},
		{"too short", `key=abc123`, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ents) != tt.wantHits {
				t.Fatalf("got %d entities, want %d: %+v", len(ents), tt.wantHits, ents)
			}
			for _, e := range ents {
				if e.Type != recognizers.EntitySecret {
					t.Errorf("got type %q, want %q", e.Type, recognizers.EntitySecret)
				}
				if e.Confidence < 0.7 {
					t.Errorf("confidence %f below threshold", e.Confidence)
				}
			}
		})
	}
}

func TestSecretRecognizerPrivateKey(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	text := `-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEA...
-----END RSA PRIVATE KEY-----`
	ents, err := r.Recognize(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("got %d entities, want 1: %+v", len(ents), ents)
	}
	if ents[0].Type != recognizers.EntitySecret {
		t.Errorf("got type %q, want %q", ents[0].Type, recognizers.EntitySecret)
	}
}

func TestSecretRecognizerBearerToken(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	text := `Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNtPnIxz`
	ents, err := r.Recognize(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("got %d entities, want 1: %+v", len(ents), ents)
	}
	if ents[0].Type != recognizers.EntitySecret {
		t.Errorf("got type %q, want %q", ents[0].Type, recognizers.EntitySecret)
	}
}

// TestSecretRecognizerValueSpan checks that a label-based match is reported over
// the captured value only: the key name, separator, quotes and `Bearer ` scheme
// stay outside the span.
func TestSecretRecognizerValueSpan(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	tests := []struct {
		name string
		text string
		want string
	}{
		{"api key", `Set api_key=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys on the worker`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys`},
		{"token", `Pipeline variable ci_token=Vb4Nm8Qw2Er6Ty0Ui3Op7As1Df5Gh9Jk2Lz6Xc is masked`, `Vb4Nm8Qw2Er6Ty0Ui3Op7As1Df5Gh9Jk2Lz6Xc`},
		{"bearer token", `Authorization: Bearer Hk2Mw9Qe4Rt7Yu1Io6Pa3Sd8Fg5Hj0Kl2Zx4C`, `Hk2Mw9Qe4Rt7Yu1Io6Pa3Sd8Fg5Hj0Kl2Zx4C`},
		{"password", `DB_HOST=db.internal.example password=Tr0ub4dor&Fixture`, `Tr0ub4dor&Fixture`},
		{"quoted password", `password: "Fixture-Pass-2024!"`, `Fixture-Pass-2024!`},
		{"access token of 33 characters reported once", `access_token=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`},
		{"auth token of 33 characters reported once", `auth_token=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`},
		{"jwt bearer token", `Authorization: Bearer ` + testJWT, testJWT},
		{"jwt after token label", `Set token=` + testJWT + ` for the sync job`, testJWT},
		{"jwt access token reported once", `access_token=` + testJWT, testJWT},
		{"dotted prose after a token keeps the bare value", `token=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.Next step`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`},
		{"dotted prose after an api key keeps the bare value", `api_key=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.Next step`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`},
		{"short header jwt bearer", `Authorization: Bearer ` + shortHeaderJWT, shortHeaderJWT},
		{"short header jwt after token label", `Set token=` + shortHeaderJWT + ` for the sync job`, shortHeaderJWT},
		{"short header jwt after api key label", `api_key=` + shortHeaderJWT, shortHeaderJWT},
		{"19 character header jwt after api key label", `api_key=` + noneHeaderJWT, noneHeaderJWT},
		{"short header jwt before dotted prose keeps the jwt", `Authorization: Bearer ` + shortHeaderJWT + `.Next step`, shortHeaderJWT},
		{"short header jwt before sentence period", `Authorization: Bearer ` + shortHeaderJWT + `. Next step`, shortHeaderJWT},
		{"five segment jwe bearer", `Authorization: Bearer ` + testJWE, testJWE},
		{"five segment jwe after access token label", `access_token=` + testJWE, testJWE},
		{"five segment jwe before sentence period", `Authorization: Bearer ` + testJWE + `.`, testJWE},
		{"bearer value before sentence period", `Authorization: Bearer Hk2Mw9Qe4Rt7Yu1Io6Pa3Sd8Fg5Hj0Kl2Zx4C.`, `Hk2Mw9Qe4Rt7Yu1Io6Pa3Sd8Fg5Hj0Kl2Zx4C`},

		// Fail-open fixes: a value that meets its shape's length floor is reported
		// whatever its character classes.
		{"lower-case passphrase password", `DB_PASSWORD=correcthorsebatterystaple`, `correcthorsebatterystaple`},
		{"all-digit token", `token=48213907561234807219564382019476`, `48213907561234807219564382019476`},
		{"dotted lower-case password", `PGPASSWORD=river.canyon`, `river.canyon`},
		{"lower-case words joined by dots as password", `password=example.config.yaml`, `example.config.yaml`},
		{"lower-case token with dotted lower-case tail", `token=abcdefghijklmnopqrstuvwxyzabcdef.ghi.jkl`, `abcdefghijklmnopqrstuvwxyzabcdef.ghi.jkl`},
		{"form label read as password (known tradeoff)", `Password: Required`, `Required`},
		{"space before password separator", `password : Tr0ub4dor&Fixture`, `Tr0ub4dor&Fixture`},

		// JSON-quoted keys: the span is the value, without its quotes.
		{"json api key", `{"api_key": "Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys"}`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys`},
		{"json password", `{"password": "Tr0ub4dor&Fixture"}`, `Tr0ub4dor&Fixture`},
		{"json single-quoted token", `{'token': 'Hk2Mw9Qe4Rt7Yu1Io6Pa3Sd8Fg5Hj0Kl2Zx4C'}`, `Hk2Mw9Qe4Rt7Yu1Io6Pa3Sd8Fg5Hj0Kl2Zx4C`},
		{"json access token without space", `{"access_token":"Vb4Nm8Qw2Er6Ty0Ui3Op7As1Df5Gh9Jk2Lz6Xc"}`, `Vb4Nm8Qw2Er6Ty0Ui3Op7As1Df5Gh9Jk2Lz6Xc`},
		{"json secret", `{"secret": "Fx8Lm3Qr6Tv9Wy2Zb5Ne1Hc4Jd7Ks0"}`, `Fx8Lm3Qr6Tv9Wy2Zb5Ne1Hc4Jd7Ks0`},

		// Dotted tokens are captured to their end for any segment count.
		{"four segment token", `token=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.a1B2c3D4.e5F6g7H8.i9J0k1L2`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.a1B2c3D4.e5F6g7H8.i9J0k1L2`},
		{"six segment token", `api_key=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.a1B2c3D4.e5F6g7H8.i9J0k1L2.m3N4o5P6`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.a1B2c3D4.e5F6g7H8.i9J0k1L2.m3N4o5P6`},
		{"four segment token before sentence period", `Authorization: Bearer Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.a1B2c3D4.e5F6g7H8. Rotate it`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.a1B2c3D4.e5F6g7H8`},

		// JWE with an empty second segment (dir): accepted at any header length.
		{"dir jwe with short header bearer", `Authorization: Bearer ` + testDirJWE, testDirJWE},
		{"dir jwe with short header after access token label", `access_token=` + testDirJWE, testDirJWE},
		{"dir jwe with long header bearer", `Authorization: Bearer ` + testDirLongJWE, testDirLongJWE},

		// A single dotted segment is kept when the value ends the text or is followed
		// by a non-space byte. Followed by whitespace it is prose and is trimmed (see
		// the dotted prose rows above).
		{"one trailing dotted segment at end of text is captured", `token=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.abc`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.abc`},
		{"jwt with a fourth segment is captured whole", `Authorization: Bearer eyJhbGciOiJIUzI0NiJ9.eyJzdWIiOiJ0ZXN0In0.c2lnbmF0dXJl.extra`, `eyJhbGciOiJIUzI0NiJ9.eyJzdWIiOiJ0ZXN0In0.c2lnbmF0dXJl.extra`},
		{"one trailing dotted segment before a newline is trimmed", "token=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.abc\nnext line", `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`},
		{"one trailing dotted segment inside json is captured", `{"token": "Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.abc", "region": "eu"}`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.abc`},
		{"two trailing dotted segments before whitespace are kept", `Authorization: Bearer ` + testJWT + `.abc.def Next`, testJWT + `.abc.def`},

		// Password sentence punctuation. One trailing `.` before whitespace or the
		// end of text is the sentence period and is dropped. `!` and `?` are part of
		// the value. A second `.` is kept, and so is a `.` followed by `,` or a quote.
		{"password sentence period", `password: Rainbow@2025.`, `Rainbow@2025`},
		{"password sentence period mid text", `Set password: Rainbow@2025. Then rotate it.`, `Rainbow@2025`},
		{"password sentence period before newline", "password=river.canyon.\nnext line", `river.canyon`},
		{"password exclamation mark is part of the value", `password: Hunter2!`, `Hunter2!`},
		{"password question mark is part of the value", `password: Hunter2?`, `Hunter2?`},
		{"password drops one period of two", `password: Rainbow@2025..`, `Rainbow@2025.`},
		{"password period before comma is kept", `password: Rainbow@2025.,`, `Rainbow@2025.,`},
		{"password period before closing quote is kept", `password: "Rainbow@2025."`, `Rainbow@2025.`},
		{"password floor is checked before the period is dropped", `password: Abcdefg.`, `Abcdefg`},

		// Password closing brackets. One trailing `)`, `]` or `}` before whitespace
		// or the end of text is dropped when the value has no opener for it. A value
		// with its own balanced brackets, or with a bracket inside, keeps them.
		{"password closing paren at end of text is dropped", `(pass=Hunter2xyz)`, `Hunter2xyz`},
		{"password closing paren before a newline is dropped", "password=Hunter2xyz)\nnext line", `Hunter2xyz`},
		{"password closing paren after a colon is dropped", `password: Hunter2xyz)`, `Hunter2xyz`},
		{"password closing paren mid text is dropped", `x (password=Hunter2xyz) y`, `Hunter2xyz`},
		{"password closing paren floor is checked before the paren is dropped", `(pass=Abcdefg)`, `Abcdefg`},
		{"password closing square bracket is dropped", `[pass=Hunter2xyz]`, `Hunter2xyz`},
		{"password closing brace is dropped", `{password=Hunter2xyz}`, `Hunter2xyz`},
		{"password with balanced parens is kept whole", `password: Pa(ss)w0rd1`, `Pa(ss)w0rd1`},
		{"password with a paren inside is kept whole", `password=abc)def12345`, `abc)def12345`},
		{"password with a trailing paren and an opener inside is kept", `password: Pa(ss)w0rd1)`, `Pa(ss)w0rd1)`},
		{"password ending in a paren with no opener inside is trimmed once", `password: Hunter2xyz))`, `Hunter2xyz)`},
		{"password with a balanced paren pair at the end is kept", `password=Hunter2(x)`, `Hunter2(x)`},
		{"password with a balanced square bracket pair at the end is kept", `password=Hunter2[x]`, `Hunter2[x]`},
		{"password with a square bracket inside is kept", `password: Pa[ss]w0rd1]`, `Pa[ss]w0rd1]`},
		{"password closing paren before a closing quote is dropped", `password: "Hunter2xyz)"`, `Hunter2xyz`},

		// Short labels `pass` and `pwd` match at a non-letter boundary. `_`, `-`,
		// start of text and a quote are separators. Long labels need no boundary.
		{"pass label at start of text", `pass=Hunter2xyz`, `Hunter2xyz`},
		{"pass label after an underscore", `db_pass=Hunter2xyz`, `Hunter2xyz`},
		{"upper-case pass label after an underscore", `DB_PASS=Hunter2xyz`, `Hunter2xyz`},
		{"pwd label after a hyphen", `my-pwd: Hunter2xyz`, `Hunter2xyz`},
		{"json pass label", `"pass": "Hunter2xyz"`, `Hunter2xyz`},
		{"password label after a word with no boundary", `dbpassword=correcthorsebatterystaple`, `correcthorsebatterystaple`},
		{"upper-case password label after a word with no boundary", `ROOTPASSWORD=correcthorsebatterystaple`, `correcthorsebatterystaple`},
		{"secret label after a word with no boundary", `mysecret=correcthorsebatterystaple`, `correcthorsebatterystaple`},
		{"password reported after a short label inside a word", `Route compass: password=Hunter2xyz`, `Hunter2xyz`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ents) != 1 {
				t.Fatalf("got %d entities, want 1: %+v", len(ents), ents)
			}
			e := ents[0]
			if got := tt.text[e.Start:e.End]; got != tt.want {
				t.Errorf("span text %q, want %q", got, tt.want)
			}
			if e.Text != tt.want {
				t.Errorf("Text %q, want %q", e.Text, tt.want)
			}
		})
	}
}

// TestSecretRecognizerRejectsShortOrUniformValue pins what is not reported. A
// value under its shape's floor never matches: the apiKey floor is 20, the token
// floor is 32 and the password floor is 8. `Bearer example.config.yaml` is
// rejected only by the 32-character token floor, since its first word is 7
// characters. A uniform value (every character identical, or a unit of up to four
// characters repeated at least twice) is rejected by isLikelySecret. A bare
// header with no signature and an upper-case `EYJ` are not a JWT. A label with no
// `=` or `:` after it, including a key name on one line and its separator on the
// next, is not a label.
func TestSecretRecognizerRejectsShortOrUniformValue(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	tests := []struct {
		name string
		text string
	}{
		{"api key value under 20 characters", `api_key=Zq7Rt2vN9bLx4Wc8Hm1`},
		{"password value under 8 characters", `password=Tr0ub4d`},
		{"token value under 32 characters", `token=Zq7Rt2vN9bLx4Wc8`},
		{"uniform digit api key value", `api_key=00000000000000000000`},
		{"uniform letter password value", `password=aaaaaaaaaaaaaaaaaaaaaaaaa`},
		{"uniform unit of two characters", `api_key=abababababababababab`},
		{"uniform unit of four characters", `password=abcdabcdabcd`},
		{"bearer followed by short dotted word", `Authorization: Bearer example.config.yaml`},
		{"bearer header with no signature is not a jwt", `Authorization: Bearer eyJhbGciOiJIUzI1NiJ9`},
		{"upper-case EYJ is not the jwt header prefix", `api_key=EYJhbGciOiJub25lIn0.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNtPnIxz`},
		{"prose mentioning password with no separator", `the password is "x"`},
		{"json key with no separator", `{"api_key" "Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys"}`},
		{"key and separator on different lines", "api_key\n: Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys"},
		{"password of short dotted words with a period", `password: a.b.c.`},
		{"password uniform once its period is dropped", `password=aaaaaaa.`},
		{"password uniform once its closing paren is dropped", `(password=aaaaaaaa)`},
		{"password under the floor once its closing paren is dropped", `(pass=Abcdef)`},
		{"pass label inside a word", `bypass=kittenmitten`},
		{"pass label inside a word in prose", `Route compass: northeastregion`},
		{"pwd label inside a word", `tripwd=abcdefghij`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ents) != 0 {
				t.Fatalf("got %d entities, want 0: %+v", len(ents), ents)
			}
		})
	}
}
