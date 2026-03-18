package strategy

// TokenUsage is a value object for token consumption analysis.
type TokenUsage struct {
	UsedTokens       int
	AvailableTokens  int
	UsedRatio        float64
	AnomalyThreshold float64
}

// NewTokenUsage creates a TokenUsage with anomaly detection.
func NewTokenUsage(used, available int, threshold float64) TokenUsage {
	if threshold <= 0 {
		threshold = 0.9
	}
	ratio := 0.0
	if available > 0 {
		ratio = float64(used) / float64(available)
	}
	return TokenUsage{
		UsedTokens:       used,
		AvailableTokens:  available,
		UsedRatio:        ratio,
		AnomalyThreshold: threshold,
	}
}

// IsAnomaly returns true if usage exceeds threshold.
func (t TokenUsage) IsAnomaly() bool {
	return t.UsedRatio >= t.AnomalyThreshold
}
