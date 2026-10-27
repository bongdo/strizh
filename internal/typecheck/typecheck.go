package typecheck

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"

	"github.com/bongdo/strizh/internal/ast"
	"github.com/bongdo/strizh/internal/config"
	"github.com/bongdo/strizh/internal/gen"
	"github.com/bongdo/strizh/internal/sema"
	"github.com/bongdo/strizh/internal/token"
	strizhrt "github.com/bongdo/strizh/runtime"
)

type Options struct {
	Root   string
	Module string
	Tests  bool
}

type Error struct {
	Path string
	Pos  token.Pos
	Msg  string
}

var (
	posRE    = regexp.MustCompile(`^(.+?\.str):(\d+)(?::(\d+))?$`)
	msgPosRE = regexp.MustCompile(`^(.+?\.str):(\d+)(?::(\d+))?: (.+)$`)
)

// shortcut: local replace targets are not hashed, restart LSP after editing them
const cacheLimit = 16

var (
	cacheMu    sync.Mutex
	cache      = make(map[string][]Error)
	cacheOrder []string
)

func Check(p *sema.Program, files map[string][]*ast.File, opt Options) (out []Error) {
	defer func() {
		if r := recover(); r != nil {
			out = []Error{{Path: opt.Root, Msg: fmt.Sprintf("typecheck panic: %v", r)}}
		}
	}()
	module := moduleOf(opt)
	generated, errs := gen.Generate(p, files, gen.Options{Root: opt.Root, Module: module, IncludeTests: opt.Tests})
	if len(errs) > 0 {
		for _, err := range errs {
			out = append(out, Error{Path: opt.Root, Msg: err.Error()})
		}
		return out
	}
	if len(generated) == 0 {
		return nil
	}
	gomod, err := os.ReadFile(filepath.Join(opt.Root, "go.mod"))
	if err != nil {
		return []Error{{Path: opt.Root, Msg: "typecheck: " + err.Error()}}
	}
	gosum, _ := os.ReadFile(filepath.Join(opt.Root, "go.sum"))
	key := cacheKey(module, generated, gomod, gosum, opt.Tests)
	if errs, ok := cacheGet(key); ok {
		return errs
	}
	out = load(opt.Root, module, generated, knownFiles(files), opt.Tests)
	cachePut(key, out)
	return out
}

func knownFiles(files map[string][]*ast.File) map[string]bool {
	known := make(map[string]bool)
	for _, list := range files {
		for _, f := range list {
			if f == nil || f.Path == "" {
				continue
			}
			if abs, err := filepath.Abs(f.Path); err == nil {
				known[filepath.Clean(abs)] = true
			}
		}
	}
	return known
}

func cacheKey(module string, files []gen.File, gomod, gosum []byte, tests bool) string {
	h := sha256.New()
	io.WriteString(h, "strizh-typecheck-v1\x00")
	if tests {
		io.WriteString(h, "tests\x00")
	}
	io.WriteString(h, module)
	io.WriteString(h, "\x00")
	for _, f := range files {
		io.WriteString(h, f.Path)
		io.WriteString(h, "\x00")
		h.Write(f.Src)
		io.WriteString(h, "\x00")
	}
	h.Write(strizhrt.Source)
	io.WriteString(h, "\x00")
	h.Write(gomod)
	io.WriteString(h, "\x00")
	h.Write(gosum)
	return hex.EncodeToString(h.Sum(nil))
}

func moduleOf(opt Options) string {
	if mod, err := config.ReadModule(opt.Root); err == nil {
		return mod
	}
	return opt.Module
}

func cacheGet(key string) ([]Error, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	errs, ok := cache[key]
	if !ok {
		return nil, false
	}
	return append([]Error(nil), errs...), true
}

