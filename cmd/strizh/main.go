package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bongdo/strizh/internal/driver"
	"github.com/bongdo/strizh/internal/lsp"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	switch cmd {
	case "build":
		os.Exit(build(args))
	case "run":
		os.Exit(run(args))
	case "check":
		os.Exit(check(args))
	case "test":
		os.Exit(test(args))
	case "fmt":
		os.Exit(fmtCmd(args))
	case "lsp":
		os.Exit(serve(args))
	case "new":
		os.Exit(newCmd(args))
	case "init":
		os.Exit(initCmd(args))
	case "version", "-v", "--version":
		fmt.Println("strizh", driver.Version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `strizh — the Strizh compiler

usage:
  strizh build [-o output] [go build flags]
  strizh run [-- program args]
  strizh check
  strizh test [go test flags]
  strizh fmt [-w] [paths]
  strizh lsp [--root dir]
  strizh new <dir>
  strizh init [dir]
  strizh version
`)
}

func open() (*driver.Project, int) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "strizh:", err)
		return nil, 1
	}
	p, err := driver.Open(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "strizh:", err)
		return nil, 1
	}
	return p, 0
}

func finish(p *driver.Project) int {
	if len(p.Errors) == 0 {
		return 0
	}
	p.PrintErrors(os.Stderr)
	return 1
}

func build(args []string) int {
	out := ""
	var goArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-o" && i+1 < len(args) {
			out = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(a, "-o=") {
			out = strings.TrimPrefix(a, "-o=")
			continue
		}
		if a == "--" {
			goArgs = append(goArgs, args[i+1:]...)
			break
		}
		goArgs = append(goArgs, a)
	}
	p, code := open()
	if p == nil {
		return code
	}
	if err := p.Build(out, goArgs); err != nil {
		if err == driver.ErrCheck {
			p.PrintErrors(os.Stderr)
			return 1
		}
		fmt.Fprintln(os.Stderr, "strizh:", err)
		return 1
	}
	return finish(p)
}

func run(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p, code := open()
	if p == nil {
		return code
	}
	code, err := p.Run(fs.Args())
	if err != nil {
		if err == driver.ErrCheck {
			p.PrintErrors(os.Stderr)
			return 1
		}
		fmt.Fprintln(os.Stderr, "strizh:", err)
		return 1
	}
	if len(p.Errors) > 0 {
		p.PrintErrors(os.Stderr)
		return 1
	}
	return code
}

func check(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p, code := open()
	if p == nil {
		return code
	}
	if err := p.Check(driver.CheckOptions{Tidy: true, Typecheck: true}); err != nil {
		fmt.Fprintln(os.Stderr, "strizh:", err)
		return 1
	}
	return finish(p)
}

func test(args []string) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	p, code := open()
	if p == nil {
		return code
	}
	code, err := p.Test(args)
	if err != nil {
		if err == driver.ErrCheck {
			p.PrintErrors(os.Stderr)
			return 1
		}
		fmt.Fprintln(os.Stderr, "strizh:", err)
		return 1
	}
	if len(p.Errors) > 0 {
		p.PrintErrors(os.Stderr)
		return 1
	}
	return code
}

func fmtCmd(args []string) int {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	write := fs.Bool("w", false, "write result to source files")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p, code := open()
	if p == nil {
		return code
	}
	if err := p.Format(*write, fs.Args(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "strizh:", err)
		return 1
	}
	return 0
}

func serve(args []string) int {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	root := fs.String("root", "", "project root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := *root
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if err := lsp.Run(os.Stdin, os.Stdout, os.Stderr, dir); err != nil {
		fmt.Fprintln(os.Stderr, "strizh lsp:", err)
		return 1
	}
	return 0
}

func newCmd(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: strizh new <dir>")
		return 2
	}
	if err := driver.New(args[0]); err != nil {
		fmt.Fprintln(os.Stderr, "strizh:", err)
		return 1
	}
	fmt.Println("created", args[0])
	return 0
}

func initCmd(args []string) int {
	dir := "."
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "usage: strizh init [dir]")
		return 2
	}
	if len(args) == 1 {
		dir = args[0]
	}
	if err := driver.Init(dir); err != nil {
		fmt.Fprintln(os.Stderr, "strizh:", err)
		return 1
	}
	fmt.Println("initialized", dir)
	return 0
}
