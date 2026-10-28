package lsp

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const validSrc = `type User = struct {
    Name: str,

    fn hello() str = {
        return "hi " + Name;
    };
};

fn greet(name: str) str = {
    return "hi " + name;
};

fn add(a: int, b: int) int = {
    return a + b;
};

fn main() void = {
    var total = 1;

    var n = add(1, 2);
    var u = User { Name = greet("x") };
    total = total + 1;
};
`

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type testClient struct {
	t      *testing.T
	w      *Conn
	in     *io.PipeWriter
	out    *io.PipeWriter
	outR   *io.PipeReader
	msgs   chan *Message
	notes  []*Message
	done   chan error
	exited bool
	stderr *syncBuffer
	id     int
}

func startClient(t *testing.T, root string) *testClient {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &testClient{
		t:      t,
		in:     inW,
		out:    outW,
		outR:   outR,
		msgs:   make(chan *Message, 256),
		done:   make(chan error, 1),
		stderr: &syncBuffer{},
	}
	c.w = NewConn(nil, inW)
	go c.readLoop(outR)
	go func() {
		c.done <- Run(inR, outW, c.stderr, root)
	}()
	t.Cleanup(func() {
		inW.Close()
		outW.Close()
		outR.Close()
		if c.exited {
			return
		}
		select {
		case <-c.done:
		case <-time.After(5 * time.Second):
			t.Errorf("server did not stop: %s", c.stderr.String())
		}
	})
	return c
}

func (c *testClient) readLoop(r io.Reader) {
	conn := NewConn(r, nil)
	for {
		msg, err := conn.Read()
		if err != nil {
			close(c.msgs)
			return
		}
		c.msgs <- msg
	}
}

func (c *testClient) recv() *Message {
	c.t.Helper()
	select {
	case msg, ok := <-c.msgs:
		if !ok {
			c.t.Fatalf("server output closed: %s", c.stderr.String())
		}
		return msg
	case <-time.After(15 * time.Second):
		c.t.Fatalf("timeout waiting for server message: %s", c.stderr.String())
	}
	return nil
}

func (c *testClient) request(method string, params any) *Message {
	c.t.Helper()
	msg := c.requestRaw(method, params)
	if msg.Error != nil {
		c.t.Fatalf("%s: %d %s", method, msg.Error.Code, msg.Error.Message)
	}
	return msg
}

func (c *testClient) requestRaw(method string, params any) *Message {
	c.t.Helper()
	c.id++
	raw, err := json.Marshal(params)
	if err != nil {
		c.t.Fatalf("marshal %s: %v", method, err)
	}
	if err := c.w.Write(&Message{ID: json.RawMessage(strconv.Itoa(c.id)), Method: method, Params: raw}); err != nil {
		c.t.Fatalf("write %s: %v", method, err)
	}
	for {
		msg := c.recv()
		if len(msg.ID) == 0 {
			c.notes = append(c.notes, msg)
			continue
		}
		return msg
	}
}

func (c *testClient) notify(method string, params any) {
	c.t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		c.t.Fatalf("marshal %s: %v", method, err)
	}
	if err := c.w.Write(&Message{Method: method, Params: raw}); err != nil {
		c.t.Fatalf("write %s: %v", method, err)
	}
}

func (c *testClient) waitDiagnostics(uri string, match func(*PublishDiagnosticsParams) bool) *PublishDiagnosticsParams {
	c.t.Helper()
	for {
		for i, msg := range c.notes {
			if msg.Method != "textDocument/publishDiagnostics" {
				continue
			}
			var params PublishDiagnosticsParams
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				continue
			}
			if params.URI != uri || !match(&params) {
				continue
			}
			c.notes = append(c.notes[:i], c.notes[i+1:]...)
			return &params
		}
		c.notes = append(c.notes, c.recv())
	}
}

func (c *testClient) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-c.done:
		c.exited = true
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("server did not exit: %s", c.stderr.String())
		return nil
	}
}

