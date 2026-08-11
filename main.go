package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/waldman/twig/internal/config"
	"github.com/waldman/twig/internal/generate"
	"github.com/waldman/twig/internal/leaf"
	"github.com/waldman/twig/internal/pathparse"
	"github.com/waldman/twig/internal/runner"
)

const usage = `twig — thin Terraform leaf runner

Usage:
  twig <command> <leaf-file> [-- <terraform-flags>...]

Commands:
  init     Generate main.tf and run terraform init
  plan     Generate main.tf, auto-init if needed, run terraform plan
  apply    Generate main.tf, auto-init if needed, run terraform apply
  destroy  Generate main.tf, auto-init if needed, run terraform destroy
  show     Print generated main.tf to stdout (no terraform)

The leaf file must be a .yaml file at:
  infra/<provider>/<profile>/<region>/<environment>/<class>/<component>.yaml
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "twig: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		fmt.Print(usage)
		return nil
	}

	cmd := args[0]
	leafPath := args[1]
	var extraArgs []string
	for i, a := range args[2:] {
		if a == "--" {
			extraArgs = args[2+i+1:]
			break
		}
	}

	if !strings.HasSuffix(leafPath, ".yaml") {
		return fmt.Errorf("leaf file must have .yaml extension: %s", leafPath)
	}

	leafAbs, err := filepath.Abs(leafPath)
	if err != nil {
		return fmt.Errorf("resolve path %q: %w", leafPath, err)
	}

	cfg, err := config.Load(filepath.Dir(leafAbs))
	if err != nil {
		return err
	}

	seg, err := pathparse.Parse(cfg.Root, leafAbs)
	if err != nil {
		return err
	}

	l, err := leaf.Load(leafAbs)
	if err != nil {
		return err
	}

	if !cfg.IsGitSource() {
		for _, key := range l.ModuleKeys {
			srcPath := filepath.Join(cfg.ModulesRoot(), l.Modules[key].Source)
			if _, err := os.Stat(srcPath); err != nil {
				return fmt.Errorf("module %q: source path does not exist: %s", key, srcPath)
			}
		}
	}

	mainTF, err := generate.Generate(cfg, seg, l)
	if err != nil {
		return err
	}

	if cmd == "show" {
		fmt.Print(mainTF)
		return nil
	}

	cacheDir := runner.CacheDir(leafAbs)

	if err := runner.WriteMain(cacheDir, mainTF); err != nil {
		return err
	}

	switch cmd {
	case "init":
		return runner.Terraform(cacheDir, "init", extraArgs)
	case "plan", "apply", "destroy":
		if runner.NeedsInit(cacheDir) {
			if err := runner.Init(cacheDir); err != nil {
				return fmt.Errorf("auto-init failed: %w", err)
			}
		}
		return runner.Terraform(cacheDir, cmd, extraArgs)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
}
