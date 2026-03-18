package shared

import "time"

// Clock abstracts time access for domain and application services.
// It allows deterministic tests and avoids hard-coding time.Now().

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

