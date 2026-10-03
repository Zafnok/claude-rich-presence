package fakediscord

import "time"

// LeakForTest starts a goroutine that the server waits for and that runs
// until release is called, to stand in for one that was left behind.
func (s *Server) LeakForTest() (release func()) {
	stuck := make(chan struct{})
	s.spawn(func() { <-stuck })
	return func() { close(stuck) }
}

// ExpirePatienceForTest makes the server's patience run out at once, so that
// a test of running out of it does not wait on real time.
func (s *Server) ExpirePatienceForTest() {
	expired := make(chan time.Time)
	close(expired)
	s.timer = func(time.Duration) <-chan time.Time { return expired }
}
