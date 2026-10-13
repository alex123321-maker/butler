package antigravity

// InferenceLimits declares the fixed native request allowance. Butler reserves
// the entire context limit rather than guessing hidden provider tokenization.
// Default AICode and Cloud Code requests retain8192. An explicitly configured
// High ceiling includes provider thinking and is fully reserved by the kernel.
// Credit overage, account rotation and model fallback remain unavailable.
func (p *Provider) InferenceLimits() (int64, int64) {
	return 1048576, int64(p.config.MaximumOutputTokens)
}
