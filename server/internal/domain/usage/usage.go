package usage

type Snapshot struct {
	UserID          string
	ProjectID       string
	TotalTokensUsed int64
	TotalTokensLimit int64
}

type Prediction struct {
	CanContinue       bool
	ExpectedRemaining float64
	UsageRatio        float64
}

func Predict(s Snapshot) Prediction {
	if s.TotalTokensLimit == 0 {
		return Prediction{
			CanContinue:       true,
			ExpectedRemaining: 0,
			UsageRatio:        0,
		}
	}
	ratio := float64(s.TotalTokensUsed) / float64(s.TotalTokensLimit)
	return Prediction{
		CanContinue:       ratio < 1.0,
		ExpectedRemaining: 1.0 - ratio,
		UsageRatio:        ratio,
	}
}

// PlanLimit represents per-plan token limits or other usage constraints.
type PlanLimit struct {
	Name           string
	TotalTokenCap  int64
	WarnThreshold  float64
	HardStopFactor float64
}


