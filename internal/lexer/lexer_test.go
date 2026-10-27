package lexer

import (
	"strings"
	"testing"

	"github.com/bongdo/strizh/internal/token"
)

func mustLex(t *testing.T, src string) ([]token.Token, []Comment, []Error) {
	t.Helper()
	toks, comments, errs := Lex([]byte(src))
	if len(toks) == 0 {
		t.Fatal("no tokens returned")
	}
	if last := toks[len(toks)-1]; last.Kind != token.EOF {
		t.Fatalf("last token is %v, want EOF", last.Kind)
	}
	return toks, comments, errs
}

func wantKinds(t *testing.T, toks []token.Token, kinds []token.Kind) {
	t.Helper()
	if len(toks) != len(kinds) {
		t.Fatalf("got %d tokens, want %d", len(toks), len(kinds))
	}
	for i, k := range kinds {
		if toks[i].Kind != k {
			t.Fatalf("token %d = %v (%q), want %v", i, toks[i].Kind, toks[i].Lit, k)
		}
	}
}

func TestOperators(t *testing.T) {
	src := "<<= >>= ... => << >> <= >= == != && || += -= *= /= %= &= |= ^= + - * / % & | ^ ~ ! = < > . , : ; ( ) { } [ ]"
	want := []token.Kind{
		token.ShlAssign, token.ShrAssign, token.Ellipsis, token.Arrow,
		token.Shl, token.Shr, token.Le, token.Ge, token.Eq, token.Ne,
		token.AndAnd, token.OrOr,
		token.PlusAssign, token.MinusAssign, token.StarAssign, token.SlashAssign,
		token.PercentAssign, token.AmpAssign, token.PipeAssign, token.CaretAssign,
		token.Plus, token.Minus, token.Star, token.Slash, token.Percent,
		token.Amp, token.Pipe, token.Caret, token.Tilde, token.Bang,
		token.Assign, token.Lt, token.Gt, token.Dot, token.Comma, token.Colon,
		token.Semi, token.LParen, token.RParen, token.LBrace, token.RBrace,
		token.LBracket, token.RBracket, token.EOF,
	}
	toks, _, errs := mustLex(t, src)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	wantKinds(t, toks, want)
	for i, tok := range toks {
		if tok.Lit != "" {
			t.Fatalf("token %d (%v) has Lit %q, want empty", i, tok.Kind, tok.Lit)
		}
	}
}

func TestKeywords(t *testing.T) {
	src := "use as fn type struct enum interface var const return if else for in match case par within limit defer break continue or test true false nil map"
	want := []token.Kind{
		token.Use, token.As, token.Fn, token.Type, token.Struct, token.Enum,
		token.Interface, token.Var, token.Const, token.Return, token.If, token.Else,
		token.For, token.In, token.Match, token.Case, token.Par, token.Within,
		token.Limit, token.Defer, token.Break, token.Continue, token.Or, token.Test,
		token.True, token.False, token.Nil, token.Map, token.EOF,
	}
	toks, _, errs := mustLex(t, src)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	wantKinds(t, toks, want)
	for i, tok := range toks {
		if tok.Lit != "" {
			t.Fatalf("token %d (%v) has Lit %q, want empty", i, tok.Kind, tok.Lit)
		}
	}
}

func TestGoReservedWordsAreIdentifiers(t *testing.T) {
	words := strings.Fields("func go chan select switch package import range goto fallthrough default")
	toks, _, errs := mustLex(t, strings.Join(words, " "))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(toks) != len(words)+1 {
		t.Fatalf("got %d tokens, want %d", len(toks), len(words)+1)
	}
	for i, word := range words {
		if toks[i].Kind != token.Ident || toks[i].Lit != word {
			t.Fatalf("token %d = %v (%q), want ident %q", i, toks[i].Kind, toks[i].Lit, word)
		}
	}
}

func TestUnicodeIdentifiers(t *testing.T) {
	words := []string{"héllo", "世界", "_x1", "x_1", "_", "__", "ünïcode"}
	toks, _, errs := mustLex(t, strings.Join(words, " "))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(toks) != len(words)+1 {
		t.Fatalf("got %d tokens, want %d", len(toks), len(words)+1)
	}
	for i, word := range words {
		if toks[i].Kind != token.Ident || toks[i].Lit != word {
			t.Fatalf("token %d = %v (%q), want ident %q", i, toks[i].Kind, toks[i].Lit, word)
		}
	}
}

