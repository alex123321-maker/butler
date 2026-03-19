package embeddings

import "sync"

// defaultVectorDimensions is the OpenAI text-embedding-3-small default.
const defaultVectorDimensions = 1536

var (
	vectorDimsMu   sync.RWMutex
	vectorDimsOnce sync.Once
	vectorDims     = defaultVectorDimensions
)

// VectorDimensions returns the configured embedding vector dimension count.
// The value is set once at startup via SetVectorDimensions and is immutable
// after that. Defaults to 1536 (OpenAI text-embedding-3-small).
func VectorDimensions() int {
	vectorDimsMu.RLock()
	defer vectorDimsMu.RUnlock()
	return vectorDims
}

// SetVectorDimensions sets the embedding vector dimension count. It can only
// be called once (at startup); subsequent calls are silently ignored.
func SetVectorDimensions(dims int) {
	if dims <= 0 {
		return
	}
	vectorDimsOnce.Do(func() {
		vectorDimsMu.Lock()
		vectorDims = dims
		vectorDimsMu.Unlock()
	})
}
