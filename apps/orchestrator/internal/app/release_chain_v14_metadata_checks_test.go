package app

import (
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestReleaseChainV14MetadataDeclared120KeepsLegacy80(t *testing.T) {
	directory := t.TempDir()
	sink := releaseChainProposalSinkFor(true, directory, "normal_sent")
	call := transport.ToolCallRequest{ToolCallRef: "synthetic-owned-call", ToolName: "synthetic.read", ArgsJSON: `{}`}
	if sink.retain(1120, "start", "synthetic-run", &call) != nil || sink.retain(1001, "submit", "synthetic-run", &call) != nil {
		t.Fatal("synthetic own observation unavailable")
	}
	if _, err := readReleaseChainProposalMetadata(directory, 1000, 120); err == nil {
		t.Fatal("legacy80 reader silently enlarged")
	}
	items, err := readReleaseChainProposalMetadata120(directory, 1000)
	if err != nil || len(items) != 2 || items[0]["ordinal"] != float64(1001) || items[1]["ordinal"] != float64(1120) {
		t.Fatal("fixed120 metadata bound or source ordering lost", err)
	}
	if _, err := readReleaseChainProposalMetadataWithDeclaredLimit(directory, 1000, 120, 121); err == nil {
		t.Fatal("unreviewed metadata declaration accepted")
	}
	if sink.retain(1121, "submit", "synthetic-run", &call) != nil {
		t.Fatal("synthetic outside-bound fixture unavailable")
	}
	if _, err := readReleaseChainProposalMetadata120(directory, 1000); err == nil {
		t.Fatal("new scope imported an outside-bound observation")
	}
}
