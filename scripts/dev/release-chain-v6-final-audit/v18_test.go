package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestV18AuditRawProtocolPins(t *testing.T) {
	p := v18AuditPolicy()
	p.VerifyPrerequisite(obj(actualPublic(t, "release-chain-v18-high-prerequisite-20261013.json", v18PrerequisiteSHA)))
	p.VerifyHuman(inputs{ExpenseException: syntheticWitness(t).ExpenseException, SafeTimeoutWaiver: actualPublic(t, "release-chain-v18-user-policy-amendment-20261013.json", v18HumanSHA)})
	if !reflect.DeepEqual(reviewedScenarioNames(p), []string{"explicit_absence", "lost_ack", "restart_sent"}) || !reflect.DeepEqual(p.RetainedScenarioNames, []string{"owner_stop"}) || p.Baseline+p.Maximum != 778 || p.KnownTotal != 2099516 || p.UnknownTotal != 51 || !p.CurrentSourceSemantics || !p.RequireCaptureStatus {
		t.Fatal("declared prospective scope/composite changed")
	}
	for _, name := range []string{"v6", "v7", "v8", "v9", "v10", "v11", "v12", "v13", "v14", "v15", "v16"} {
		old := selectedAuditPolicy(name)
		if old.CurrentSourceSemantics || old.RequireCaptureStatus || old.RetainedScenarioNames != nil {
			t.Fatal("historical checker silently realigned", name)
		}
	}
}
func TestV18AuditActual658CarryAndRetainedStop(t *testing.T) {
	root := candidateRepo(t)
	for _, v := range []int{16, 17} {
		prior, count := 616, 636
		priorPin, historyPin, bridgePin, fold := v16HistorySHA, v16ClosedHistorySHA, v16ClosedBridgeSHA, v17PriorSHA
		input, output, total, unknown, pairs := int64(1372607), int64(30515), int64(1844192), 50, 30
		if v == 17 {
			prior, count = 636, 658
			priorPin, historyPin, bridgePin, fold = v16ClosedHistorySHA, v17HistorySHA, v17BridgeSHA, v18PriorSHA
			input, output, total, unknown, pairs = 1601910, 34311, 2099516, 51, 31
		}
		previous := obj(pinnedObject(root, fmt.Sprintf("release-chain-v%d-history%d-20261013.json", v-1, prior), priorPin))
		current := obj(pinnedObject(root, fmt.Sprintf("release-chain-v%d-history%d-20261013.json", v, count), historyPin))
		bridge := obj(pinnedObject(root, fmt.Sprintf("release-chain-v%d-carry%d-independent-20261013.json", v, count), bridgePin))
		verifyV18ClosedHistoryObjects(previous, current, bridge, v, prior, count, fold, total, input, output, unknown, pairs)
	}
	verifyV18OwnerStopComponent(root)
}
func TestV18AuditSourceIdentityUsesRevisionEnvelope(t *testing.T) {
	id := "source_01a23220ddd3cfcb6d86b749d7259f847eb75bb829a59c53f51a7dbb6480d9d5"
	expected := "2c8f703ac21700c0ec505a01757eade27ea8ea39ca61c6e75abdb74c6ac28e5d"
	if v18DocumentRevisionHash(id) != expected || expected == digest([]byte(v18OriginalCSV)) {
		t.Fatal("catalog envelope confused with raw body")
	}
	ref := frozenObject(`{"source_id":"` + id + `","revision":1,"access_revision":1,"content_hash":"` + expected + `"}`)
	row := obj(clone(ref))
	row["status"] = "active"
	verifyV18DocumentRevision(ref, map[string]object{id: row})
	row["content_hash"] = digest([]byte(v18OriginalCSV))
	defer func() {
		if recover() == nil {
			t.Fatal("raw body digest accepted as catalog revision")
		}
	}()
	verifyV18DocumentRevision(ref, map[string]object{id: row})
}
func TestV18AuditDerivedCatalogRevisionWithContractParent(t *testing.T) {
	version := frozenObject(`{"converter_id":"developer-inventory","digest":"204161eb69280938ab4607d90519b8ce4944239f3c7349761010689251ae02ee","need_hash":"3af7de4de50794fca312f25ab6dc6873179bba5400770d32237bbac3bbb835de","source_ref_json":{"source_id":"source_bff108d1a4d2afcb0af3dc7dc7aec80c566ffd77"}}`)
	refs := frozenObject(`{"document":{"source_id":"source_01a23220ddd3cfcb6d86b749d7259f847eb75bb829a59c53f51a7dbb6480d9d5"},"need":{"source_id":"source_4f652f79aaa2f953a94f9c0bef3b1cbc7a4e0bcb27067d994a7c79016ebcf652"},"spec":{"source_id":"source_7973b4b367ee0b8f082dbf0f78f70d360b020b5b04e83ac2ac43771d0b44278a"}}`)
	for _, pair := range [][2]string{{"source_e87043d032c9a597f01460859ea01c1933520900", "41c808f49b61a7901eba3f74904d67fa88bbf57b8db5b19c5b3737b70dd9921a"}, {"source_7900600c9485d21936b81b4d98f72608cafaa0d0", "481571adfb9741ccfb2ccc61a6c25efda6763cd76b694165d35ab53538169363"}} {
		ref := frozenObject(`{"source_id":"` + pair[0] + `","revision":1,"access_revision":1,"content_hash":"` + pair[1] + `"}`)
		row := obj(clone(ref))
		row["status"] = "active"
		verifyV18DerivedConversionRevision(ref, row, version, refs)
		for _, change := range []string{"raw_body_hash", "foreign_spec", "foreign_source", "wrong_package", "stale_access"} {
			t.Run(pair[0]+"/"+change, func(t *testing.T) {
				r, s, v, f := obj(clone(ref)), obj(clone(row)), obj(clone(version)), obj(clone(refs))
				switch change {
				case "raw_body_hash":
					r["content_hash"], s["content_hash"] = digest([]byte(v18GoldenOutput)), digest([]byte(v18GoldenOutput))
				case "foreign_spec":
					obj(f["spec"])["source_id"] = "foreign-contract"
				case "foreign_source":
					r["source_id"], s["source_id"] = "foreign-output", "foreign-output"
				case "wrong_package":
					v["digest"] = strings.Repeat("b", 64)
				case "stale_access":
					s["access_revision"] = jsonNumber(2)
				}
				defer func() {
					if recover() == nil {
						t.Fatal("derived source proof tamper accepted")
					}
				}()
				verifyV18DerivedConversionRevision(r, s, v, f)
			})
		}
	}
}
func TestV18AuditStoppedWaitingIntentsRemainUnarmed(t *testing.T) {
	initial := frozenObject(`{"revision":1,"constraints_revision":1}`)
	tasks := map[string]object{"root": frozenObject(`{"revision":2,"constraints_revision":2}`), "child": frozenObject(`{"revision":2,"constraints_revision":2}`)}
	db := frozenObject(`{"continuations":[{"continuation_id":"one","parent_task_id":"root","child_task_id":"child","parent_revision":1,"constraints_revision":1,"purpose":"candidate_return","intent_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","state":"waiting","input_id":null,"ready_hash":""},{"continuation_id":"two","parent_task_id":"root","child_task_id":"child","parent_revision":1,"constraints_revision":1,"purpose":"activated_catalog","intent_hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","state":"waiting","input_id":null,"ready_hash":""}]}`)
	verifyV18StoppedWaitingIntents(db, initial, tasks, "root", "child")
	for _, kind := range []string{"queued", "armed_input", "ready_hash", "foreign_child", "current_rev", "duplicate_purpose"} {
		t.Run(kind, func(t *testing.T) {
			x := obj(clone(db))
			c := obj(arr(x["continuations"])[0])
			switch kind {
			case "queued":
				c["state"] = "queued"
			case "armed_input":
				c["input_id"] = "forged-input"
			case "ready_hash":
				c["ready_hash"] = strings.Repeat("c", 64)
			case "foreign_child":
				c["child_task_id"] = "foreign"
			case "current_rev":
				c["parent_revision"] = jsonNumber(2)
			case "duplicate_purpose":
				c["purpose"] = "activated_catalog"
			}
			defer func() {
				if recover() == nil {
					t.Fatal("stopped055 revived")
				}
			}()
			verifyV18StoppedWaitingIntents(x, initial, tasks, "root", "child")
		})
	}
}
