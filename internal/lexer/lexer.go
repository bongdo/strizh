package lexer

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/bongdo/strizh/internal/token"
)

type Comment struct {
	Pos   token.Pos
	End   token.Pos
	Text  string
	Block bool
}

type Error struct {
	Pos token.Pos
	Msg string
}

type lexer struct {
	src      []byte
	i        int
	line     int
	col      int
	tokens   []token.Token
	comments []Comment
	errors   []Error
}

func Lex(src []byte) ([]token.Token, []Comment, []Error) {
	l := &lexer{src: src, line: 1, col: 1}
	l.run()
	return l.tokens, l.comments, l.errors
}

func (l *lexer) run() {
	for l.i < len(l.src) {
		c := l.src[l.i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f':
			l.advance()
		case c == '/' && l.peek(1) == '/':
			l.lineComment()
		case c == '/' && l.peek(1) == '*':
			l.blockComment()
		case c >= '0' && c <= '9':
			l.number()
		case c == '"':
			l.quoted(token.String, '"')
		case c == '\'':
			l.quoted(token.Rune, '\'')
		case c == '`':
			l.rawString()
		case l.identStart():
			l.ident()
		default:
			l.symbol()
		}
	}
	p := l.pos()
	l.tokens = append(l.tokens, token.Token{Kind: token.EOF, Pos: p, End: p})
}

func (l *lexer) pos() token.Pos {
	return token.Pos{Offset: l.i, Line: l.line, Col: l.col}
}

func (l *lexer) peek(n int) byte {
	if l.i+n >= len(l.src) {
		return 0
	}
	return l.src[l.i+n]
}

func (l *lexer) advance() {
	if l.i >= len(l.src) {
		return
	}
	switch l.src[l.i] {
	case '\n':
		l.i++
		l.line++
		l.col = 1
	case '\r':
		l.i++
		if l.i < len(l.src) && l.src[l.i] == '\n' {
			l.i++
		}
		l.line++
		l.col = 1
	default:
		l.i++
		l.col++
	}
}

func (l *lexer) advanceN(n int) {
	l.i += n
	l.col += n
}

func (l *lexer) emit(kind token.Kind, start token.Pos, off int) {
	l.tokens = append(l.tokens, token.Token{
		Kind: kind,
		Lit:  string(l.src[off:l.i]),
		Pos:  start,
		End:  l.pos(),
	})
}

func (l *lexer) emitKind(kind token.Kind, start token.Pos) {
	l.tokens = append(l.tokens, token.Token{Kind: kind, Pos: start, End: l.pos()})
}

func (l *lexer) invalid(start token.Pos, off int, msg string) {
	l.tokens = append(l.tokens, token.Token{
		Kind: token.Invalid,
		Lit:  string(l.src[off:l.i]),
		Pos:  start,
		End:  l.pos(),
	})
	l.errors = append(l.errors, Error{Pos: start, Msg: msg})
}

func (l *lexer) reject(start token.Pos, off, size int, msg string) {
	l.advanceN(size)
	l.invalid(start, off, msg)
}

func (l *lexer) identStart() bool {
	c := l.src[l.i]
	if c == '_' || isLetter(c) {
		return true
	}
	if c < utf8.RuneSelf {
		return false
	}
	r, _ := utf8.DecodeRune(l.src[l.i:])
	return r != utf8.RuneError && unicode.IsLetter(r)
}

func (l *lexer) ident() {
	start, off := l.pos(), l.i
	first := true
	for l.i < len(l.src) {
		c := l.src[l.i]
		if c < utf8.RuneSelf {
			if c != '_' && !isLetter(c) && (first || !isDigit(c)) {
				break
			}
			l.advanceN(1)
		} else {
			r, size := utf8.DecodeRune(l.src[l.i:])
			if r == utf8.RuneError && size == 1 {
				break
			}
			if !unicode.IsLetter(r) && (first || !unicode.IsDigit(r)) {
				break
			}
			l.advanceN(size)
		}
		first = false
	}
	lit := string(l.src[off:l.i])
	if kind := token.Lookup(lit); kind != token.Ident {
		l.emitKind(kind, start)
		return
	}
	l.emit(token.Ident, start, off)
}

