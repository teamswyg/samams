package strategy

import "server/internal/domain/shared"

// RepeatedFailureTracker counts consecutive task failures per subject.
type RepeatedFailureTracker struct {
	SubjectID string
	DoneCount int
	Threshold int
	events    []shared.DomainEvent
}

// NewRepeatedFailureTracker creates a tracker with a given threshold.
func NewRepeatedFailureTracker(subjectID string, threshold int) *RepeatedFailureTracker {
	if threshold <= 0 {
		threshold = 5
	}
	return &RepeatedFailureTracker{
		SubjectID: subjectID,
		Threshold: threshold,
	}
}

// RecordDone increments the failure count and checks threshold.
func (t *RepeatedFailureTracker) RecordDone(clock shared.Clock) {
	t.DoneCount++
	if t.DoneCount >= t.Threshold {
		t.addEvent(clock, "RepeatedFailureDetected", map[string]any{
			"subject_id": t.SubjectID,
			"done_count": t.DoneCount,
			"threshold":  t.Threshold,
		})
	}
}

// IsThresholdReached returns true if the failure count has met or exceeded the threshold.
func (t *RepeatedFailureTracker) IsThresholdReached() bool {
	return t.DoneCount >= t.Threshold
}

// Reset clears the counter.
func (t *RepeatedFailureTracker) Reset() {
	t.DoneCount = 0
}

// PullDomainEvents returns and clears pending events.
func (t *RepeatedFailureTracker) PullDomainEvents() []shared.DomainEvent {
	out := make([]shared.DomainEvent, len(t.events))
	copy(out, t.events)
	t.events = t.events[:0]
	return out
}

func (t *RepeatedFailureTracker) addEvent(clock shared.Clock, name string, payload any) {
	t.events = append(t.events, shared.NewDomainEvent(
		clock, name, "strategy", t.SubjectID, "failure_tracker", t.SubjectID, "warning", payload,
	))
}
