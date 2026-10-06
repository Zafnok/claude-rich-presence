package host

// The timing constants, for tests that move the fake clock by them, and the
// pure functions.
const (
	RetryBase      = retryBase
	RetryCap       = retryCap
	StandDownDelay = standDownDelay
	GreetTimeout   = greetTimeout
	HasteWait      = hasteWait
	HasteTries     = hasteTries
	QueueSize      = queueSize
)

var (
	Newer      = newer
	Jittered   = jittered
	Supersedes = supersedes
)
