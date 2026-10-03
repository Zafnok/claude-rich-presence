// Package cli dispatches commands and is the composition root: the only
// package that constructs real operating-system implementations and wires them
// to the rest of the program.
//
// It may import any package under internal except internal/testutil. Nothing
// under internal may import it.
package cli
