package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/geoffsee/pulumi-podman/internal/specadapt"
)

func main() {
	in := flag.String("in", "spec/swagger.yaml", "upstream Podman swagger.yaml")
	out := flag.String("out", "internal/specdata/openapi.adapted.yaml", "adapted OpenAPI path")
	pathmap := flag.String("map", "internal/specdata/pathmap.json", "runtime path map")
	flag.Parse()

	if err := adaptSpec(*in, *out, *pathmap); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func adaptSpec(in, out, pathmap string) error {
	raw, err := os.ReadFile(in)
	if err != nil {
		return fmt.Errorf("read spec: %w", err)
	}
	res, err := specadapt.Adapt(raw)
	if err != nil {
		return fmt.Errorf("adapt spec: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(pathmap), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, res.Spec, 0o644); err != nil {
		return fmt.Errorf("write adapted spec: %w", err)
	}
	if err := os.WriteFile(pathmap, res.PathMap, 0o644); err != nil {
		return fmt.Errorf("write path map: %w", err)
	}
	fmt.Printf("adapted %d operations from %s\n", len(res.Rules), in)
	fmt.Printf("  spec: %s\n", out)
	fmt.Printf("  map:  %s\n", pathmap)
	return nil
}
