package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudV2ReviewedAuthoredCorpusMatchesOriginalsAndExcludesSecretInput(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", CloudV2CorpusPath))
	if err != nil || hashWindowBytes(raw) != "467ae4978d2d19eed0cad753c0e6ee61081a4b71ee8a0060be5e67b37a26e67c" {
		t.Fatal("reviewed candidate bytes changed")
	}
	c, err := LoadCloudV2Corpus(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal("independently reviewed candidate rejected", err)
	}
	if CloudV2CorpusDigest(c) != "62768fbcc7f389825946bb1632884c1de5ef835644ae473a68bb1ffc3caca172" || CloudV2CriteriaDigest(c) != "280479d286975a09f60e0cee582200d3a36ab11099a86339cdfe0ea6c0556767" {
		t.Fatal("reviewed semantic/criteria pin changed")
	}
	for split, count := range map[string]int{"development": 96, "tuning": 64, "holdout": 76} {
		x, err := ExportCloudV2Inputs(c, split)
		if err != nil || len(x.Cases) != count || split == "holdout" && len(x.BlockedIDs) != 4 {
			t.Fatal("reviewed export inventory changed", split, err)
		}
		encoded, _ := json.Marshal(x)
		for _, forbidden := range []string{"SYNTHETIC-NOT-A-CREDENTIAL-Q7M4", "src230x", "src230y", `"golden"`, `"group"`, "semantic_family", "attack_family"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatal("private/answer metadata entered prospective model input")
			}
		}
	}
	for _, e := range cloudV2Examples(c).Examples {
		x := VerifyCloudV2Response(e, e.Golden)
		if !x.Exact || x.SchemaError || x.SourceFabrication || x.UnsafeAuthority {
			t.Fatal("original field witness or bounded schema changed")
		}
	}
}