func (l *lexer) number() {
	start, off := l.pos(), l.i
	base := 10
	label := ""
	switch {
	case l.peek(0) == '0' && (l.peek(1) == 'x' || l.peek(1) == 'X'):
		base, label = 16, "hex"
	case l.peek(0) == '0' && (l.peek(1) == 'o' || l.peek(1) == 'O'):
		base, label = 8, "octal"
	case l.peek(0) == '0' && (l.peek(1) == 'b' || l.peek(1) == 'B'):
		base, label = 2, "binary"
	}
	if base != 10 {
		l.advanceN(2)
		digits := 0
		for l.i < len(l.src) {
			c := l.src[l.i]
			if c == '_' {
				l.advanceN(1)
				continue
			}
			if !digitInBase(c, base) {
				break
			}
			digits++
			l.advanceN(1)
		}
		if digits == 0 {
			l.invalid(start, off, label+" literal has no digits")
			return
		}
		l.emit(token.Int, start, off)
		return
	}

	l.scanDigits()
	isFloat := false
	if l.i < len(l.src) && l.src[l.i] == '.' && l.peek(1) != '.' {
		isFloat = true
		l.advanceN(1)
		l.scanDigits()
	}
	if l.i < len(l.src) && (l.src[l.i] == 'e' || l.src[l.i] == 'E') {
		if n := l.exponentPrefix(); n > 0 {
			isFloat = true
			l.advanceN(n)
			l.scanDigits()
		}
	}
	if n := l.durationUnit(); n > 0 {
		l.advanceN(n)
		l.emit(token.Duration, start, off)
		return
	}
	if isFloat {
		l.emit(token.Float, start, off)
		return
	}
	l.emit(token.Int, start, off)
}

func (l *lexer) scanDigits() {
	for l.i < len(l.src) {
		c := l.src[l.i]
		if c == '_' || isDigit(c) {
			l.advanceN(1)
			continue
		}
		return
	}
}

func (l *lexer) exponentPrefix() int {
	j := l.i + 1
	if j < len(l.src) && (l.src[j] == '+' || l.src[j] == '-') {
		j++
	}
	if j >= len(l.src) || !isDigit(l.src[j]) {
		return 0
	}
	return j - l.i
}

func (l *lexer) durationUnit() int {
	units := [...]string{"ns", "us", "µs", "ms", "s", "m", "h"}
	for _, u := range units {
		if len(l.src)-l.i >= len(u) && string(l.src[l.i:l.i+len(u)]) == u {
			return len(u)
		}
	}
	return 0
}

func (l *lexer) quoted(kind token.Kind, quote byte) {
	start, off := l.pos(), l.i
	name := "string"
	if kind == token.Rune {
		name = "rune"
	}
	l.advanceN(1)
	for l.i < len(l.src) {
		c := l.src[l.i]
		if c == quote {
			l.advanceN(1)
			l.emit(kind, start, off)
			return
		}
		if c == '\n' || c == '\r' {
			l.errors = append(l.errors, Error{Pos: start, Msg: "newline in " + name + " literal"})
			l.emit(kind, start, off)
			return
		}
		if c == '\\' {
			l.advanceN(1)
			if l.i >= len(l.src) {
				l.errors = append(l.errors, Error{Pos: start, Msg: "unterminated " + name + " literal"})
				l.emit(kind, start, off)
				return
			}
			if c2 := l.src[l.i]; c2 == '\n' || c2 == '\r' {
				l.errors = append(l.errors, Error{Pos: start, Msg: "newline in " + name + " literal"})
				l.emit(kind, start, off)
				return
			}
			l.advance()
			continue
		}
		l.advance()
	}
	l.errors = append(l.errors, Error{Pos: start, Msg: "unterminated " + name + " literal"})
	l.emit(kind, start, off)
}

func (l *lexer) rawString() {
	start, off := l.pos(), l.i
	l.advanceN(1)
	for l.i < len(l.src) {
		if l.src[l.i] == '`' {
			l.advanceN(1)
			l.emit(token.RawString, start, off)
			return
		}
		l.advance()
	}
	l.errors = append(l.errors, Error{Pos: start, Msg: "unterminated raw string literal"})
	l.emit(token.RawString, start, off)
}

func (l *lexer) lineComment() {
	start, off := l.pos(), l.i
	l.advanceN(2)
	for l.i < len(l.src) && l.src[l.i] != '\n' && l.src[l.i] != '\r' {
		l.advance()
	}
	l.comments = append(l.comments, Comment{Pos: start, End: l.pos(), Text: string(l.src[off:l.i])})
}