func newProject(t *testing.T, src string) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	config := []byte("name = \"hello\"\nmodule = \"example.com/hello\"\n")
	if err := os.WriteFile(filepath.Join(root, "strizh.toml"), config, 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "main.str")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, path, pathToURI(path)
}

func hasLabel(items []CompletionItem, label string) bool {
	for _, item := range items {
		if item.Label == label {
			return true
		}
	}
	return false
}

func applyEdit(src string, edit TextEdit) string {
	data := []byte(src)
	start := OffsetAt(data, edit.Range.Start)
	end := OffsetAt(data, edit.Range.End)
	return string(data[:start]) + edit.NewText + string(data[end:])
}

func TestInitializeCapabilities(t *testing.T) {
	root, _, uri := newProject(t, validSrc)
	c := startClient(t, "")
	msg := c.request("initialize", InitializeParams{
		RootURI:          pathToURI(root),
		WorkspaceFolders: []WorkspaceFolder{{URI: pathToURI(root), Name: "hello"}},
	})
	var result InitializeResult
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		t.Fatal(err)
	}
	caps := result.Capabilities
	if caps.TextDocumentSync != 1 {
		t.Fatalf("textDocumentSync = %d", caps.TextDocumentSync)
	}
	if !caps.HoverProvider || !caps.DefinitionProvider || !caps.DocumentSymbolProvider || !caps.DocumentFormattingProvider {
		t.Fatalf("missing capabilities: %+v", caps)
	}
	if caps.CompletionProvider == nil || !hasString(caps.CompletionProvider.TriggerCharacters, ".") || !hasString(caps.CompletionProvider.TriggerCharacters, ":") {
		t.Fatalf("completion provider: %+v", caps.CompletionProvider)
	}
	if caps.SignatureHelpProvider == nil || !hasString(caps.SignatureHelpProvider.TriggerCharacters, "(") || !hasString(caps.SignatureHelpProvider.TriggerCharacters, ",") {
		t.Fatalf("signature help provider: %+v", caps.SignatureHelpProvider)
	}
	if result.ServerInfo.Name != "strizh" || result.ServerInfo.Version != "0.2.0" {
		t.Fatalf("server info: %+v", result.ServerInfo)
	}
	diags := c.waitDiagnostics(uri, func(*PublishDiagnosticsParams) bool { return true })
	if len(diags.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags.Diagnostics)
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestDiagnostics(t *testing.T) {
	root, _, uri := newProject(t, validSrc)
	invalidSrc := strings.Replace(validSrc, "var total = 1;", "var total = unknown;", 1)
	c := startClient(t, root)
	c.request("initialize", InitializeParams{})
	c.notify("textDocument/didOpen", DidOpenParams{
		TextDocument: TextDocumentItem{URI: uri, LanguageID: "strizh", Version: 1, Text: invalidSrc},
	})
	diags := c.waitDiagnostics(uri, func(params *PublishDiagnosticsParams) bool {
		if params.Version == nil || *params.Version != 1 {
			return false
		}
		for _, diag := range params.Diagnostics {
			if strings.Contains(diag.Message, "undefined: unknown") {
				return true
			}
		}
		return false
	})
	var found *Diagnostic
	for i := range diags.Diagnostics {
		if strings.Contains(diags.Diagnostics[i].Message, "undefined: unknown") {
			found = &diags.Diagnostics[i]
		}
	}
	if found == nil {
		t.Fatalf("missing diagnostic: %+v", diags.Diagnostics)
	}
	if found.Severity != 1 || found.Source != "strizh" {
		t.Fatalf("diagnostic: %+v", found)
	}
	if found.Range.Start.Line == 0 && found.Range.Start.Character == 0 {
		t.Fatalf("diagnostic range not set: %+v", found.Range)
	}
	c.notify("textDocument/didChange", DidChangeParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []TextDocumentChange{{Text: validSrc}},
	})
	diags = c.waitDiagnostics(uri, func(params *PublishDiagnosticsParams) bool {
		return params.Version != nil && *params.Version == 2
	})
	if len(diags.Diagnostics) != 0 {
		t.Fatalf("diagnostics after full change: %+v", diags.Diagnostics)
	}
	src := []byte(validSrc)
	start := strings.Index(validSrc, "var total = 1;") + len("var total = ")
	end := start + 1
	c.notify("textDocument/didChange", DidChangeParams{
		TextDocument: VersionedTextDocumentIdentifier{URI: uri, Version: 3},
		ContentChanges: []TextDocumentChange{{
			Range: &Range{Start: PositionAt(src, start), End: PositionAt(src, end)},
			Text:  "unknown",
		}},
	})
	c.waitDiagnostics(uri, func(params *PublishDiagnosticsParams) bool {
		if params.Version == nil || *params.Version != 3 {
			return false
		}
		for _, diag := range params.Diagnostics {
			if strings.Contains(diag.Message, "undefined: unknown") {
				return true
			}
		}
		return false
	})
}

