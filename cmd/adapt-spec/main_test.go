package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffsee/pulumi-podman/internal/specadapt"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Tests in this package run with cwd = cmd/adapt-spec.
	root := filepath.Clean(filepath.Join(dir, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s: %v", root, err)
	}
	return root
}

func TestAdaptSpecWritesArtifacts(t *testing.T) {
	root := repoRoot(t)
	in := filepath.Join(root, "spec", "swagger.yaml")
	dir := t.TempDir()
	out := filepath.Join(dir, "nested", "openapi.yaml")
	pathmap := filepath.Join(dir, "nested", "map", "pathmap.json")

	if err := adaptSpec(in, out, pathmap); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(in)
	if err != nil {
		t.Fatal(err)
	}
	res, err := specadapt.Adapt(raw)
	if err != nil {
		t.Fatal(err)
	}
	gotSpec, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	gotMap, err := os.ReadFile(pathmap)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotSpec, res.Spec) {
		t.Fatal("adapted spec does not match Adapt()")
	}
	if !bytes.Equal(gotMap, res.PathMap) {
		t.Fatal("path map does not match Adapt()")
	}
}

func TestAdaptSpecMissingInput(t *testing.T) {
	dir := t.TempDir()
	err := adaptSpec(filepath.Join(dir, "missing.yaml"), filepath.Join(dir, "out.yaml"), filepath.Join(dir, "map.json"))
	if err == nil || !strings.Contains(err.Error(), "read spec") {
		t.Fatalf("err = %v", err)
	}
}

func TestAdaptSpecInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(in, []byte("{not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := adaptSpec(in, filepath.Join(dir, "out.yaml"), filepath.Join(dir, "map.json"))
	if err == nil || !strings.Contains(err.Error(), "adapt spec") {
		t.Fatalf("err = %v", err)
	}
}

func TestAdaptSpecCLI(t *testing.T) {
	root := repoRoot(t)
	exe := filepath.Join(t.TempDir(), "adapt-spec")
	build := exec.Command("go", "build", "-o", exe, ".")
	build.Dir = filepath.Join(root, "cmd", "adapt-spec")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	dir := t.TempDir()
	outSpec := filepath.Join(dir, "openapi.yaml")
	outMap := filepath.Join(dir, "pathmap.json")
	cmd := exec.Command(exe,
		"-in", filepath.Join(root, "spec", "swagger.yaml"),
		"-out", outSpec,
		"-map", outMap,
	)
	stdout, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cli: %v\n%s", err, stdout)
	}
	if !strings.Contains(string(stdout), "adapted") {
		t.Fatalf("stdout = %s", stdout)
	}
	if _, err := os.Stat(outSpec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outMap); err != nil {
		t.Fatal(err)
	}

	missing := exec.Command(exe, "-in", filepath.Join(dir, "nope.yaml"), "-out", outSpec, "-map", outMap)
	if err := missing.Run(); err == nil {
		t.Fatal("expected non-zero exit for missing input")
	}
}