func cachePut(key string, errs []Error) {
	for _, e := range errs {
		if !strings.HasSuffix(e.Path, ".str") {
			return
		}
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if _, ok := cache[key]; ok {
		return
	}
	if len(cacheOrder) >= cacheLimit {
		delete(cache, cacheOrder[0])
		cacheOrder = cacheOrder[1:]
	}
	cache[key] = append([]Error(nil), errs...)
	cacheOrder = append(cacheOrder, key)
}

func load(root, module string, files []gen.File, known map[string]bool, tests bool) []Error {
	overlay := make(map[string][]byte, len(files)+1)
	importDirs := make(map[string]string, len(files))
	seen := make(map[string]bool)
	var patterns []string
	for _, f := range files {
		path, err := filepath.Abs(f.Path)
		if err != nil {
			return []Error{{Path: root, Msg: "typecheck: " + err.Error()}}
		}
		path = filepath.Clean(path)
		overlay[path] = f.Src
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		pattern := "."
		importPath := module
		if rel != "." {
			pattern = "./" + filepath.ToSlash(rel)
			importPath = module + "/" + filepath.ToSlash(rel)
		}
		importDirs[importPath] = filepath.Dir(path)
		if !seen[pattern] {
			seen[pattern] = true
			patterns = append(patterns, pattern)
		}
	}
	sort.Strings(patterns)
	if len(patterns) == 0 {
		return []Error{{Path: root, Msg: "typecheck: no generated packages"}}
	}
	overlay[filepath.Join(root, ".strizh", "runtime", "runtime.go")] = strizhrt.Source
	cfg := &packages.Config{
		Mode:    packages.LoadImports | packages.NeedExportFile,
		Dir:     root,
		Tests:   tests,
		Overlay: overlay,
		Env:     append(os.Environ(), "GOFLAGS=-buildvcs=false"),
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return []Error{{Path: root, Msg: "typecheck: " + err.Error()}}
	}
	if len(pkgs) == 0 {
		return []Error{{Path: root, Msg: "typecheck: no packages loaded"}}
	}

	var out []Error
	seenErr := make(map[string]bool)
	add := func(path string, line, col int, msg string) {
		key := path + "\x00" + strconv.Itoa(line) + "\x00" + strconv.Itoa(col) + "\x00" + msg
		if seenErr[key] {
			return
		}
		seenErr[key] = true
		out = append(out, Error{Path: path, Pos: token.Pos{Line: line, Col: col}, Msg: msg})
	}
	var other *packages.Error
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for i := range pkg.Errors {
			e := &pkg.Errors[i]
			if path, pos, ok := mapPos(root, known, e.Pos); ok {
				add(path, pos.Line, pos.Col, e.Msg)
				continue
			}
			if parseBundle(root, known, importDirs, e.Msg, add) {
				continue
			}
			if other == nil {
				other = e
			}
		}
	})
	if len(out) > 0 {
		return out
	}
	if other != nil {
		return []Error{{Path: root, Msg: "typecheck: " + other.Error()}}
	}
	return nil
}

func parseBundle(root string, known map[string]bool, importDirs map[string]string, msg string, add func(string, int, int, string)) bool {
	dir := root
	found := false
	for _, line := range strings.Split(msg, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			if d, ok := importDirs[strings.TrimSpace(strings.TrimPrefix(line, "# "))]; ok {
				dir = d
			}
			continue
		}
		m := msgPosRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		lineNo, err := strconv.Atoi(m[2])
		if err != nil || lineNo < 1 {
			continue
		}
		col := 1
		if m[3] != "" {
			if n, err := strconv.Atoi(m[3]); err == nil && n > 0 {
				col = n
			}
		}
		path, ok := resolvePath(root, known, dir, m[1])
		if !ok {
			continue
		}
		add(path, lineNo, col, m[4])
		found = true
	}
	return found
}

func mapPos(root string, known map[string]bool, s string) (string, token.Pos, bool) {
	m := posRE.FindStringSubmatch(s)
	if m == nil {
		return "", token.Pos{}, false
	}
	line, err := strconv.Atoi(m[2])
	if err != nil || line < 1 {
		return "", token.Pos{}, false
	}
	col := 1
	if m[3] != "" {
		if n, err := strconv.Atoi(m[3]); err == nil && n > 0 {
			col = n
		}
	}
	path, ok := resolvePath(root, known, root, m[1])
	if !ok {
		return "", token.Pos{}, false
	}
	return path, token.Pos{Line: line, Col: col}, true
}

func resolvePath(root string, known map[string]bool, base, path string) (string, bool) {
	if filepath.IsAbs(path) {
		path = filepath.Clean(path)
		if underRoot(root, path) {
			return path, true
		}
		return "", false
	}
	path = filepath.FromSlash(path)
	for _, dir := range []string{root, base} {
		candidate := filepath.Clean(filepath.Join(dir, path))
		if !underRoot(root, candidate) {
			continue
		}
		if known[candidate] {
			return candidate, true
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}
	return "", false
}

func underRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