func TestTypeDiagnostics(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"strizh.toml": "name = \"hello\"\nmodule = \"example.com/hello\"\n",
		"go.mod":      "module example.com/hello\n\ngo 1.27\n",
	}
	src := "use fmt;\n\nfn main() void or error = {\n    var n = 1 + \"x\";\n    fmt.Println(n) or return;\n};\n"
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "main.str")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := pathToURI(path)
	c := startClient(t, root)
	c.request("initialize", InitializeParams{})
	c.notify("textDocument/didOpen", DidOpenParams{
		TextDocument: TextDocumentItem{URI: uri, LanguageID: "strizh", Version: 1, Text: src},
	})
	diags := c.waitDiagnostics(uri, func(params *PublishDiagnosticsParams) bool {
		for _, diag := range params.Diagnostics {
			if strings.Contains(diag.Message, "mismatched types") {
				return true
			}
		}
		return false
	})
	var found *Diagnostic
	for i := range diags.Diagnostics {
		if strings.Contains(diags.Diagnostics[i].Message, "mismatched types") {
			found = &diags.Diagnostics[i]
		}
	}
	if found == nil {
		t.Fatalf("missing type diagnostic: %+v", diags.Diagnostics)
	}
	if found.Range.Start.Line != 3 {
		t.Fatalf("diagnostic line: %+v", found.Range)
	}
	c.notify("textDocument/didChange", DidChangeParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []TextDocumentChange{{Text: src}},
	})
	changed := c.waitDiagnostics(uri, func(params *PublishDiagnosticsParams) bool {
		return params.Version != nil && *params.Version == 2
	})
	for _, diag := range changed.Diagnostics {
		if strings.Contains(diag.Message, "mismatched types") {
			t.Fatalf("type diagnostic not cleared on didChange: %+v", changed.Diagnostics)
		}
	}
	c.notify("textDocument/didSave", DidSaveParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
	diags = c.waitDiagnostics(uri, func(params *PublishDiagnosticsParams) bool {
		if params.Version == nil || *params.Version != 2 {
			return false
		}
		for _, diag := range params.Diagnostics {
			if strings.Contains(diag.Message, "mismatched types") {
				return true
			}
		}
		return false
	})
	if len(diags.Diagnostics) == 0 {
		t.Fatal("type diagnostics lost after save")
	}
}

