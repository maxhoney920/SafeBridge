//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	createNoWindow                  = 0x08000000
	jobObjectExtendedLimitInfoClass = 9
	jobObjectLimitKillOnJobClose    = 0x2000
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	jobHandle                    uintptr
)

type ioCounters struct {
	ReadOperationCount, WriteOperationCount, OtherOperationCount uint64
	ReadTransferCount, WriteTransferCount, OtherTransferCount    uint64
}

type jobBasicLimit struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobExtendedLimit struct {
	BasicLimitInformation jobBasicLimit
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// Job Object с KILL_ON_JOB_CLOSE: ядро гарантированно умирает вместе с помощником.
func ensureJob() uintptr {
	if jobHandle != 0 {
		return jobHandle
	}
	h, _, _ := procCreateJobObjectW.Call(0, 0)
	if h == 0 {
		return 0
	}
	var info jobExtendedLimit
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	r, _, _ := procSetInformationJobObject.Call(h, jobObjectExtendedLimitInfoClass,
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	if r == 0 {
		syscall.CloseHandle(syscall.Handle(h))
		return 0
	}
	jobHandle = h
	return h
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func prepareChild(cmd *exec.Cmd) { hideWindow(cmd) }

func afterStart(cmd *exec.Cmd) {
	job := ensureJob()
	if job == 0 || cmd.Process == nil {
		return
	}
	const access = 0x0001 | 0x0100 // PROCESS_TERMINATE | PROCESS_SET_QUOTA
	ph, err := syscall.OpenProcess(access, false, uint32(cmd.Process.Pid))
	if err != nil {
		return
	}
	defer syscall.CloseHandle(ph)
	procAssignProcessToJobObject.Call(job, uintptr(ph))
}

func terminate(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}

func killStaleCore(pidFile string) {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	os.Remove(pidFile)
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return
	}
	c := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH")
	hideWindow(c)
	out, err := c.Output()
	if err != nil || !strings.Contains(strings.ToLower(string(out)), "sing-box") {
		return
	}
	if p, err := os.FindProcess(pid); err == nil {
		p.Kill()
	}
}
