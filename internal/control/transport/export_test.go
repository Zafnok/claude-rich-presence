package transport

// Unexported pieces that the tests in package transport_test reach directly.
var (
	CheckDir          = checkDir
	ClassifyDialError = classifyDialError
	DialAs            = dial
)

const (
	ErrRefused          = errRefused
	ErrNoDir            = errNoDir
	MaxSocketPathDarwin = maxSocketPathDarwin
	MaxSocketPathOther  = maxSocketPathOther
)
