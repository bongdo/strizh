package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

const FileName = "strizh.toml"

type Config struct {
	Name   string `toml:"name"`
	Module string `toml:"module"`
	Go     string `toml:"go"`
}

var nameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

func FindRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, FileName)); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("%s not found in %s or above", FileName, dir)
		}
		abs = parent
	}
}

func Load(root string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(filepath.Join(root, FileName), &cfg); err != nil {
		return nil, err
	}
	if cfg.Name == "" {
		cfg.Name = filepath.Base(root)
	}
	if !nameRE.MatchString(cfg.Name) {
		return nil, fmt.Errorf("invalid project name %q", cfg.Name)
	}
	if cfg.Module == "" {
		cfg.Module = "example.com/" + cfg.Name
	}
	if strings.ContainsAny(cfg.Module, " \t\n") {
		return nil, fmt.Errorf("invalid module path %q", cfg.Module)
	}
	if cfg.Go == "" {
		cfg.Go = "1.27"
	}
	return &cfg, nil
}

func Save(root string, cfg *Config) error {
	var b strings.Builder
	fmt.Fprintf(&b, "name = %q\n", cfg.Name)
	fmt.Fprintf(&b, "module = %q\n", cfg.Module)
	fmt.Fprintf(&b, "go = %q\n", cfg.Go)
	return os.WriteFile(filepath.Join(root, FileName), []byte(b.String()), 0o644)
}

func ReadModule(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", errors.New("module line not found in go.mod")
}
