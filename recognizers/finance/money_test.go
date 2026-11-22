package finance

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

func moneyValues(t *testing.T, text string) []string {
	t.Helper()
	ents, err := NewMoneyRecognizer().Recognize(context.Background(), text)
	if err != nil {
		t.Fatalf("Recognize(%q): %v", text, err)
	}
	var out []string
	for _, e := range ents {
		if text[e.Start:e.End] != e.Text {
			t.Errorf("%q: span [%d:%d] is %q but Text is %q", text, e.Start, e.End, text[e.Start:e.End], e.Text)
		}
		if e.Type != recognizers.EntityMoney || e.Confidence != moneyScore {
			t.Errorf("%q: type %s confidence %v", text, e.Type, e.Confidence)
		}
		out = append(out, e.Text)
	}
	return out
}

func TestMoneyPositives(t *testing.T) {
	cases := []struct{ text, want string }{
		{"Fee of $1,250.00 due.", "$1,250.00"},
		{"Pay US$50 today", "US$50"},
		{"Deposit S$ 40 enclosed", "S$ 40"},
		{"Rent RM 900 monthly", "RM 900"},
		{"Rent RM900 monthly", "RM900"},
		{"Price €3.5 million", "€3.5 million"},
		{"Penalty £20k", "£20k"},
		{"Bonus ¥10,000", "¥10,000"},
		{"Total 12,000 USD paid", "12,000 USD"},
		{"Loan 3.5m SGD", "3.5m SGD"},
		{"Amount SGD 2bn", "SGD 2bn"},
		{"Sum USD 100", "USD 100"},
		{"Jumlah 2 juta MYR", "2 juta MYR"},
		{"Value $5 billion", "$5 billion"},
		{"Paid $1,000 thousand", "$1,000 thousand"},
		{"Price per kg: 4,99 EUR this week.", "4,99 EUR"},
		{"Harga €12,50 hari ini", "€12,50"},
	}
	for _, c := range cases {
		got := moneyValues(t, c.text)
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("%q: got %q, want [%q]", c.text, got, c.want)
		}
	}
}

func TestMoneyNegatives(t *testing.T) {
	for _, text := range []string{
		"The count was 1,250 in total.",           // bare number
		"Quarter 3 of 2024 and 42 items",          // bare numbers
		"RM is the abbreviation for ringgit.",     // RM not followed by digit
		"The RMS Titanic sailed",                  // RMS is not RM
		"ARM 5 and ARM5",                          // RM glued to a letter
		"USDA reports 5 cattle",                   // USD glued to letters
		"100 USDT wallet",                         // code followed by letters
		"Use $HOME or $PATH in the shell",         // symbol with no amount
		"hash $2a$10$N9qo8uLOickgx2ZMRZoMye",      // bcrypt: "$10" glued to "$"
		"version 2.5 released",                    // no symbol or code
		"Email me at alice@example.com for $",     // symbol with no digits
		"Code EUROPE 2020 and ISO 4217 reference", // codes not whole word
		"Rate 8.63\nUSD for the day",              // a line break is not the space in "USD 8.63"
		"Total 1,2500 USD in the table",           // "1,2500" is not an amount
		"Cost $1,2500 per unit",                   // "$1" is part of "$1,2500"
	} {
		if got := moneyValues(t, text); len(got) != 0 {
			t.Errorf("%q: got %q, want no MONEY", text, got)
		}
	}
}

func TestMoneyByteOffsetsWithMultibyteSymbols(t *testing.T) {
	text := "Harga €12.50 dan £7 dan ¥300."
	ents, err := NewMoneyRecognizer().Recognize(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"€12.50", "£7", "¥300"}
	if len(ents) != len(want) {
		t.Fatalf("got %d entities, want %d", len(ents), len(want))
	}
	for i, e := range ents {
		if text[e.Start:e.End] != want[i] {
			t.Errorf("entity %d: span %q, want %q", i, text[e.Start:e.End], want[i])
		}
	}
}
