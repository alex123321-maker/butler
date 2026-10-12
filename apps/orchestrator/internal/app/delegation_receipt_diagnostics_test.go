package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

type nativeReceiptProjectionReason = string

const (
	nativeReceiptMatched               nativeReceiptProjectionReason = "passed"
	nativeReceiptOutputTooLarge        nativeReceiptProjectionReason = "output_too_large"
	nativeReceiptInvalidJSON           nativeReceiptProjectionReason = "invalid_json"
	nativeReceiptTrailingData          nativeReceiptProjectionReason = "trailing_data"
	nativeReceiptObjectRequired        nativeReceiptProjectionReason = "object_required"
	nativeReceiptFieldLimit            nativeReceiptProjectionReason = "object_field_limit"
	nativeReceiptDuplicateFields       nativeReceiptProjectionReason = "duplicate_fields"
	nativeReceiptUnknownFields         nativeReceiptProjectionReason = "unknown_fields"
	nativeReceiptMissingIDs            nativeReceiptProjectionReason = "required_field_missing"
	nativeReceiptArrayRequired         nativeReceiptProjectionReason = "id_array_required"
	nativeReceiptCountInvalid          nativeReceiptProjectionReason = "id_count_invalid"
	nativeReceiptStringRequired        nativeReceiptProjectionReason = "id_string_required"
	nativeReceiptEmptyID               nativeReceiptProjectionReason = "id_empty"
	nativeReceiptDuplicateIDs          nativeReceiptProjectionReason = "ids_duplicate"
	nativeReceiptActualChildrenInvalid nativeReceiptProjectionReason = "actual_children_invalid"
	nativeReceiptIDsMismatch           nativeReceiptProjectionReason = "ids_mismatch"
)

// Fixture diagnostics retain only fixed reason enums, hashes, counts and
// validation flags. No raw output, key, ID or decoder error may be exported.
type nativeReceiptProjectionEvidence struct {
	Reason               nativeReceiptProjectionReason `json:"reason"`
	Bytes                int                           `json:"bytes"`
	SHA256               string                        `json:"sha256"`
	JSONValid            bool                          `json:"json_valid"`
	JSONObject           bool                          `json:"json_object"`
	RequiredFieldPresent bool                          `json:"required_field_present"`
	DuplicateFields      bool                          `json:"duplicate_fields"`
	UnknownFields        bool                          `json:"unknown_fields"`
	ExactFieldSet        bool                          `json:"exact_field_set"`
	IDsArray             bool                          `json:"ids_array"`
	IDCount              *int                          `json:"id_count"`
	IDsStrings           bool                          `json:"ids_strings"`
	IDsDistinct          bool                          `json:"ids_distinct"`
	ActualChildrenValid  bool                          `json:"actual_children_valid"`
	IDsMatch             bool                          `json:"ids_match"`
}

