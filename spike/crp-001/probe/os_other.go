//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func parentInfo() []map[string]any {
	var chain []map[string]any
	pid := os.Getppid()
	for i := 0; i < 4 && pid > 1; i++ {
		out, err := exec.Command("ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			break
		}
		f := strings.Fields(string(out))
		if len(f) < 2 {
			break
		}
		chain = append(chain, map[string]any{"pid": pid, "name": strings.Join(f[1:], " ")})
		pid, _ = strconv.Atoi(f[0])
	}
	return chain
}

func consoleInfo() map[string]any {
	return map[string]any{"note": fmt.Sprint("not applicable")}
}
