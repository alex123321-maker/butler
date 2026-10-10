package localmodels

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/security/workspace"
)

const DecisionRevision = "02b55d7076f15129e59ab3f94783f32c4b088674"
const DecideRevision = "e173bca1f0c4217d7a55b3b0c705d5ece8a0218d"

// Only this exact reviewed upstream code/config can reach a local trust loader.
// The model-root bind mount must remain read-only. Runtime callers cannot name
// code, revisions, files or arbitrary endpoints through inference input.
var decisionCode = map[string]string{
	"audio.py":              "c5ff09d52d6a079e79d0505c89dbd0582ba2bc9abf2b26595d8d5b1d0c347bdf",
	"encoder.py":            "5f2e20319ee42a7367febf06c7653d7b3f9b28ef99a64cb9ec7739563dcfcffe",
	"modeling_d1.py":        "2b71a5c909ec5d3aa6307378a41a0fd24af1e17d5c1c1f8d510582147051a686",
	"prompt.py":             "a6b29a55ec8345f1fc1fdbfcc4b64d80d473dc4316f095a62c51cb6cce1194cf",
	"vision.py":             "43ad71029f82e20fd26b97574627773d47639d7a3742c8b04fe5bf55f41c4c28",
	"config.json":           "dfccae241822f81752426629c88c8b8d7efdd03dd19418b6645c604fad43d619",
	"tokenizer_config.json": "03efc74b752b899b6292ff7e607738be59e6f206fec9aca32bbc05e9184090fc",
}

func (b *Process) verifyDecisionCode(p *pb.ModelProfile) error {
	if p.Revision != DecisionRevision || p.RuntimeVersion != "torch2.10.0+cu128/transformers5.19.0/d1code02b55d7" || p.Precision != "FP32" || p.PreferredDevice != "cpu" || p.PreprocessingVersion != "d1-text-choice-v1" {
		return fault(Unavailable)
	}
	for name, digest := range decisionCode {
		body, e := workspace.ReadRegular(b.cfg.ModelRoot, filepath.Join(p.Revision, name), 1<<20)
		if e != nil {
			return fault(Unavailable)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != digest {
			return fault(Unavailable)
		}
	}
	return nil
}
