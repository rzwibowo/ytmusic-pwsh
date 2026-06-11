//go:build windows

package main

import (
	"os"
	"syscall"
	"time"
	"unsafe"
)

const (
	stdInputHandle  = ^uintptr(9)
	stdOutputHandle = ^uintptr(10)
	keyEventType    = 0x0001
)

var (
	procGetStdHandle                  = kernel32.NewProc("GetStdHandle")
	procGetConsoleMode                = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode                = kernel32.NewProc("SetConsoleMode")
	procGetNumberOfConsoleInputEvents = kernel32.NewProc("GetNumberOfConsoleInputEvents")
	procReadConsoleInputW             = kernel32.NewProc("ReadConsoleInputW")
	procGetConsoleScreenBufferInfo    = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

type inputRecord struct {
	EventType uint16
	Padding   uint16
	KeyDown   int32
	Repeat    uint16
	Virtual   uint16
	ScanCode  uint16
	Char      uint16
	Control   uint32
	Padding2  [8]byte
}

type coord struct{ X, Y int16 }
type smallRect struct{ Left, Top, Right, Bottom int16 }
type screenBufferInfo struct {
	Size, CursorPosition coord
	Attributes           uint16
	Window               smallRect
	MaximumWindowSize    coord
}

type consoleInput struct {
	handle syscall.Handle
	mode   uint32
}

func openConsoleInput() (*consoleInput, error) {
	handle, _, err := procGetStdHandle.Call(stdInputHandle)
	if handle == 0 || handle == ^uintptr(0) {
		return nil, err
	}
	var mode uint32
	ok, _, err := procGetConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if ok == 0 {
		return nil, err
	}
	newMode := mode &^ uint32(0x0002|0x0004)
	newMode |= 0x0001 | 0x0008
	if ok, _, err = procSetConsoleMode.Call(handle, uintptr(newMode)); ok == 0 {
		return nil, err
	}
	out, _, _ := procGetStdHandle.Call(stdOutputHandle)
	var outMode uint32
	if ok, _, _ := procGetConsoleMode.Call(out, uintptr(unsafe.Pointer(&outMode))); ok != 0 {
		_, _, _ = procSetConsoleMode.Call(out, uintptr(outMode|0x0004))
	}
	return &consoleInput{handle: syscall.Handle(handle), mode: mode}, nil
}

func (c *consoleInput) close() {
	_, _, _ = procSetConsoleMode.Call(uintptr(c.handle), uintptr(c.mode))
}

func (c *consoleInput) readKey(timeout time.Duration) (keyPress, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var count uint32
		ok, _, _ := procGetNumberOfConsoleInputEvents.Call(uintptr(c.handle), uintptr(unsafe.Pointer(&count)))
		if ok == 0 || count == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		var record inputRecord
		var read uint32
		ok, _, _ = procReadConsoleInputW.Call(
			uintptr(c.handle), uintptr(unsafe.Pointer(&record)), 1, uintptr(unsafe.Pointer(&read)),
		)
		if ok == 0 || read == 0 || record.EventType != keyEventType || record.KeyDown == 0 {
			continue
		}
		return keyPress{virtual: record.Virtual, char: rune(record.Char)}, true
	}
	return keyPress{}, false
}

func terminalWidth() int {
	handle := os.Stdout.Fd()
	var info screenBufferInfo
	ok, _, _ := procGetConsoleScreenBufferInfo.Call(handle, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return 120
	}
	return int(info.Window.Right-info.Window.Left) + 1
}