func TestPositionsUTF8AndCRLF(t *testing.T) {
	src := "héllo\r\n世界 x"
	toks, _, errs := mustLex(t, src)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := []struct {
		pos token.Pos
		end token.Pos
	}{
		{token.Pos{Offset: 0, Line: 1, Col: 1}, token.Pos{Offset: 6, Line: 1, Col: 7}},
		{token.Pos{Offset: 8, Line: 2, Col: 1}, token.Pos{Offset: 14, Line: 2, Col: 7}},
		{token.Pos{Offset: 15, Line: 2, Col: 8}, token.Pos{Offset: 16, Line: 2, Col: 9}},
		{token.Pos{Offset: 16, Line: 2, Col: 9}, token.Pos{Offset: 16, Line: 2, Col: 9}},
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(toks), len(want))
	}
	for i, w := range want {
		if toks[i].Pos != w.pos || toks[i].End != w.end {
			t.Fatalf("token %d positions = %+v..%+v, want %+v..%+v", i, toks[i].Pos, toks[i].End, w.pos, w.end)
		}
	}
}

func TestNewlineForms(t *testing.T) {
	src := "a\r\nb\rc\n"
	toks, _, errs := mustLex(t, src)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := []token.Pos{
		{Offset: 0, Line: 1, Col: 1},
		{Offset: 3, Line: 2, Col: 1},
		{Offset: 5, Line: 3, Col: 1},
		{Offset: 7, Line: 4, Col: 1},
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(toks), len(want))
	}
	for i, p := range want {
		if toks[i].Pos != p {
			t.Fatalf("token %d Pos = %+v, want %+v", i, toks[i].Pos, p)
		}
	}
}

func TestEOFPosition(t *testing.T) {
	cases := []struct {
		src  string
		want token.Pos
	}{
		{"", token.Pos{Offset: 0, Line: 1, Col: 1}},
		{"abc", token.Pos{Offset: 3, Line: 1, Col: 4}},
		{"a\n", token.Pos{Offset: 2, Line: 2, Col: 1}},
		{"a\r\n", token.Pos{Offset: 3, Line: 2, Col: 1}},
		{"a\r", token.Pos{Offset: 2, Line: 2, Col: 1}},
	}
	for _, c := range cases {
		toks, _, _ := mustLex(t, c.src)
		eof := toks[len(toks)-1]
		if eof.Pos != c.want || eof.End != c.want {
			t.Fatalf("EOF for %q = %+v..%+v, want %+v", c.src, eof.Pos, eof.End, c.want)
		}
	}
}

func TestNumbersAndDurations(t *testing.T) {
	cases := []struct {
		src  string
		kind token.Kind
	}{
		{"0", token.Int},
		{"123", token.Int},
		{"1_000", token.Int},
		{"0xFF", token.Int},
		{"0Xab_cd", token.Int},
		{"0o755", token.Int},
		{"0b1010", token.Int},
		{"1.5", token.Float},
		{"1.", token.Float},
		{"1e9", token.Float},
		{"1E-9", token.Float},
		{"1.5e+10", token.Float},
		{"1.5E3", token.Float},
		{"6.02e23", token.Float},
		{"10ns", token.Duration},
		{"500us", token.Duration},
		{"1µs", token.Duration},
		{"2ms", token.Duration},
		{"30s", token.Duration},
		{"5m", token.Duration},
		{"2h", token.Duration},
		{"1.5ms", token.Duration},
		{"1_000ms", token.Duration},
	}
	for _, c := range cases {
		toks, _, errs := mustLex(t, c.src)
		if len(errs) != 0 {
			t.Fatalf("%q: unexpected errors: %v", c.src, errs)
		}
		if len(toks) != 2 {
			t.Fatalf("%q: got %d tokens, want 2", c.src, len(toks))
		}
		if toks[0].Kind != c.kind {
			t.Fatalf("%q: kind = %v, want %v", c.src, toks[0].Kind, c.kind)
		}
		if toks[0].Lit != c.src {
			t.Fatalf("%q: Lit = %q", c.src, toks[0].Lit)
		}
		if toks[0].End.Offset != len(c.src) {
			t.Fatalf("%q: End.Offset = %d, want %d", c.src, toks[0].End.Offset, len(c.src))
		}
	}
}

