package acceptance

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type structuralDiagnosticInputFixture struct {
	Version    string    `json:"version"`
	Provenance string    `json:"provenance"`
	Purpose    string    `json:"purpose"`
	Cases      []Example `json:"cases"`
}

func structuralDiagnosticAuthorLoad(t *testing.T) structuralDiagnosticInputFixture {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/acceptance/cloud-structural-json-diagnostic-20261012.json")
	var c structuralDiagnosticInputFixture
	if err != nil || DecodeStrict(raw, &c, "version", "provenance", "purpose", "cases") != nil {
		t.Fatal("new public structural input JSON denied")
	}
	return c
}

func TestCloudStructuralDiagnosticAuthorOriginalWitnessAndAllIDSchema(t *testing.T) {
	c := structuralDiagnosticAuthorLoad(t)
	if c.Version != "butler-cloud-structural-json-diagnostic-inputs-v1" || c.Provenance != "synthetic_public" || c.Purpose != "unseen_structural_protocol_observations_not_quality" || len(c.Cases) != 2 {
		t.Fatal("prospective diagnostic scope changed")
	}
	wanted := []string{"diag:de725ee718ceafca85ae", "diag:ab1d6bea6fc63d47b62d"}
	for i, e := range c.Cases {
		if e.ID != wanted[i] || e.Kind != "retrieval" || e.Input == "" || len(e.Documents) != 2 || e.NoModel {
			t.Fatal("fixed public inputs changed")
		}
		check := VerifyCloudV3Response(e, e.Golden)
		if !check.Exact || check.SchemaError || check.SourceFabrication || check.UnsafeAuthority {
			t.Fatal("separate exact original reference check denied")
		}
		var answer struct {
			Refs []string `json:"refs"`
		}
		if json.Unmarshal(e.Golden, &answer) != nil || answer.Refs == nil || len(answer.Refs) != 1-i {
			t.Fatal("known-positive and genuine no-answer inventory changed")
		}
		input := cloudV3Input(e)
		model, err := MakeCloudV3ModelInput(input, CloudRoleSingle, nil)
		if err != nil {
			t.Fatal("bounded model input denied")
		}
		ids := []string{}
		for id, body := range e.Documents {
			if !freshV3SourceID.MatchString(id) || body == "" {
				t.Fatal("new opaque original identity denied")
			}
			ids = append(ids, id)
		}
		sort.Strings(ids)
		if !reflect.DeepEqual(v3SchemaIDValues(t, model.Schema, e.Kind), ids) || !reflect.DeepEqual(model.Documents, e.Documents) || model.Input != e.Input || len(model.ChildReports) != 0 {
			t.Fatal("trusted schema selected golden subset or changed original inputs")
		}
		raw, _ := json.Marshal(model)
		for _, metadata := range []string{`"golden"`, `"provenance"`, `"purpose"`, `"split"`, `"group"`} {
			if strings.Contains(string(raw), metadata) {
				t.Fatal("author-only metadata entered model context")
			}
		}
	}
}

func TestCloudStructuralDiagnosticAuthorWrongReferenceRejected(t *testing.T) {
	c := structuralDiagnosticAuthorLoad(t)
	for _, index := range []int{0, 1} {
		e := c.Cases[index]
		wrong := `{"refs":["d_fa7ffd6f75b91802f704"]}`
		if index == 1 {
			wrong = `{"refs":["d_2ccbe907d85f7050d06e"]}`
		}
		if VerifyCloudV3Response(e, []byte(wrong)).Exact {
			t.Fatal("unrelated or absent original reference accepted")
		}
	}
}
