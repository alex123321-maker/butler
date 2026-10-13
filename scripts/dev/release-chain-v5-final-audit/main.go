// Network-free independent operator checker. It has no provider/auth/executor.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/butler/butler/internal/acceptance"
)

func readExact(path, expected string, limit int64) ([]byte, error) {
	if !hashPattern.MatchString(expected) {
		return nil, errors.New("operator_input_pin_required")
	}
	stat, e := os.Lstat(path)
	if e != nil || !stat.Mode().IsRegular() || stat.Mode().Perm() != 0600 || stat.Size() <= 0 || stat.Size() > limit {
		return nil, errors.New("named_safe_file_identity_denied")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, errors.New("named_safe_file_read_denied")
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(stat, opened) {
		return nil, errors.New("named_safe_file_replaced")
	}
	raw, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || int64(len(raw)) != stat.Size() || digest(raw) != expected {
		return nil, errors.New("named_safe_file_bytes_denied")
	}
	return raw, nil
}
func binaryHash(path, expected string) error {
	stat, e := os.Lstat(path)
	if e != nil || !stat.Mode().IsRegular() || stat.Mode().Perm() != 0555 || !hashPattern.MatchString(expected) {
		return errors.New("readonly_runtime_binary_identity_denied")
	}
	f, e := os.Open(path)
	if e != nil {
		return errors.New("readonly_runtime_binary_read_denied")
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(stat, opened) {
		return errors.New("readonly_runtime_binary_replaced")
	}
	h := sha256.New()
	n, e := io.Copy(h, f)
	if e != nil || n != stat.Size() || hex.EncodeToString(h.Sum(nil)) != expected {
		return errors.New("readonly_runtime_binary_hash_denied")
	}
	return nil
}
func run() error {
	pins := map[string]*string{}
	for _, name := range []string{"scope", "pg-ledger", "pg-initial", "pg-product", "runtime-state", "hardware", "runtime-profile", "source-manifest"} {
		pins[name] = flag.String(name+"-sha256", "", "exact independently retained named file identity")
	}
	code := flag.String("commit", "", "exact prepared measurement commit")
	source := flag.String("source-sha256", "", "exact prepared source TreeHash")
	app := flag.String("app-sha256", "", "prepared app.test executable")
	broker := flag.String("broker-sha256", "", "prepared Tool Broker executable")
	converter := flag.String("converter-sha256", "", "prepared isolated converter executable")
	pgImage := flag.String("postgres-image", "", "prepared exact PostgreSQL image")
	runtimeImage := flag.String("runtime-image", "", "prepared exact converter runtime image")
	flag.Parse()
	if os.Geteuid() != 10001 {
		return errors.New("readonly_audit_owner_required")
	}
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(*code) || !hashPattern.MatchString(*source) {
		return errors.New("prepared_source_identity_required")
	}
	measuredCode, e := readExact("/source-commit.txt", digest([]byte(*code+"\n")), 41)
	if e != nil || string(measuredCode) != *code+"\n" {
		return errors.New("measured_source_commit_denied")
	}
	tree, e := acceptance.TreeHash("/src", nil)
	if e != nil || tree != *source {
		return errors.New("prepared_source_tree_denied")
	}
	for _, item := range [][2]string{{"/app.test", *app}, {"/tool-broker", *broker}, {"/tool-converter", *converter}} {
		if e = binaryHash(item[0], item[1]); e != nil {
			return e
		}
	}
	parsed := map[string]any{}
	bindings := map[string]string{"code_commit": *code, "source_sha256": *source, "app_sha256": *app, "broker_sha256": *broker, "converter_sha256": *converter, "quality_sha256": qualitySHA, "prerequisite_sha256": prerequisiteSHA}
	for name, pin := range pins {
		path, maximum := "/"+name+".json", int64(8<<20)
		if name == "source-manifest" {
			path, maximum = "/source-manifest.sha256", 1<<20
		}
		raw, e := readExact(path, *pin, maximum)
		if e != nil {
			return e
		}
		bindings[name+"_sha256"] = *pin
		if name == "source-manifest" {
			if e = sourceManifest("/src", raw); e != nil {
				return e
			}
		} else {
			v, e := decode(raw)
			if e != nil {
				return e
			}
			parsed[name] = v
		}
	}
	for name, pin := range map[string]string{"quality": qualitySHA, "prerequisite": prerequisiteSHA} {
		raw, e := readExact("/"+name+".json", pin, 8<<20)
		if e != nil {
			return e
		}
		v, e := decode(raw)
		if e != nil {
			return e
		}
		parsed[name] = v
	}
	profileHash := operatorBindings(obj(parsed["hardware"]), obj(parsed["runtime-state"]), obj(parsed["runtime-profile"]), *pgImage, *runtimeImage, *converter)
	for _, value := range arr(parsed["pg-product"]) {
		product := obj(obj(value)["payload_json"])
		for _, version := range rows(obj(product["postgres"]), "converter_versions") {
			require(version["profile_pin"] == profileHash, "converter_profile_binary_binding")
		}
	}
	result, err := audit(inputs{parsed["scope"], parsed["pg-ledger"], parsed["pg-initial"], parsed["pg-product"], parsed["runtime-state"], parsed["quality"], parsed["prerequisite"]})
	if e = json.NewEncoder(os.Stdout).Encode(struct {
		Report   report            `json:"chain_audit"`
		Bindings map[string]string `json:"evidence_bindings"`
	}{result, bindings}); e != nil {
		return errors.New("safe_audit_output_failed")
	}
	return err
}

// Match every named file and every regular file in the frozen snapshot. The
// shell manifest hashes and the independent TreeHash are distinct encodings.
func sourceManifest(root string, raw []byte) error {
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return errors.New("source_manifest_frame_denied")
	}
	expected := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if len(line) < 69 || !hashPattern.MatchString(line[:64]) || line[64:68] != "  ./" {
			return errors.New("source_manifest_line_denied")
		}
		rel := line[68:]
		if rel == "" || strings.ContainsAny(rel, "\\\x00\r\t") || filepath.IsAbs(rel) || filepath.ToSlash(filepath.Clean(rel)) != rel || rel == ".." || strings.HasPrefix(rel, "../") || expected[rel] != "" {
			return errors.New("source_manifest_path_denied")
		}
		expected[rel] = line[:64]
	}
	seen := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return errors.New("source_manifest_walk_denied")
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return errors.New("source_manifest_special_file_denied")
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return errors.New("source_manifest_walk_denied")
		}
		pin := expected[filepath.ToSlash(rel)]
		if pin == "" {
			return errors.New("source_manifest_missing_file")
		}
		sum, e := acceptance.FileHash(path)
		if e != nil || sum != pin {
			return errors.New("source_manifest_file_bytes_denied")
		}
		seen++
		return nil
	})
	if err != nil {
		return err
	}
	if seen != len(expected) {
		return errors.New("source_manifest_unowned_file")
	}
	return nil
}