func TestDurationsDoNotMerge(t *testing.T) {
	toks, _, errs := mustLex(t, "1m30s")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	wantKinds(t, toks, []token.Kind{token.Duration, token.Duration, token.EOF})
	if toks[0].Lit != "1m" || toks[1].Lit != "30s" {
		t.Fatalf("lits = %q, %q, want 1m, 30s", toks[0].Lit, toks[1].Lit)
	}
}

func TestBadNumberPrefixes(t *testing.T) {
	for _, src := range []string{"0x", "0o", "0b"} {
		toks, _, errs := mustLex(t, src)
		if len(errs) != 1 {
			t.Fatalf("%q: got %d errors, want 1", src, len(errs))
		}
		if toks[0].Kind != token.Invalid {
			t.Fatalf("%q: kind = %v, want Invalid", src, toks[0].Kind)
		}
		if !strings.Contains(errs[0].Msg, "has no digits") {
			t.Fatalf("%q: msg = %q", src, errs[0].Msg)
		}
	}
}

func TestStringsAndRunes(t *testing.T) {
	src := `var s = "a\nb\t\"c\\"; var a = 'x'; var b = '\n'; var c = '\'';`
	toks, _, errs := mustLex(t, src)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	wantKinds(t, toks, []token.Kind{
		token.Var, token.Ident, token.Assign, token.String, token.Semi,
		token.Var, token.Ident, token.Assign, token.Rune, token.Semi,
		token.Var, token.Ident, token.Assign, token.Rune, token.Semi,
		token.Var, token.Ident, token.Assign, token.Rune, token.Semi,
		token.EOF,
	})
	wantLit := []string{`"a\nb\t\"c\\"`, `'x'`, `'\n'`, `'\''`}
	gotLit := []string{toks[3].Lit, toks[8].Lit, toks[13].Lit, toks[18].Lit}
	for i := range wantLit {
		if gotLit[i] != wantLit[i] {
			t.Fatalf("literal %d = %q, want %q", i, gotLit[i], wantLit[i])
		}
	}
}

func TestRawString(t *testing.T) {
	src := "var r = `raw\ntext`; var u = `http://x`;"
	toks, comments, errs := mustLex(t, src)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(comments) != 0 {
		t.Fatalf("got %d comments, want 0", len(comments))
	}
	wantKinds(t, toks, []token.Kind{
		token.Var, token.Ident, token.Assign, token.RawString, token.Semi,
		token.Var, token.Ident, token.Assign, token.RawString, token.Semi,
		token.EOF,
	})
	if toks[3].Lit != "`raw\ntext`" {
		t.Fatalf("raw string Lit = %q", toks[3].Lit)
	}
	if toks[3].Pos != (token.Pos{Offset: 8, Line: 1, Col: 9}) {
		t.Fatalf("raw string Pos = %+v", toks[3].Pos)
	}
	if toks[3].End != (token.Pos{Offset: 18, Line: 2, Col: 6}) {
		t.Fatalf("raw string End = %+v", toks[3].End)
	}
	if toks[8].Lit != "`http://x`" {
		t.Fatalf("raw string Lit = %q", toks[8].Lit)
	}
}

func TestUnterminatedString(t *testing.T) {
	toks, _, errs := mustLex(t, `var s = "abc`)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1", len(errs))
	}
	if errs[0].Pos != (token.Pos{Offset: 8, Line: 1, Col: 9}) {
		t.Fatalf("error Pos = %+v", errs[0].Pos)
	}
	if !strings.Contains(errs[0].Msg, "unterminated string literal") {
		t.Fatalf("error Msg = %q", errs[0].Msg)
	}
	wantKinds(t, toks, []token.Kind{token.Var, token.Ident, token.Assign, token.String, token.EOF})
	if toks[3].Lit != `"abc` {
		t.Fatalf("string Lit = %q", toks[3].Lit)
	}
	if toks[3].End != (token.Pos{Offset: 12, Line: 1, Col: 13}) {
		t.Fatalf("string End = %+v", toks[3].End)
	}
}

