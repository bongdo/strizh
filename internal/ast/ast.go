package ast

import "github.com/bongdo/strizh/internal/token"

type Node interface {
	Pos() token.Pos
	EndPos() token.Pos
}

type Span struct {
	Start token.Pos
	End   token.Pos
}

func (s Span) Pos() token.Pos {
	return s.Start
}

func (s Span) EndPos() token.Pos {
	return s.End
}

type File struct {
	Span
	Path     string
	Src      []byte
	Uses     []*Use
	Decls    []Decl
	Comments []*Comment
}

type Comment struct {
	Span
	Text  string
	Block bool
}

type Decl interface {
	Node
	declNode()
}

type Use struct {
	Span
	Alias        string
	Path         string
	Local        bool
	GoPath       string
	PackageName  string
	LocalDir     string
	LocalPackage string
	Used         bool
}

func (*Use) declNode() {}

type FuncDecl struct {
	Span
	Name       string
	TypeParams []*TypeParam
	Params     []*Param
	Results    []Type
	Err        bool
	Body       *Block
	Owner      *TypeDecl
	NeedsCtx   bool
	GoName     string
}

func (*FuncDecl) declNode() {}

type Param struct {
	Span
	Name string
	Type Type
}

type TypeParam struct {
	Span
	Name string
}

type TypeKind int

const (
	TypeAlias TypeKind = iota
	TypeStruct
	TypeEnum
	TypeInterface
)

type TypeDecl struct {
	Span
	Name       string
	TypeParams []*TypeParam
	Kind       TypeKind
	Target     Type
	Fields     []*Field
	Methods    []*FuncDecl
	Variants   []*Variant
	IFace      []*IFaceMethod
	GoName     string
}

func (*TypeDecl) declNode() {}

type Field struct {
	Span
	Name string
	Type Type
}

type Variant struct {
	Span
	Name   string
	Fields []*Field
	Enum   *TypeDecl
	GoName string
}

type IFaceMethod struct {
	Span
	Name     string
	Params   []*Param
	Results  []Type
	Err      bool
	NeedsCtx bool
}

type VarDecl struct {
	Span
	Names  []string
	Values []Expr
	Const  bool
}

func (*VarDecl) declNode() {}

type TestDecl struct {
	Span
	Name string
	Body *Block
	GoID string
}

func (*TestDecl) declNode() {}

type Type interface {
	Node
	typeNode()
}

type NamedType struct {
	Span
	Package  string
	Name     string
	Decl     *TypeDecl
	Variant  *Variant
	GoName   string
	PkgAlias string
}

func (*NamedType) typeNode() {}

type SliceType struct {
	Span
	Elem Type
}

func (*SliceType) typeNode() {}

type ArrayType struct {
	Span
	Elem Type
	Len  Expr
}

func (*ArrayType) typeNode() {}

type GenericType struct {
	Span
	Base Type
	Args []Type
}

func (*GenericType) typeNode() {}

type MapType struct {
	Span
	Key   Type
	Value Type
}

func (*MapType) typeNode() {}

type PointerType struct {
	Span
	Elem Type
}

func (*PointerType) typeNode() {}

type FuncType struct {
	Span
	Params   []Type
	Results  []Type
	Err      bool
	NeedsCtx bool
}

func (*FuncType) typeNode() {}

type InterfaceType struct {
	Span
	Methods []*IFaceMethod
}

func (*InterfaceType) typeNode() {}

type StructType struct {
	Span
	Fields []*Field
}

func (*StructType) typeNode() {}

type GoType struct {
	Span
	Text string
}

func (*GoType) typeNode() {}

type Expr interface {
	Node
	exprNode()
}

type RefKind int

const (
	RefNone RefKind = iota
	RefLocal
	RefParam
	RefGlobal
	RefConst
	RefFunc
	RefType
	RefField
	RefSelf
	RefBuiltin
	RefEnumVariant
	RefPackage
)

type Ref struct {
	Kind    RefKind
	Name    string
	Field   *Field
	Variant *Variant
	Func    *FuncDecl
	Type    *TypeDecl
	TypeVal Type
	Decl    Node
	Path    string
	Use     *Use
	Go      any
}

type Ident struct {
	Span
	Name string
	Ref  *Ref
}

func (*Ident) exprNode() {}

type SelectorExpr struct {
	Span
	X    Expr
	Name string
	Ref  *Ref
}

func (*SelectorExpr) exprNode() {}

type CallKind int

const (
	CallUnknown CallKind = iota
	CallFunc
	CallMethod
	CallBuiltin
	CallConversion
	CallType
)