// This is a projection of two independently persisted admission IDs. Matching
// it supplies no child-result, task-completion or owner-authority verdict.
func nativeReceiptProjection(raw string, children []string) nativeReceiptProjectionEvidence {
	hash := sha256.Sum256([]byte(raw))
	r := nativeReceiptProjectionEvidence{
		Bytes: len(raw), SHA256: hex.EncodeToString(hash[:]),
		JSONValid:           json.Valid([]byte(raw)),
		ActualChildrenValid: len(children) == 2 && children[0] != "" && children[1] != "" && children[0] != children[1],
	}
	deny := func(reason nativeReceiptProjectionReason) nativeReceiptProjectionEvidence {
		r.Reason = reason
		return r
	}
	if len(raw) > 65536 {
		return deny(nativeReceiptOutputTooLarge)
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	var object json.RawMessage
	if decoder.Decode(&object) != nil {
		return deny(nativeReceiptInvalidJSON)
	}
	r.JSONObject = len(object) > 0 && object[0] == '{'
	if decoder.Decode(new(json.RawMessage)) != io.EOF {
		return deny(nativeReceiptTrailingData)
	}
	if !r.JSONValid {
		return deny(nativeReceiptInvalidJSON)
	}
	if !r.JSONObject {
		return deny(nativeReceiptObjectRequired)
	}
	decoder = json.NewDecoder(strings.NewReader(string(object)))
	if _, err := decoder.Token(); err != nil {
		return deny(nativeReceiptInvalidJSON)
	}
	keys := make([]string, 0, 2)
	var idsRaw json.RawMessage
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return deny(nativeReceiptInvalidJSON)
		}
		if len(keys) == 32 {
			return deny(nativeReceiptFieldLimit)
		}
		// Decoder.Token resolves JSON Unicode escapes before this comparison.
		// EqualFold also identifies case/Unicode fold aliases, but never makes
		// such an alias an accepted spelling of the required ASCII field.
		for _, previous := range keys {
			r.DuplicateFields = r.DuplicateFields || strings.EqualFold(previous, key)
		}
		keys = append(keys, key)
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return deny(nativeReceiptInvalidJSON)
		}
		if key == "admitted_task_ids" {
			if !r.RequiredFieldPresent {
				idsRaw = value
			}
			r.RequiredFieldPresent = true
		} else {
			r.UnknownFields = true
		}
	}
	if _, err := decoder.Token(); err != nil {
		return deny(nativeReceiptInvalidJSON)
	}
	if r.DuplicateFields {
		return deny(nativeReceiptDuplicateFields)
	}
	if r.UnknownFields {
		return deny(nativeReceiptUnknownFields)
	}
	if !r.RequiredFieldPresent {
		return deny(nativeReceiptMissingIDs)
	}
	r.ExactFieldSet = true
	r.IDsArray = len(idsRaw) > 0 && idsRaw[0] == '['
	if !r.IDsArray {
		return deny(nativeReceiptArrayRequired)
	}
	var values []json.RawMessage
	if json.Unmarshal(idsRaw, &values) != nil {
		return deny(nativeReceiptInvalidJSON)
	}
	count := len(values)
	r.IDCount = &count
	if count != 2 {
		return deny(nativeReceiptCountInvalid)
	}
	ids := make([]string, 2)
	for i, value := range values {
		if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &ids[i]) != nil {
			return deny(nativeReceiptStringRequired)
		}
	}
	r.IDsStrings = true
	if ids[0] == "" || ids[1] == "" {
		return deny(nativeReceiptEmptyID)
	}
	r.IDsDistinct = ids[0] != ids[1]
	if !r.IDsDistinct {
		return deny(nativeReceiptDuplicateIDs)
	}
	if !r.ActualChildrenValid {
		return deny(nativeReceiptActualChildrenInvalid)
	}
	r.IDsMatch = ids[0] == children[0] && ids[1] == children[1] || ids[0] == children[1] && ids[1] == children[0]
	if !r.IDsMatch {
		return deny(nativeReceiptIDsMismatch)
	}
	r.Reason = nativeReceiptMatched
	return r
}