func TestNewlineInString(t *testing.T) {
	src := "var s = \"abc\nnext;"
	toks, _, errs := mustLex(t, src)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1", len(errs))
	}
	if !strings.Contains(errs[0].Msg, "newline in string literal") {
		t.Fatalf("error Msg = %q", errs[0].Msg)
	}
	wantKinds(t, toks, []token.Kind{
		token.Var, token.Ident, token.Assign, token.String, token.Ident, token.Semi, token.EOF,
	})
	if toks[3].Lit != `"abc` {
		t.Fatalf("string Lit = %q", toks[3].Lit)
	}
	if toks[3].End != (token.Pos{Offset: 12, Line: 1, Col: 13}) {
		t.Fatalf("string End = %+v", toks[3].End)
	}
	if toks[4].Pos != (token.Pos{Offset: 13, Line: 2, Col: 1}) {
		t.Fatalf("next token Pos = %+v", toks[4].Pos)
	}
}

func TestUnterminatedRune(t *testing.T) {
	toks, _, errs := mustLex(t, `var r = 'a`)
	if len(errs) != 1 || !strings.Contains(errs[0].Msg, "unterminated rune literal") {
		t.Fatalf("errors = %v", errs)
	}
	wantKinds(t, toks, []token.Kind{token.Var, token.Ident, token.Assign, token.Rune, token.EOF})
}

func TestUnterminatedRawString(t *testing.T) {
	_, _, errs := mustLex(t, "`abc")
	if len(errs) != 1 || !strings.Contains(errs[0].Msg, "unterminated raw string literal") {
		t.Fatalf("errors = %v", errs)
	}
}

func TestComments(t *testing.T) {
	src := "// line\nx /* block\ncomment */ y"
	toks, comments, errs := mustLex(t, src)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	wantKinds(t, toks, []token.Kind{token.Ident, token.Ident, token.EOF})
	if toks[0].Lit != "x" || toks[1].Lit != "y" {
		t.Fatalf("tokens = %q, %q", toks[0].Lit, toks[1].Lit)
	}
	if len(comments) != 2 {
		t.Fatalf("got %d comments, want 2", len(comments))
	}
	if comments[0].Text != "// line" || comments[0].Block {
		t.Fatalf("comment 0 = %+v", comments[0])
	}
	if comments[0].Pos != (token.Pos{Offset: 0, Line: 1, Col: 1}) {
		t.Fatalf("comment 0 Pos = %+v", comments[0].Pos)
	}
	if comments[0].End != (token.Pos{Offset: 7, Line: 1, Col: 8}) {
		t.Fatalf("comment 0 End = %+v", comments[0].End)
	}
	if comments[1].Text != "/* block\ncomment */" || !comments[1].Block {
		t.Fatalf("comment 1 = %+v", comments[1])
	}
	if comments[1].Pos != (token.Pos{Offset: 10, Line: 2, Col: 3}) {
		t.Fatalf("comment 1 Pos = %+v", comments[1].Pos)
	}
	if comments[1].End != (token.Pos{Offset: 29, Line: 3, Col: 11}) {
		t.Fatalf("comment 1 End = %+v", comments[1].End)
	}
}

func TestUnterminatedBlockComment(t *testing.T) {
	toks, comments, errs := mustLex(t, "x /* foo")
	wantKinds(t, toks, []token.Kind{token.Ident, token.EOF})
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1", len(errs))
	}
	if errs[0].Pos != (token.Pos{Offset: 2, Line: 1, Col: 3}) {
		t.Fatalf("error Pos = %+v", errs[0].Pos)
	}
	if !strings.Contains(errs[0].Msg, "unterminated block comment") {
		t.Fatalf("error Msg = %q", errs[0].Msg)
	}
	if len(comments) != 1 || !comments[0].Block || comments[0].Text != "/* foo" {
		t.Fatalf("comments = %+v", comments)
	}
	if comments[0].End != (token.Pos{Offset: 8, Line: 1, Col: 9}) {
		t.Fatalf("comment End = %+v", comments[0].End)
	}
}

