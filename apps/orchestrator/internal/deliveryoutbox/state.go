package deliveryoutbox

// State reports the most conservative durable state across all chunks.
func State(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}
	state := "sent"
	for _, m := range messages {
		switch m.Status {
		case "delivery_unknown":
			return "delivery_unknown"
		case "failed":
			state = "failed"
		case "pending", "sending":
			if state != "failed" {
				state = "pending"
			}
		case "result_ready":
			if state == "sent" {
				state = "result_ready"
			}
		}
	}
	return state
}
