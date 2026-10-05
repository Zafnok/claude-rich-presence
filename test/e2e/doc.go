// Package e2e holds the end-to-end tests: black-box tests of the built
// binary, run as real processes on the real operating system, with only
// Claude and Discord replaced by fakes (CRP-043).
//
// It has no code of its own. The tests build cmd/rich-presence once, with
// coverage instrumentation, and start as many copies as a scenario needs.
// Each scenario has a runtime directory, a home directory and a Discord
// endpoint name of its own, so the scenarios run in parallel, never meet a
// presence host or a Discord that is running on the machine, and leave
// nothing behind.
//
// # Coverage
//
// The processes write coverage data to the directory named by the
// environment variable RICH_PRESENCE_E2E_COVERDIR, which must be an absolute
// path, and to a directory that is thrown away when it is not set. The gate
// merges that data with the unit-test profile (CONTRIBUTING.md). A process
// that a scenario kills writes none.
//
// # Time
//
// The scenarios run on real time, with the interval between Discord updates
// set to its floor through the configuration. Nothing waits a fixed time for
// a state: each wait polls a condition, under a deadline that is a watchdog
// and not a delay.
package e2e