func TestFeatures(t *testing.T) {
	root, _, uri := newProject(t, validSrc)
	c := startClient(t, root)
	c.request("initialize", InitializeParams{})
	c.notify("textDocument/didOpen", DidOpenParams{
		TextDocument: TextDocumentItem{URI: uri, LanguageID: "strizh", Version: 1, Text: validSrc},
	})
	openDiags := c.waitDiagnostics(uri, func(params *PublishDiagnosticsParams) bool {
		return params.Version != nil && *params.Version == 1
	})
	if len(openDiags.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", openDiags.Diagnostics)
	}
	src := []byte(validSrc)
	totalPos := PositionAt(src, strings.Index(validSrc, "total + 1")+1)
	hover := c.request("textDocument/hover", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     totalPos,
	})
	var hoverResult Hover
	if err := json.Unmarshal(hover.Result, &hoverResult); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hoverResult.Contents.Value, "total") || !strings.Contains(hoverResult.Contents.Value, "int") {
		t.Fatalf("hover: %+v", hoverResult)
	}
	greetPos := PositionAt(src, strings.Index(validSrc, `greet("x")`)+1)
	hover = c.request("textDocument/hover", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     greetPos,
	})
	if err := json.Unmarshal(hover.Result, &hoverResult); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hoverResult.Contents.Value, "fn greet") {
		t.Fatalf("function hover: %+v", hoverResult)
	}
	def := c.request("textDocument/definition", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     totalPos,
	})
	var location Location
	if err := json.Unmarshal(def.Result, &location); err != nil {
		t.Fatal(err)
	}
	if location.URI != uri {
		t.Fatalf("definition URI: %q", location.URI)
	}
	want := PositionAt(src, strings.Index(validSrc, "var total"))
	if location.Range.Start != want {
		t.Fatalf("definition start = %+v, want %+v", location.Range.Start, want)
	}
	blank := strings.Index(validSrc, "\n\n    var n") + 1
	completion := c.request("textDocument/completion", CompletionParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     PositionAt(src, blank),
		},
	})
	var list CompletionList
	if err := json.Unmarshal(completion.Result, &list); err != nil {
		t.Fatal(err)
	}
	if !hasLabel(list.Items, "total") || !hasLabel(list.Items, "fn") {
		t.Fatalf("completion items: %+v", list.Items)
	}
	method := c.request("textDocument/completion", CompletionParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     PositionAt(src, strings.Index(validSrc, `return "hi " + Name;`)),
		},
	})
	if err := json.Unmarshal(method.Result, &list); err != nil {
		t.Fatal(err)
	}
	if !hasLabel(list.Items, "Name") {
		t.Fatalf("method completion items: %+v", list.Items)
	}
	sig := c.request("textDocument/signatureHelp", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     PositionAt(src, strings.Index(validSrc, `greet("x")`)+len(`greet("`)),
	})
	var help SignatureHelp
	if err := json.Unmarshal(sig.Result, &help); err != nil {
		t.Fatal(err)
	}
	if len(help.Signatures) != 1 || !strings.Contains(help.Signatures[0].Label, "greet") {
		t.Fatalf("signature help: %+v", help)
	}
	if help.ActiveParameter != 0 {
		t.Fatalf("active parameter = %d", help.ActiveParameter)
	}
	addPos := strings.Index(validSrc, "add(1, 2)") + len("add(1, ")
	sig = c.request("textDocument/signatureHelp", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     PositionAt(src, addPos),
	})
	if err := json.Unmarshal(sig.Result, &help); err != nil {
		t.Fatal(err)
	}
	if len(help.Signatures) != 1 || !strings.Contains(help.Signatures[0].Label, "add") {
		t.Fatalf("signature help: %+v", help)
	}
	if help.ActiveParameter != 1 {
		t.Fatalf("active parameter = %d", help.ActiveParameter)
	}
	formatted := c.request("textDocument/formatting", FormattingParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Options:      FormattingOptions{TabSize: 4, InsertSpaces: true},
	})
	var edits []TextEdit
	if err := json.Unmarshal(formatted.Result, &edits); err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("edits: %+v", edits)
	}
	first := applyEdit(validSrc, edits[0])
	c.notify("textDocument/didChange", DidChangeParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []TextDocumentChange{{Text: first}},
	})
	c.waitDiagnostics(uri, func(params *PublishDiagnosticsParams) bool {
		return params.Version != nil && *params.Version == 2
	})
	formatted = c.request("textDocument/formatting", FormattingParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Options:      FormattingOptions{TabSize: 4, InsertSpaces: true},
	})
	if err := json.Unmarshal(formatted.Result, &edits); err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 || edits[0].NewText != first {
		t.Fatalf("formatting is not idempotent: %+v", edits)
	}
	symbols := c.request("textDocument/documentSymbol", DocumentSymbolParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
	var docs []DocumentSymbol
	if err := json.Unmarshal(symbols.Result, &docs); err != nil {
		t.Fatal(err)
	}
	kinds := make(map[string]int)
	for _, symbol := range docs {
		kinds[symbol.Name] = symbol.Kind
	}
	if kinds["User"] != dskStruct || kinds["main"] != dskFunction {
		t.Fatalf("document symbols: %+v", docs)
	}
}

