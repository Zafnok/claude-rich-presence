package diag_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
)

func TestCounters(t *testing.T) {
	var c diag.Counters
	if got := c.Snapshot(); got != (diag.Counts{}) {
		t.Errorf("new counters = %+v", got)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				c.EventReceived()
			}
			for j := 0; j < 3; j++ {
				c.EventDropped()
			}
			for j := 0; j < 2; j++ {
				c.Reconnected()
			}
			c.FailedOver()
			for j := 0; j < 5; j++ {
				c.ConnectionRejected()
			}
		}()
	}
	wg.Wait()
	want := diag.Counts{EventsReceived: 32, EventsDropped: 24, Reconnects: 16, Failovers: 8, ConnectionsRejected: 40}
	if got := c.Snapshot(); got != want {
		t.Errorf("Snapshot = %+v, want %+v", got, want)
	}

	var out lines
	diag.NewLogger(&out, config.LogInfo, now).Info("counters", want.Attrs()...)
	if suffix := "events_received=32 events_dropped=24 reconnects=16 failovers=8 connections_rejected=40\n"; !strings.HasSuffix(out.String(), suffix) {
		t.Errorf("logged %q, want it to end %q", out.String(), suffix)
	}
}