func operatorBindings(hardware, runtime, profile object, pgImage, runtimeImage, converter string) string {
	keys(hardware, "go_image postgres_image runtime_base_image go kernel logical_cpus host_ram_kib cpu_manifest_sha256 build_memory_bytes fixture_memory_bytes converter_memory_bytes build_cpus fixture_cpus postgres_cpus converter_cpus provider actual_native_calls channel weights")
	require(hardware["go_image"] == "sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61" && hardware["postgres_image"] == pgImage && hardware["runtime_base_image"] == runtimeImage && strings.HasPrefix(pgImage, "sha256:") && hashPattern.MatchString(strings.TrimPrefix(pgImage, "sha256:")) && strings.HasPrefix(runtimeImage, "sha256:") && hashPattern.MatchString(strings.TrimPrefix(runtimeImage, "sha256:")), "prepared_image_binding")
	require(hardware["provider"] == "actual_native_High" && hardware["channel"] == "loopback_http_simulator" && hardware["weights"] == "not_observed" && hardware["actual_native_calls"] == nil && hardware["go"] == "1.27.2" && integer(hardware["build_memory_bytes"]) == 2<<30 && integer(hardware["fixture_memory_bytes"]) == 1536<<20 && integer(hardware["converter_memory_bytes"]) == 128<<20 && integer(hardware["build_cpus"]) == 2 && integer(hardware["fixture_cpus"]) == 2 && integer(hardware["postgres_cpus"]) == 1 && integer(hardware["converter_cpus"]) == 1 && integer(hardware["logical_cpus"]) > 0 && integer(hardware["host_ram_kib"]) > 0 && text(hardware["kernel"]) != "", "declared_runtime_boundary")
	sha(hardware["cpu_manifest_sha256"])
	require(runtime["image_id"] == hardware["go_image"] && runtime["postgres_image_id"] == pgImage && runtime["converter_image_id"] == runtimeImage, "actual_process_image_binding")
	keys(profile, "dependency checker")
	dep := obj(profile["dependency"])
	keys(dep, "id version sha256")
	require(hashPattern.MatchString(converter) && dep["id"] == "butler-converter-ir" && dep["version"] == "1.0.0" && dep["sha256"] == converter && profile["checker"] == "butler.delimited-json.checks.v1", "isolated_converter_profile")
	// Profile's struct JSON order is part of the existing production digest.
	type dependency struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		SHA     string `json:"sha256"`
	}
	typedProfile := struct {
		Dependency dependency `json:"dependency"`
		Checker    string     `json:"checker"`
	}{dependency{"butler-converter-ir", "1.0.0", converter}, "butler.delimited-json.checks.v1"}
	return hashValue(typedProfile)
}
func main() {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "typed_operator_contract_denied")
			os.Exit(1)
		}
	}()
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