func TestGoImportCompletion(t *testing.T) {
	src := `use fmt;

fn main() void = {
    fmt.Println("hi");
};
`
	root, _, uri := newProject(t, src)
	module := "module example.com/hello\n\ngo 1.27\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	c := startClient(t, root)
	c.request("initialize", InitializeParams{})
	c.notify("textDocument/didOpen", DidOpenParams{
		TextDocument: TextDocumentItem{URI: uri, LanguageID: "strizh", Version: 1, Text: src},
	})
	diags := c.waitDiagnostics(uri, func(params *PublishDiagnosticsParams) bool {
		return params.Version != nil && *params.Version == 1
	})
	if len(diags.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags.Diagnostics)
	}
	data := []byte(src)
	dot := strings.Index(src, "fmt.") + len("fmt.")
	msg := c.request("textDocument/completion", CompletionParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     PositionAt(data, dot),
		},
	})
	var list CompletionList
	if err := json.Unmarshal(msg.Result, &list); err != nil {
		t.Fatal(err)
	}
	if !hasLabel(list.Items, "Println") {
		t.Fatalf("completion items: %+v", list.Items)
	}
	arg := strings.Index(src, `Println("hi")`) + len(`Println("`)
	sig := c.request("textDocument/signatureHelp", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     PositionAt(data, arg),
	})
	var help SignatureHelp
	if err := json.Unmarshal(sig.Result, &help); err != nil {
		t.Fatal(err)
	}
	if len(help.Signatures) != 1 || !strings.Contains(help.Signatures[0].Label, "Println") {
		t.Fatalf("signature help: %+v", help)
	}
	if help.ActiveParameter != 0 {
		t.Fatalf("active parameter = %d", help.ActiveParameter)
	}
}

func TestParseErrorFormatting(t *testing.T) {
	broken := "fn main( {\n"
	root, _, uri := newProject(t, validSrc)
	c := startClient(t, root)
	c.request("initialize", InitializeParams{})
	c.notify("textDocument/didOpen", DidOpenParams{
		TextDocument: TextDocumentItem{URI: uri, LanguageID: "strizh", Version: 1, Text: broken},
	})
	diags := c.waitDiagnostics(uri, func(params *PublishDiagnosticsParams) bool {
		if params.Version == nil || *params.Version != 1 {
			return false
		}
		for _, diag := range params.Diagnostics {
			if diag.Severity == 1 && diag.Source == "strizh" {
				return true
			}
		}
		return false
	})
	if len(diags.Diagnostics) == 0 {
		t.Fatal("missing parse diagnostic")
	}
	msg := c.request("textDocument/formatting", FormattingParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Options:      FormattingOptions{TabSize: 4, InsertSpaces: true},
	})
	var edits []TextEdit
	if err := json.Unmarshal(msg.Result, &edits); err != nil {
		t.Fatal(err)
	}
	if len(edits) != 0 {
		t.Fatalf("edits for broken source: %+v", edits)
	}
}

