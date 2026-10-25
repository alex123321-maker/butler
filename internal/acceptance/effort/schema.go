package effort

import "encoding/json"

// NativeSchemaV2 is a trusted transport syntax contract shared by all arms.
// It contains no expected answers or evaluator source requirements.
var NativeSchemaV2 = json.RawMessage(`{"type":"OBJECT","required":["goal_version","decision","value","answer","sources","read_refs","claims","hypotheses","open_checks","contradictions"],"properties":{"goal_version":{"type":"INTEGER"},"decision":{"type":"STRING"},"value":{"type":"STRING"},"answer":{"type":"STRING"},"sources":{"type":"ARRAY","maxItems":"16","items":{"type":"STRING"}},"read_refs":{"type":"ARRAY","maxItems":"16","items":{"type":"STRING"}},"claims":{"type":"ARRAY","maxItems":"16","items":{"type":"OBJECT","required":["text","source","quote"],"properties":{"text":{"type":"STRING"},"source":{"type":"STRING"},"quote":{"type":"STRING"}}}},"hypotheses":{"type":"ARRAY","maxItems":"16","items":{"type":"STRING"}},"open_checks":{"type":"ARRAY","maxItems":"16","items":{"type":"STRING"}},"contradictions":{"type":"ARRAY","maxItems":"16","items":{"type":"STRING"}}}}`)