type CallInfo struct {
	Kind        CallKind
	Results     int
	ResultTypes []Type
	HasError    bool
	NeedsCtx    bool
	ParamTypes  []Type
	Builtin     string
	Fn          *FuncDecl
}

type CallExpr struct {
	Span
	Fun      Expr
	TypeArgs []Type
	Args     []Expr
	Ellipsis bool
	Info     *CallInfo
}

func (*CallExpr) exprNode() {}

type IndexExpr struct {
	Span
	X     Expr
	Index Expr
}

func (*IndexExpr) exprNode() {}

type SliceExpr struct {
	Span
	X    Expr
	Low  Expr
	High Expr
}

func (*SliceExpr) exprNode() {}

type UnaryExpr struct {
	Span
	Op token.Kind
	X  Expr
}

func (*UnaryExpr) exprNode() {}

type BinaryExpr struct {
	Span
	Op token.Kind
	X  Expr
	Y  Expr
}

func (*BinaryExpr) exprNode() {}

type ParenExpr struct {
	Span
	X Expr
}

func (*ParenExpr) exprNode() {}

type BasicLit struct {
	Span
	Kind  token.Kind
	Value string
}

func (*BasicLit) exprNode() {}

type CompositeLit struct {
	Span
	Type    Type
	Entries []*CompositeEntry
	Variant *Variant
}

func (*CompositeLit) exprNode() {}

type CompositeEntry struct {
	Span
	Key     string
	KeyExpr Expr
	Value   Expr
	Field   *Field
}

type FuncLit struct {
	Span
	Params     []*Param
	Results    []Type
	Err        bool
	Body       *Block
	Arrow      Expr
	ParamTypes []Type
	NeedsCtx   bool
	GoCallback bool
}

func (*FuncLit) exprNode() {}

type NilLit struct {
	Span
	Type Type
}

func (*NilLit) exprNode() {}

type MatchExpr struct {
	Span
	Subject     Expr
	Cases       []*Case
	ResultType  Type
	IsExpr      bool
	SubjectType Type
	TypeSwitch  bool
}

func (*MatchExpr) exprNode() {}

type OrMode int

const (
	OrReturn OrMode = iota
	OrPanic
	OrHandle
)

type OrExpr struct {
	Span
	X        Expr
	Mode     OrMode
	Name     string
	Block    *Block
	PanicArg Expr
}

func (*OrExpr) exprNode() {}

type Case struct {
	Span
	Pat     *Pattern
	Body    []Stmt
	Expr    Expr
	Block   *Block
	Default bool
}

type Pattern struct {
	Span
	Wildcard   bool
	Qualifier  string
	Name       string
	Bind       string
	TypePat    Type
	Fields     []string
	Literal    Expr
	Variant    *Variant
	FieldDecls []*Field
}

type Stmt interface {
	Node
	stmtNode()
}

type Block struct {
	Span
	Stmts []Stmt
}

func (*Block) stmtNode() {}

type EmptyStmt struct {
	Span
}

func (*EmptyStmt) stmtNode() {}

type DeclStmt struct {
	Span
	Decl *VarDecl
}

func (*DeclStmt) stmtNode() {}

type AssignStmt struct {
	Span
	Lhs []Expr
	Op  token.Kind
	Rhs []Expr
}

func (*AssignStmt) stmtNode() {}

type ExprStmt struct {
	Span
	X Expr
}

func (*ExprStmt) stmtNode() {}

type IfStmt struct {
	Span
	Cond Expr
	Then *Block
	Else Stmt
}

func (*IfStmt) stmtNode() {}

type RangeClause struct {
	Span
	Names    []string
	X        Expr
	Known    bool
	IsMap    bool
	KeyType  Type
	ElemType Type
}

type ForStmt struct {
	Span
	Init  Stmt
	Cond  Expr
	Post  Stmt
	Range *RangeClause
	Body  *Block
	Par   bool
	Limit Expr
}

func (*ForStmt) stmtNode() {}

type ReturnStmt struct {
	Span
	Results []Expr
}

func (*ReturnStmt) stmtNode() {}

type BreakStmt struct {
	Span
}

func (*BreakStmt) stmtNode() {}

type ContinueStmt struct {
	Span
}

func (*ContinueStmt) stmtNode() {}

type DeferStmt struct {
	Span
	Call *CallExpr
}

func (*DeferStmt) stmtNode() {}

type ParStmt struct {
	Span
	Body *Block
}

func (*ParStmt) stmtNode() {}

type WithinStmt struct {
	Span
	Duration Expr
	Body     *Block
}

func (*WithinStmt) stmtNode() {}
