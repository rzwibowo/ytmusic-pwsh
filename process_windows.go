//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
)

type ioCounters struct {
	ReadOperationCount, WriteOperationCount, OtherOperationCount uint64
	ReadTransferCount, WriteTransferCount, OtherTransferCount    uint64
}

type basicLimitInformation struct {
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

type extendedLimitInformation struct {
	BasicLimitInformation basicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

var processJob syscall.Handle

const (
	processTerminate = 0x0001
	processSetQuota  = 0x0100
	processQueryInfo = 0x0400
)

func attachProcessToJob(process *os.Process) error {
	if processJob == 0 {
		handle, _, callErr := procCreateJobObjectW.Call(0, 0)
		if handle == 0 {
			return callErr
		}
		processJob = syscall.Handle(handle)
		info := extendedLimitInformation{}
		info.BasicLimitInformation.LimitFlags = 0x00002000
		ok, _, callErr := procSetInformationJobObject.Call(
			handle, 9, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info),
		)
		if ok == 0 {
			return callErr
		}
	}
	// AssignProcessToJobObject requires PROCESS_SET_QUOTA and
	// PROCESS_TERMINATE access on the target process.
	processHandle, err := syscall.OpenProcess(
		processTerminate|processSetQuota|processQueryInfo,
		false,
		uint32(process.Pid),
	)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(processHandle)
	ok, _, callErr := procAssignProcessToJobObject.Call(uintptr(processJob), uintptr(processHandle))
	if ok == 0 {
		return fmt.Errorf("AssignProcessToJobObject: %w", callErr)
	}
	return nil
}

func hideProcessWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
