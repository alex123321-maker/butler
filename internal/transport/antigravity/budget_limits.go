package antigravity

// InferenceLimits declares the fixed native request allowance. Butler reserves
// the entire context limit rather than guessing hidden provider tokenization.
// Both AICode tiered and explicit Cloud Code High use the same output allowance.
// Credit overage, account rotation and model fallback remain unavailable.
func (*Provider) InferenceLimits() (int64, int64) { return 1048576, maxOutputTokens }
