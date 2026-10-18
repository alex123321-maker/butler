# Exact cmd/provider-auth diagnose-cloudcode / AntigravityDiagnostic contract.
# A cached resolved token or response_decoded alone is insufficient.
.resolved == true and .unsafe_refresh == false and .after.expired == false and
(.trace | type == "array") and
any(.trace[]; .stage == "paid_discovery" and .outcome == "completed" and .http_status == 200) and
all(.trace[] | select(.stage == "paid_discovery");
  (.outcome == "started" and ((.http_status // 0) == 0)) or
  ((.outcome == "response_decoded" or .outcome == "completed") and .http_status == 200))
