package fakediscord

// LeakForTest starts a goroutine that the server waits for and that runs
// until release is called, to stand in for one that was left behind.
func (s *Server) LeakForTest() (release func()) {
	stuck := make(chan struct{})
	s.spawn(func() { <-stuck })
	return func() { close(stuck) }
}

// ReleaseForTest frees the endpoint after a shutdown that gave up waiting,
// which leaves it held.
func (s *Server) ReleaseForTest() {
	s.wg.Wait()
	_ = s.ln.Close()
}
