package actionauth

func isMemoryRead(tool string) bool {
	return tool == "memory.search" || tool == "memory.open" || tool == "memory.neighbors"
}
