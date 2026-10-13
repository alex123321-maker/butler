package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCloudV3ReviewedWholeAuthoredCorpusOriginalWitnessAndNoSecretExport(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", CloudV3CorpusPath))
	if err != nil || hashWindowBytes(raw) != "20099b3b5d7b5ec21cea7e3f1b6ddc62b9bc623859891a9450e47c18303c3da4" || bytes.Contains(raw, []byte("\r")) {
		t.Fatal("reviewed exact LF candidate changed")
	}
	c, err := LoadCloudV3Corpus(bytes.NewReader(raw))
	if err != nil {
		t.Fatal("reviewed candidate parser", err)
	}
	if CloudV3CorpusDigest(c) != "da139da4fb5a7d7d451e261e38ff6301a89a243c81f671e3a03e29fbba0d7520" || CloudV3SchemaDigest() != "69ab4d01cf7e2d9bed53321b2da6925ec7285bffea7a22a995070b8796f15ce1" || CloudV3PolicyDigest(c) != "00e75e7666529c3cf59c72758d3f83a21e8616664f2f7386c44ea3c3c6f60fb7" || CloudV3CriteriaDigest(c) != "a739917609173971b08ed2555b8655abd66ae63658645a8d91cb40244943a5b7" || CloudV3ResourcesDigest() != "2000c3efa0f47865ce774995a37e0e56f777e2f4a38674d6935234f80e727da7" {
		t.Fatal("reviewed semantic/schema/policy/criteria/resource identity changed")
	}
	counts := map[string]int{}
	groups := map[string]string{}
	absent := map[string]bool{}
	for _, g := range c.Groups {
		groups[g.ID] = g.Split
	}
	for _, e := range c.Cases {
		original := Example{Kind: e.Kind, Documents: e.Documents, Golden: e.Golden}
		check := VerifyCloudV3Response(original, e.Golden)
		if !check.Exact || check.SchemaError || check.SourceFabrication || check.UnsafeAuthority {
			t.Fatal("actual reviewed witness/schema changed", e.ID)
		}
		if e.Kind == "extraction" {
			var facts struct {
				Facts []json.RawMessage `json:"facts"`
			}
			if json.Unmarshal(e.Golden, &facts) != nil {
				t.Fatal("gold JSON")
			}
			if len(facts.Facts) == 0 {
				counts[groups[e.Group]]++
				absent[e.ID] = true
			}
		}
		if e.NoModel {
			continue
		}
		input := cloudV3Input(Example{ID: e.ID, Kind: e.Kind, Input: e.Input, Documents: e.Documents})
		model, err := MakeCloudV3ModelInput(input, CloudRoleSingle, nil)
		if err != nil {
			t.Fatal("exact model input", e.ID)
		}
		if e.Kind == "retrieval" || e.Kind == "extraction" {
			ids := []string{}
			for id := range e.Documents {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			if !reflect.DeepEqual(v3SchemaIDValues(t, model.Schema, e.Kind), ids) {
				t.Fatal("all originals not in current per-case schema", e.ID)
			}
		}
	}
	for _, part := range []string{"development", "tuning", "holdout"} {
		if counts[part] != 2 {
			t.Fatal("negative extraction coverage lost", part)
		}
	}
	if len(absent) != 6 {
		t.Fatal("missing-field count changed")
	}
	for _, id := range []string{"q305d", "q312d", "q315d", "q320d", "q323d", "q328d"} {
		if !absent[id] {
			t.Fatal("reviewed absence identity changed", id)
		}
	}
	for part, count := range map[string]int{"development": 96, "tuning": 64, "holdout": 76} {
		x, err := ExportCloudV3Inputs(c, part)
		if err != nil || len(x.Cases) != count || part == "holdout" && len(x.BlockedIDs) != 4 {
			t.Fatal("model eligibility changed", part)
		}
		encoded, _ := json.Marshal(x)
		for _, forbidden := range []string{"SYNTHETIC_V3_PAIRING_CHALLENGE", "SYNTHETIC_V3_PAIRING_REPLY", `"golden"`, `"group"`, `"semantic_family"`, `"attack_family"`} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatal("secret/answer/group metadata exported")
			}
		}
	}
	t.Logf("reviewed corpus=%s schema=%s policy=%s criteria=%s resources=%s", CloudV3CorpusDigest(c), CloudV3SchemaDigest(), CloudV3PolicyDigest(c), CloudV3CriteriaDigest(c), CloudV3ResourcesDigest())
}
