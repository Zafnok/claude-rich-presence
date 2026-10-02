package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	user32                         = syscall.NewLazyDLL("user32.dll")
	procGetFinalPathNameByHandleW  = kernel32.NewProc("GetFinalPathNameByHandleW")
	procGetCurrentPackageFullName  = kernel32.NewProc("GetCurrentPackageFullName")
	procIsProcessInJob             = kernel32.NewProc("IsProcessInJob")
	procQueryInformationJobObject  = kernel32.NewProc("QueryInformationJobObject")
	procGetConsoleWindow           = kernel32.NewProc("GetConsoleWindow")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procProcessIdToSessionId       = kernel32.NewProc("ProcessIdToSessionId")
	procIsWindowVisible            = user32.NewProc("IsWindowVisible")
)

func candidates() []candidate {
	const leaf = "rich-presence-crp002"
	home, _ := os.UserHomeDir()
	return []candidate{
		{"localappdata", filepath.Join(os.Getenv("LOCALAPPDATA"), leaf)}, // ADR-0006 first choice
		{"temp", filepath.Join(os.TempDir(), leaf)},                      // ADR-0006 second choice
		{"home", filepath.Join(home, "."+leaf)},                          // outside AppData
	}
}

// tryLock opens the lock file with no sharing, which is the exclusive lock of
// ADR-0005 on Windows. The handle is deliberately never closed.
func tryLock(path string) (string, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return "", err
	}
	return finalPathOfHandle(h), nil
}

func finalPathOfHandle(h syscall.Handle) string {
	buf := make([]uint16, 1024)
	n, _, _ := procGetFinalPathNameByHandleW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if n == 0 || int(n) > len(buf) {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// finalPath reports where a path really is once redirection is resolved.
func finalPath(path string) string {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	const backupSemantics = 0x02000000 // needed to open a directory
	h, err := syscall.CreateFile(p, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, backupSemantics, 0)
	if err != nil {
		return "error: " + err.Error()
	}
	defer syscall.CloseHandle(h)
	return finalPathOfHandle(h)
}

type jobExtendedLimits struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
	IoInfo                  [6]uint64
	ProcessMemoryLimit      uintptr
	JobMemoryLimit          uintptr
	PeakProcessMemoryUsed   uintptr
	PeakJobMemoryUsed       uintptr
}

func sysFacts() fields {
	f := fields{}

	// Package identity. 15700 means the process has none.
	n := uint32(1024)
	buf := make([]uint16, n)
	rc, _, _ := procGetCurrentPackageFullName.Call(uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])))
	f["packageRc"] = rc
	f["packageFullName"] = syscall.UTF16ToString(buf)

	// Job membership, and the limits of the innermost job.
	self, _ := syscall.GetCurrentProcess()
	var inJob int32
	procIsProcessInJob.Call(uintptr(self), 0, uintptr(unsafe.Pointer(&inJob)))
	f["inJob"] = inJob != 0
	if inJob != 0 {
		var info jobExtendedLimits
		const jobObjectExtendedLimitInformation = 9
		ok, _, err := procQueryInformationJobObject.Call(0, jobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0)
		if ok == 0 {
			f["jobQueryErr"] = err.Error()
		} else {
			f["jobLimitFlags"] = fmt.Sprintf("0x%x", info.LimitFlags)
			f["jobKillOnClose"] = info.LimitFlags&0x2000 != 0
			f["jobBreakawayOk"] = info.LimitFlags&0x800 != 0
			f["jobSilentBreakawayOk"] = info.LimitFlags&0x1000 != 0
		}
	}

	// Console. A console program started without a hidden-window flag gets a visible one.
	hwnd, _, _ := procGetConsoleWindow.Call()
	f["consoleWindow"] = hwnd != 0
	if hwnd != 0 {
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		f["consoleVisible"] = visible != 0
	}

	types := map[uint32]string{0: "unknown", 1: "disk", 2: "char", 3: "pipe"}
	stdio := fields{}
	for name, fh := range map[string]*os.File{"stdin": os.Stdin, "stdout": os.Stdout, "stderr": os.Stderr} {
		t, _ := syscall.GetFileType(syscall.Handle(fh.Fd()))
		stdio[name] = types[t]
	}
	f["stdio"] = stdio

	var session uint32
	procProcessIdToSessionId.Call(uintptr(os.Getpid()), uintptr(unsafe.Pointer(&session)))
	f["sessionId"] = session

	// Mark of the Web on our own binary: did the downloaded bundle's zone carry over?
	if exe, err := os.Executable(); err == nil {
		if zone, err := os.ReadFile(exe + ":Zone.Identifier"); err == nil {
			f["zoneIdentifier"] = string(zone)
		} else {
			f["zoneIdentifier"] = "none"
		}
	}

	f["ancestry"] = ancestry()
	return f
}

// ancestry walks parent process ids up to the root, with image paths where
// they can be read. Process names only; no windows, no command lines.
func ancestry() []string {
	parents := map[uint32]uint32{}
	names := map[uint32]string{}
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return []string{"error: " + err.Error()}
	}
	defer syscall.CloseHandle(snap)
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		parents[e.ProcessID] = e.ParentProcessID
		names[e.ProcessID] = syscall.UTF16ToString(e.ExeFile[:])
	}
	chain := []string{}
	pid := uint32(os.Getpid())
	for depth := 0; depth < 10; depth++ {
		name, ok := names[pid]
		if !ok {
			chain = append(chain, fmt.Sprintf("%d <gone>", pid))
			break
		}
		if full := imagePath(pid); full != "" {
			name = full
		}
		chain = append(chain, fmt.Sprintf("%d %s", pid, name))
		pid = parents[pid]
		if pid == 0 {
			break
		}
	}
	return chain
}

func imagePath(pid uint32) string {
	const queryLimited = 0x1000
	h, err := syscall.OpenProcess(queryLimited, false, pid)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	ok, _, _ := procQueryFullProcessImageNameW.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

func openDiscord() (io.ReadWriteCloser, string, []string) {
	attempts := []string{}
	for n := 0; n < 10; n++ {
		name := fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, n)
		fh, err := os.OpenFile(name, os.O_RDWR, 0)
		if err == nil {
			return fh, name, attempts
		}
		attempts = append(attempts, fmt.Sprintf("%d: %v", n, err))
	}
	return nil, "", attempts
}
