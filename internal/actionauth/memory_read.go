package actionauth

func isMemoryRead(tool string) bool {
	return tool == "memory.search" || tool == "memory.open" || tool == "memory.neighbors" || tool == "memory.facts" || tool == "messages.search" || tool == "messages.inbox"
}