func TestLocalImport(t *testing.T) {
	mainSrc := `use "./lib" as lib;

fn main() void = {
    var s = lib.Hello();
};
`
	root, _, mainURI := newProject(t, mainSrc)
	libDir := filepath.Join(root, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	libSrc := "fn Hello() str = {\n    return \"hi\";\n};\n"
	libPath := filepath.Join(libDir, "lib.str")
	if err := os.WriteFile(libPath, []byte(libSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	libURI := pathToURI(libPath)
	c := startClient(t, root)
	c.request("initialize", InitializeParams{})
	c.notify("textDocument/didOpen", DidOpenParams{
		TextDocument: TextDocumentItem{URI: mainURI, LanguageID: "strizh", Version: 1, Text: mainSrc},
	})
	diags := c.waitDiagnostics(mainURI, func(params *PublishDiagnosticsParams) bool {
		return params.Version != nil && *params.Version == 1
	})
	if len(diags.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags.Diagnostics)
	}
	mainData := []byte(mainSrc)
	dot := strings.Index(mainSrc, "lib.") + len("lib.")
	completion := c.request("textDocument/completion", CompletionParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: mainURI},
			Position:     PositionAt(mainData, dot),
		},
	})
	var list CompletionList
	if err := json.Unmarshal(completion.Result, &list); err != nil {
		t.Fatal(err)
	}
	if !hasLabel(list.Items, "Hello") {
		t.Fatalf("completion items: %+v", list.Items)
	}
	def := c.request("textDocument/definition", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: mainURI},
		Position:     PositionAt(mainData, strings.Index(mainSrc, "Hello")+1),
	})
	var location Location
	if err := json.Unmarshal(def.Result, &location); err != nil {
		t.Fatal(err)
	}
	if location.URI != libURI {
		t.Fatalf("definition URI = %q, want %q", location.URI, libURI)
	}
	want := PositionAt([]byte(libSrc), strings.Index(libSrc, "fn Hello"))
	if location.Range.Start != want {
		t.Fatalf("definition start = %+v, want %+v", location.Range.Start, want)
	}
}

func TestShutdownExit(t *testing.T) {
	c := startClient(t, "")
	c.request("initialize", InitializeParams{})
	msg := c.request("shutdown", nil)
	if string(msg.Result) != "null" {
		t.Fatalf("shutdown result = %s", msg.Result)
	}
	c.notify("exit", nil)
	if err := c.wait(t); err != nil {
		t.Fatal(err)
	}
}

func TestErrors(t *testing.T) {
	c := startClient(t, "")
	c.request("initialize", InitializeParams{})
	msg := c.requestRaw("textDocument/unknown", nil)
	if msg.Error == nil || msg.Error.Code != -32601 {
		t.Fatalf("unknown method response: %+v", msg)
	}
	msg = c.requestRaw("textDocument/hover", 1)
	if msg.Error == nil || msg.Error.Code != -32602 {
		t.Fatalf("invalid params response: %+v", msg)
	}
	c.request("shutdown", nil)
	c.notify("exit", nil)
	if err := c.wait(t); err != nil {
		t.Fatal(err)
	}
}

func TestApplyChanges(t *testing.T) {
	src := "fn main() void = {\n    var x = 1;\n};\n"
	full := applyChanges(src, []TextDocumentChange{{Text: "replacement"}})
	if full != "replacement" {
		t.Fatalf("full change = %q", full)
	}
	data := []byte(src)
	start := strings.Index(src, "1")
	end := start + 1
	changed := applyChanges(src, []TextDocumentChange{{
		Range: &Range{Start: PositionAt(data, start), End: PositionAt(data, end)},
		Text:  "unknown",
	}})
	if !strings.Contains(changed, "var x = unknown;") {
		t.Fatalf("incremental change = %q", changed)
	}
}