func TestInvalidSymbols(t *testing.T) {
	src := `@ $ # ? \ <- ++ -- ..`
	toks, _, errs := mustLex(t, src)
	if len(errs) != 9 {
		t.Fatalf("got %d errors, want 9", len(errs))
	}
	if len(toks) != 10 {
		t.Fatalf("got %d tokens, want 10", len(toks))
	}
	for i := 0; i < 9; i++ {
		if toks[i].Kind != token.Invalid {
			t.Fatalf("token %d = %v, want Invalid", i, toks[i].Kind)
		}
		if toks[i].Lit == "" {
			t.Fatalf("token %d has empty Lit", i)
		}
	}
	if !strings.Contains(errs[0].Msg, "unexpected character") {
		t.Fatalf("error 0 = %q", errs[0].Msg)
	}
	if !strings.Contains(errs[5].Msg, "<-") {
		t.Fatalf("error 5 = %q", errs[5].Msg)
	}
	if !strings.Contains(errs[6].Msg, "++") {
		t.Fatalf("error 6 = %q", errs[6].Msg)
	}
	if !strings.Contains(errs[7].Msg, "--") {
		t.Fatalf("error 7 = %q", errs[7].Msg)
	}
	if !strings.Contains(errs[8].Msg, "..") {
		t.Fatalf("error 8 = %q", errs[8].Msg)
	}
}

func TestInvalidUTF8(t *testing.T) {
	toks, _, errs := Lex([]byte{'a', 0xff, 'b'})
	if len(toks) != 4 {
		t.Fatalf("got %d tokens, want 4", len(toks))
	}
	if toks[1].Kind != token.Invalid {
		t.Fatalf("token 1 = %v, want Invalid", toks[1].Kind)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Msg, "invalid UTF-8") {
		t.Fatalf("errors = %v", errs)
	}
}

func TestAllBytePairsTerminate(t *testing.T) {
	for a := 0; a < 256; a++ {
		for b := 0; b < 256; b++ {
			toks, _, _ := Lex([]byte{byte(a), byte(b)})
			if len(toks) == 0 || toks[len(toks)-1].Kind != token.EOF {
				t.Fatalf("bytes %#x %#x: missing EOF", a, b)
			}
		}
	}
}

func TestComplexSnippet(t *testing.T) {
	src := `use fmt;
type Node = struct {
	value: u8[16][],
};
fn load(xs: Node[]) void or error = {
	within 30s {
		par for (var i = 0; i < len(xs); i += 1) limit 64 {
			sizes[i] = fetchSize(i) or return;
		};
	};
	var p = nil[*Node];
	return match (p) {
	case _ => return;
	};
};
`
	want := []token.Kind{
		token.Use, token.Ident, token.Semi,
		token.Type, token.Ident, token.Assign, token.Struct, token.LBrace,
		token.Ident, token.Colon, token.Ident, token.LBracket, token.Int,
		token.RBracket, token.LBracket, token.RBracket, token.Comma,
		token.RBrace, token.Semi,
		token.Fn, token.Ident, token.LParen, token.Ident, token.Colon,
		token.Ident, token.LBracket, token.RBracket, token.RParen,
		token.Ident, token.Or, token.Ident, token.Assign, token.LBrace,
		token.Within, token.Duration, token.LBrace,
		token.Par, token.For, token.LParen, token.Var, token.Ident,
		token.Assign, token.Int, token.Semi, token.Ident, token.Lt,
		token.Ident, token.LParen, token.Ident, token.RParen, token.Semi,
		token.Ident, token.PlusAssign, token.Int, token.RParen,
		token.Limit, token.Int, token.LBrace,
		token.Ident, token.LBracket, token.Ident, token.RBracket,
		token.Assign, token.Ident, token.LParen, token.Ident, token.RParen,
		token.Or, token.Return, token.Semi,
		token.RBrace, token.Semi,
		token.RBrace, token.Semi,
		token.Var, token.Ident, token.Assign, token.Nil,
		token.LBracket, token.Star, token.Ident, token.RBracket, token.Semi,
		token.Return, token.Match, token.LParen, token.Ident, token.RParen,
		token.LBrace,
		token.Case, token.Ident, token.Arrow, token.Return, token.Semi,
		token.RBrace, token.Semi,
		token.RBrace, token.Semi,
		token.EOF,
	}
	toks, _, errs := mustLex(t, src)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	wantKinds(t, toks, want)
	if toks[12].Lit != "16" {
		t.Fatalf("array length Lit = %q", toks[12].Lit)
	}
	if toks[34].Lit != "30s" {
		t.Fatalf("duration Lit = %q", toks[34].Lit)
	}
	if toks[42].Lit != "0" || toks[53].Lit != "1" || toks[56].Lit != "64" {
		t.Fatalf("int lits = %q, %q, %q", toks[42].Lit, toks[53].Lit, toks[56].Lit)
	}
}
