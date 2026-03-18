package task

type Status int32

const (
	StatusUnknown Status = iota
	StatusCreated
	StatusInProgress
	StatusDone
	StatusStopped
	StatusHardStopped
	StatusCancelled
	StatusRedistributed
)

