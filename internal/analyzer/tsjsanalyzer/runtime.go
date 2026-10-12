package tsjsanalyzer

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed helper.cjs assets/*
var assets embed.FS

// extract never trusts previously extracted JavaScript. Each process gets a
// private directory, and content-addressed assets are verified before use.
func extract() (string, func(), error) {
	compiler, err := assets.ReadFile("assets/typescript.cjs")
	if err != nil {
		return "", nil, fmt.Errorf("TypeScript compiler unavailable; run make prepare-analyzers: %w", err)
	}
	helper, err := assets.ReadFile("helper.cjs")
	if err != nil {
		return "", nil, err
	}
	digest := sha256.Sum256(append(append([]byte{}, compiler...), helper...))
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(cache, 0700); err != nil {
		return "", nil, err
	}
	// MkdirTemp creates a non-shared 0700 directory even when a cache ancestor
	// already exists. Never execute a path chosen by repository metadata.
	dir, err := os.MkdirTemp(cache, "nami-tsjs-6.0.3-"+hex.EncodeToString(digest[:])+"-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	files := map[string][]byte{"typescript.cjs": compiler, "helper.cjs": helper}
	for _, name := range []string{"LICENSE.txt", "ThirdPartyNoticeText.txt"} {
		content, err := assets.ReadFile("assets/" + name)
		if err != nil {
			cleanup()
			return "", nil, err
		}
		files[name] = content
	}
	for name, content := range files {
		tmp, err := os.CreateTemp(dir, ".extract-")
		if err != nil {
			cleanup()
			return "", nil, err
		}
		_, writeErr := tmp.Write(content)
		closeErr := tmp.Close()
		if writeErr != nil {
			cleanup()
			return "", nil, writeErr
		}
		if closeErr != nil {
			cleanup()
			return "", nil, closeErr
		}
		target := filepath.Join(dir, name)
		if err := os.Rename(tmp.Name(), target); err != nil {
			cleanup()
			return "", nil, err
		}
		if err := verifyArtifact(target, content); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return dir, cleanup, nil
}
func verifyArtifact(name string, expected []byte) error {
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("unsafe analyzer artifact permissions: %s", name)
	}
	actual, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	if sha256.Sum256(actual) != sha256.Sum256(expected) {
		return fmt.Errorf("analyzer artifact integrity mismatch: %s", name)
	}
	return nil
}
func parse(root string, files []string) ([]byte, error) {
	runtime, err := exec.LookPath("node")
	if err != nil {
		return nil, fmt.Errorf("Node.js unavailable: %w", err)
	}
	dir, cleanup, err := extract()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	input, err := marshalInput(root, files)
	if err != nil {
		return nil, err
	}
	command := exec.Command(runtime, filepath.Join(dir, "helper.cjs"))
	command.Dir = dir
	// Pass only runtime essentials. NODE_* (including V8 coverage writes),
	// loader injection variables and npm customizations cannot affect the helper.
	allowed := map[string]bool{"PATH": true, "HOME": true, "USERPROFILE": true, "SYSTEMROOT": true, "WINDIR": true, "TMPDIR": true, "TMP": true, "TEMP": true, "LANG": true, "LC_ALL": true}
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if allowed[strings.ToUpper(key)] {
			command.Env = append(command.Env, value)
		}
	}
	command.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("TS/JS helper failed: %w: %s", err, stderr.String())
	}
	return output, nil
}
