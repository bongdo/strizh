package token

type Kind int

const (
	Invalid Kind = iota
	EOF
	Ident
	Int
	Float
	String
	RawString
	Rune
	Duration

	Semi
	Comma
	Dot
	Colon
	LParen
	RParen
	LBrace
	RBrace
	LBracket
	RBracket

	Assign
	Eq
	Ne
	Lt
	Le
	Gt
	Ge

	Plus
	Minus
	Star
	Slash
	Percent
	Amp
	Pipe
	Caret
	Tilde
	Shl
	Shr
	Bang
	AndAnd
	OrOr

	PlusAssign
	MinusAssign
	StarAssign
	SlashAssign
	PercentAssign
	AmpAssign
	PipeAssign
	CaretAssign
	ShlAssign
	ShrAssign

	Ellipsis
	Arrow

	Use
	As
	Fn
	Type
	Struct
	Enum
	Interface
	Var
	Const
	Return
	If
	Else
	For
	In
	Match
	Case
	Par
	Within
	Limit
	Defer
	Break
	Continue
	Or
	Test
	True
	False
	Nil
	Map
)

var names = map[Kind]string{
	Invalid:       "invalid",
	EOF:           "end of file",
	Ident:         "identifier",
	Int:           "int",
	Float:         "float",
	String:        "string",
	RawString:     "raw string",
	Rune:          "rune",
	Duration:      "duration",
	Semi:          ";",
	Comma:         ",",
	Dot:           ".",
	Colon:         ":",
	LParen:        "(",
	RParen:        ")",
	LBrace:        "{",
	RBrace:        "}",
	LBracket:      "[",
	RBracket:      "]",
	Assign:        "=",
	Eq:            "==",
	Ne:            "!=",
	Lt:            "<",
	Le:            "<=",
	Gt:            ">",
	Ge:            ">=",
	Plus:          "+",
	Minus:         "-",
	Star:          "*",
	Slash:         "/",
	Percent:       "%",
	Amp:           "&",
	Pipe:          "|",
	Caret:         "^",
	Tilde:         "~",
	Shl:           "<<",
	Shr:           ">>",
	Bang:          "!",
	AndAnd:        "&&",
	OrOr:          "||",
	PlusAssign:    "+=",
	MinusAssign:   "-=",
	StarAssign:    "*=",
	SlashAssign:   "/=",
	PercentAssign: "%=",
	AmpAssign:     "&=",
	PipeAssign:    "|=",
	CaretAssign:   "^=",
	ShlAssign:     "<<=",
	ShrAssign:     ">>=",
	Ellipsis:      "...",
	Arrow:         "=>",
	Use:           "use",
	As:            "as",
	Fn:            "fn",
	Type:          "type",
	Struct:        "struct",
	Enum:          "enum",
	Interface:     "interface",
	Var:           "var",
	Const:         "const",
	Return:        "return",
	If:            "if",
	Else:          "else",
	For:           "for",
	In:            "in",
	Match:         "match",
	Case:          "case",
	Par:           "par",
	Within:        "within",
	Limit:         "limit",
	Defer:         "defer",
	Break:         "break",
	Continue:      "continue",
	Or:            "or",
	Test:          "test",
	True:          "true",
	False:         "false",
	Nil:           "nil",
	Map:           "map",
}

func (k Kind) String() string {
	if s, ok := names[k]; ok {
		return s
	}
	return "unknown"
}

var keywords = map[string]Kind{}

func init() {
	for k, s := range names {
		if k >= Use {
			keywords[s] = k
		}
	}
}

func Lookup(ident string) Kind {
	if k, ok := keywords[ident]; ok {
		return k
	}
	return Ident
}

var goReserved = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

func GoReserved(name string) bool {
	return goReserved[name]
}

type Pos struct {
	Offset int
	Line   int
	Col    int
}

func (p Pos) Valid() bool {
	return p.Line > 0
}

type Token struct {
	Kind Kind
	Lit  string
	Pos  Pos
	End  Pos
}
