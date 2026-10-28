//go:build ignore

// Research-only isolated dependency build. The shared module cache, go.mod and ordinary
// builds are never modified. Both study arms use the same exact callback fix.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

const originalSHA = "076f1d636cd6e2a1b8cadaea24ac60375cd98f1d63034ff796427f49a4408684"
const derivedSHA = "9fdbb58e77cebad5ff489cec5481b1aac2d5e111e44157605b796f7a7d8078fc"
const upstream = "https://github.com/jackc/pgx/blob/v5.11.0/conn.go#L440-L449"

func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

func retain(path string, raw []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		existing, e := os.ReadFile(path)
		if e != nil || !bytes.Equal(existing, raw) {
			return errors.New("existing research build bytes differ")
		}
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := file.Write(raw)
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func prepare(moduleCache, directory string) error {
	original, err := filepath.Abs(filepath.Join(moduleCache, "github.com/jackc/pgx/v5@v5.8.0/conn.go"))
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(original)
	if err != nil || digest(raw) != originalSHA {
		return errors.New("exact original pgx5.8.0 source unavailable")
	}
	old := []byte("\tif err := c.deallocateInvalidatedCachedStatements(ctx); err != nil {\n\t\treturn pgconn.CommandTag{}, err\n\t}\n")
	new := []byte("\tif err := c.deallocateInvalidatedCachedStatements(ctx); err != nil {\n\t\tif c.queryTracer != nil {\n\t\t\tc.queryTracer.TraceQueryEnd(ctx, c, TraceQueryEndData{Err: err})\n\t\t}\n\t\treturn pgconn.CommandTag{}, err\n\t}\n")
	if bytes.Count(raw, old) != 1 {
		return errors.New("exact single callback patch target unavailable")
	}
	patched := bytes.Replace(raw, old, new, 1)
	if digest(patched) != derivedSHA {
		return errors.New("derived callback-only source hash mismatch")
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	type item struct {
		Path     string `json:"path"`
		Bytes    int    `json:"bytes"`
		Original string `json:"original_sha256"`
		Derived  string `json:"derived_sha256"`
	}
	files := []item{}
	var total int
	moduleSource := filepath.Dir(original)
	replacement := filepath.Join(directory, "pgx")
	err = filepath.WalkDir(moduleSource, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(moduleSource, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(replacement, rel)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0700)
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 || len(files) >= 5000 {
			return errors.New("unbounded or nonregular research module source")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		total += len(body)
		if total > 64<<20 {
			return errors.New("research module source exceeds bound")
		}
		oldHash := digest(body)
		if rel == "conn.go" {
			if oldHash != originalSHA {
				return errors.New("original changed during copy")
			}
			body = patched
		}
		if err = retain(destination, body); err != nil {
			return err
		}
		if err = os.Chmod(destination, 0444); err != nil {
			return err
		}
		files = append(files, item{filepath.ToSlash(rel), len(body), oldHash, digest(body)})
		return nil
	})
	if err != nil {
		return err
	}
	expected := map[string]bool{}
	for _, file := range files {
		expected[file.Path] = true
	}
	err = filepath.WalkDir(replacement, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(replacement, path)
		if err != nil {
			return err
		}
		if !expected[filepath.ToSlash(rel)] {
			return errors.New("unexpected research module file")
		}
		return nil
	})
	if err != nil {
		return err
	}
	filesRaw, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return err
	}
	filesRaw = append(filesRaw, '\n')
	if err = retain(filepath.Join(directory, "module-files.json"), filesRaw); err != nil {
		return err
	}
	rootMod, err := os.ReadFile("go.mod")
	if err != nil {
		return err
	}
	rootSum, err := os.ReadFile("go.sum")
	if err != nil {
		return err
	}
	researchMod := append(append([]byte(nil), rootMod...), []byte("\n// Research-only observer callback; no production module change.\nreplace github.com/jackc/pgx/v5 => "+strconv.Quote(filepath.ToSlash(replacement))+"\n")...)
	if err = retain(filepath.Join(directory, "research.mod"), researchMod); err != nil {
		return err
	}
	if err = retain(filepath.Join(directory, "research.sum"), rootSum); err != nil {
		return err
	}
	pins := struct {
		Version     string `json:"version"`
		Original    string `json:"original_sha256"`
		Derived     string `json:"derived_sha256"`
		Files       string `json:"module_files_sha256"`
		RootMod     string `json:"original_go_mod_sha256"`
		RootSum     string `json:"original_go_sum_sha256"`
		ResearchMod string `json:"research_mod_sha256"`
		Upstream    string `json:"upstream"`
	}{"behavior76-pgx-observer-build-v1", originalSHA, derivedSHA, digest(filesRaw), digest(rootMod), digest(rootSum), digest(researchMod), upstream}
	pinsRaw, err := json.MarshalIndent(pins, "", "  ")
	if err != nil {
		return err
	}
	pinsRaw = append(pinsRaw, '\n')
	if err = retain(filepath.Join(directory, "pins.json"), pinsRaw); err != nil {
		return err
	}
	fmt.Println("isolated research pgx callback build", derivedSHA, "module inventory", digest(filesRaw))
	return nil
}

func main() {
	if len(os.Args) != 3 || prepare(os.Args[1], os.Args[2]) != nil {
		fmt.Fprintln(os.Stderr, "exact research-only pgx callback build unavailable")
		os.Exit(1)
	}
}