func TestNativeReceiptProjectionDiagnostics(t *testing.T) {
	children := []string{"child-a", "child-b"}
	cases := []struct {
		name, raw string
		reason    nativeReceiptProjectionReason
	}{
		{"plain", `{"admitted_task_ids":["child-a","child-b"]}`, nativeReceiptMatched},
		{"whitespace_reordered_ids", " \n\t{ \"admitted_task_ids\" : [ \"child-b\", \"child-a\" ] }\r\n", nativeReceiptMatched},
		{"escaped_canonical_key_and_id", `{"\u0061dmitted_task_ids":["child\u002da","child-b"]}`, nativeReceiptMatched},
		{"narrative", `The admitted tasks are child-a and child-b.`, nativeReceiptInvalidJSON},
		{"fenced", "```json\n{\"admitted_task_ids\":[\"child-a\",\"child-b\"]}\n```", nativeReceiptInvalidJSON},
		{"malformed", `{"admitted_task_ids":["child-a","child-b"]`, nativeReceiptInvalidJSON},
		{"trailing_object", `{"admitted_task_ids":["child-a","child-b"]}{}`, nativeReceiptTrailingData},
		{"trailing_narrative", `{"admitted_task_ids":["child-a","child-b"]} done`, nativeReceiptTrailingData},
		{"non_object", `["child-a","child-b"]`, nativeReceiptObjectRequired},
		{"null_object", `null`, nativeReceiptObjectRequired},
		{"missing", `{}`, nativeReceiptMissingIDs},
		{"alias", `{"ADMITTED_TASK_IDS":["child-a","child-b"]}`, nativeReceiptUnknownFields},
		{"unicode_confusable", `{"admitted_task_ıds":["child-a","child-b"]}`, nativeReceiptUnknownFields},
		{"duplicate", `{"admitted_task_ids":["child-a","child-b"],"admitted_task_ids":["child-a","child-b"]}`, nativeReceiptDuplicateFields},
		{"duplicate_case_alias", `{"admitted_task_ids":["child-a","child-b"],"ADMITTED_TASK_IDS":["child-a","child-b"]}`, nativeReceiptDuplicateFields},
		{"duplicate_alias_first", `{"ADMITTED_TASK_IDS":["child-a","child-b"],"admitted_task_ids":["child-a","child-b"]}`, nativeReceiptDuplicateFields},
		{"duplicate_unicode_escape", `{"admitted_task_ids":["child-a","child-b"],"admitted_task_\u0069ds":["child-a","child-b"]}`, nativeReceiptDuplicateFields},
		{"duplicate_unicode_fold", `{"admitted_task_ids":["child-a","child-b"],"admitted_task_idſ":["child-a","child-b"]}`, nativeReceiptDuplicateFields},
		{"extra", `{"admitted_task_ids":["child-a","child-b"],"accepted":true}`, nativeReceiptUnknownFields},
		{"wrapper", `{"receipt":{"admitted_task_ids":["child-a","child-b"]}}`, nativeReceiptUnknownFields},
		{"null_ids", `{"admitted_task_ids":null}`, nativeReceiptArrayRequired},
		{"object_ids", `{"admitted_task_ids":{"a":"child-a","b":"child-b"}}`, nativeReceiptArrayRequired},
		{"count", `{"admitted_task_ids":["child-a"]}`, nativeReceiptCountInvalid},
		{"null_id", `{"admitted_task_ids":[null,"child-b"]}`, nativeReceiptStringRequired},
		{"numeric_id", `{"admitted_task_ids":[42,"child-b"]}`, nativeReceiptStringRequired},
		{"empty_id", `{"admitted_task_ids":["","child-b"]}`, nativeReceiptEmptyID},
		{"duplicate_id", `{"admitted_task_ids":["child-a","child-a"]}`, nativeReceiptDuplicateIDs},
		{"wrong_id", `{"admitted_task_ids":["child-a","other"]}`, nativeReceiptIDsMismatch},
		{"untrimmed_id", `{"admitted_task_ids":[" child-a","child-b"]}`, nativeReceiptIDsMismatch},
		{"uncorrected_case_id", `{"admitted_task_ids":["CHILD-A","child-b"]}`, nativeReceiptIDsMismatch},
		{"oversize", strings.Repeat(" ", 65537), nativeReceiptOutputTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := nativeReceiptProjection(tc.raw, children)
			hash := sha256.Sum256([]byte(tc.raw))
			if d.Reason != tc.reason || d.Bytes != len(tc.raw) || d.SHA256 != hex.EncodeToString(hash[:]) || d.JSONValid != json.Valid([]byte(tc.raw)) || d.IDsMatch != (tc.reason == nativeReceiptMatched) {
				t.Fatal("receipt diagnostic reason/hash/flags invalid")
			}
			if d.IDsMatch && (!d.JSONObject || !d.JSONValid || !d.ExactFieldSet || !d.IDsArray || !d.IDsStrings || !d.IDsDistinct || d.IDCount == nil || *d.IDCount != 2 || !d.ActualChildrenValid) {
				t.Fatal("matched diagnostic lacks exact structural evidence")
			}
		})
	}
}

func TestNativeReceiptProjectionActualChildrenAndContentFreeExport(t *testing.T) {
	raw := `{"admitted_task_ids":["child-a","child-b"]}`
	for _, children := range [][]string{nil, {"child-a"}, {"child-a", "child-b", "child-c"}, {"", "child-b"}, {"child-a", "child-a"}} {
		d := nativeReceiptProjection(raw, children)
		if d.Reason != nativeReceiptActualChildrenInvalid || d.ActualChildrenValid || d.IDsMatch {
			t.Fatal("missing/invalid actual admissions became successful projection")
		}
	}
	marker := "private-untrusted-diagnostic-marker"
	for _, body := range []string{
		`{"admitted_task_ids":["` + marker + `","child-b"]}`,
		`{"admitted_task_ids":["child-a","child-b"],"` + marker + `":"Bearer ` + marker + `"}`,
		`provider error: ` + marker,
	} {
		d := nativeReceiptProjection(body, []string{marker, "child-b"})
		encoded, err := json.Marshal(d)
		if err != nil || strings.Contains(string(encoded), marker) || strings.Contains(string(encoded), "Bearer") || strings.Contains(string(encoded), "admitted_task_ids") {
			t.Fatal("raw output/key/ID/error entered diagnostic export")
		}
	}
}
