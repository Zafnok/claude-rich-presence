//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

func procName(pid uint32) (name string, parent uint32) {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", 0
	}
	defer syscall.CloseHandle(snap)
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		if e.ProcessID == pid {
			return syscall.UTF16ToString(e.ExeFile[:]), e.ParentProcessID
		}
	}
	return "", 0
}

// parentInfo walks up to four ancestors so we can see whether a shell sits between Claude Code and us.
func parentInfo() []map[string]any {
	var chain []map[string]any
	pid := uint32(os.Getppid())
	for i := 0; i < 4 && pid != 0; i++ {
		name, pp := procName(pid)
		if name == "" {
			break
		}
		chain = append(chain, map[string]any{"pid": pid, "name": name})
		pid = pp
	}
	return chain
}

func consoleInfo() map[string]any {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	u32 := syscall.NewLazyDLL("user32.dll")
	hwnd, _, _ := k32.NewProc("GetConsoleWindow").Call()
	visible := false
	if hwnd != 0 {
		v, _, _ := u32.NewProc("IsWindowVisible").Call(hwnd)
		visible = v != 0
	}
	return map[string]any{"has_console_window": hwnd != 0, "window_visible": visible}
}