func (l *lexer) blockComment() {
	start := l.pos()
	l.advanceN(2)
	for l.i < len(l.src) {
		if l.src[l.i] == '*' && l.peek(1) == '/' {
			l.advanceN(2)
			break
		}
		l.advance()
	}
	l.errors = append(l.errors, Error{Pos: start, Msg: "block comments are not supported; use //"})
}

func (l *lexer) symbol() {
	start, off := l.pos(), l.i
	c := l.src[l.i]
	n1, n2 := l.peek(1), l.peek(2)
	kind := token.Invalid
	size := 1

	switch c {
	case '<':
		switch {
		case n1 == '<' && n2 == '=':
			kind, size = token.ShlAssign, 3
		case n1 == '<':
			kind, size = token.Shl, 2
		case n1 == '=':
			kind, size = token.Le, 2
		case n1 == '-':
			l.reject(start, off, 2, "unsupported operator '<-'")
			return
		default:
			kind, size = token.Lt, 1
		}
	case '>':
		switch {
		case n1 == '>' && n2 == '=':
			kind, size = token.ShrAssign, 3
		case n1 == '>':
			kind, size = token.Shr, 2
		case n1 == '=':
			kind, size = token.Ge, 2
		default:
			kind, size = token.Gt, 1
		}
	case '=':
		switch n1 {
		case '=':
			kind, size = token.Eq, 2
		case '>':
			kind, size = token.Arrow, 2
		default:
			kind, size = token.Assign, 1
		}
	case '!':
		if n1 == '=' {
			kind, size = token.Ne, 2
		} else {
			kind, size = token.Bang, 1
		}
	case '+':
		switch n1 {
		case '+':
			l.reject(start, off, 2, "unsupported operator '++'")
			return
		case '=':
			kind, size = token.PlusAssign, 2
		default:
			kind, size = token.Plus, 1
		}
	case '-':
		switch n1 {
		case '-':
			l.reject(start, off, 2, "unsupported operator '--'")
			return
		case '=':
			kind, size = token.MinusAssign, 2
		default:
			kind, size = token.Minus, 1
		}
	case '.':
		switch {
		case n1 == '.' && n2 == '.':
			kind, size = token.Ellipsis, 3
		case n1 == '.':
			l.reject(start, off, 2, "unsupported operator '..'")
			return
		default:
			kind, size = token.Dot, 1
		}
	case '*':
		if n1 == '=' {
			kind, size = token.StarAssign, 2
		} else {
			kind, size = token.Star, 1
		}
	case '/':
		if n1 == '=' {
			kind, size = token.SlashAssign, 2
		} else {
			kind, size = token.Slash, 1
		}
	case '%':
		if n1 == '=' {
			kind, size = token.PercentAssign, 2
		} else {
			kind, size = token.Percent, 1
		}
	case '&':
		switch n1 {
		case '&':
			kind, size = token.AndAnd, 2
		case '=':
			kind, size = token.AmpAssign, 2
		default:
			kind, size = token.Amp, 1
		}
	case '|':
		switch n1 {
		case '|':
			kind, size = token.OrOr, 2
		case '=':
			kind, size = token.PipeAssign, 2
		default:
			kind, size = token.Pipe, 1
		}
	case '^':
		if n1 == '=' {
			kind, size = token.CaretAssign, 2
		} else {
			kind, size = token.Caret, 1
		}
	case '~':
		kind, size = token.Tilde, 1
	case ';':
		kind, size = token.Semi, 1
	case ',':
		kind, size = token.Comma, 1
	case ':':
		kind, size = token.Colon, 1
	case '(':
		kind, size = token.LParen, 1
	case ')':
		kind, size = token.RParen, 1
	case '{':
		kind, size = token.LBrace, 1
	case '}':
		kind, size = token.RBrace, 1
	case '[':
		kind, size = token.LBracket, 1
	case ']':
		kind, size = token.RBracket, 1
	default:
		r, n := utf8.DecodeRune(l.src[l.i:])
		if r == utf8.RuneError && n == 1 {
			l.reject(start, off, 1, fmt.Sprintf("invalid UTF-8 byte 0x%02x", c))
			return
		}
		l.reject(start, off, n, fmt.Sprintf("unexpected character %q", r))
		return
	}

	l.advanceN(size)
	l.emitKind(kind, start)
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func digitInBase(c byte, base int) bool {
	switch base {
	case 16:
		return isDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
	case 8:
		return c >= '0' && c <= '7'
	case 2:
		return c == '0' || c == '1'
	}
	return false
}
