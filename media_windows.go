//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

// Global media keys via RegisterHotKey, so Play/Pause works even when the
// terminal window is not focused. A tray icon is not required for this.
const (
	wmHotKey         = 0x0312
	modNoRepeat      = 0x4000
	vkMediaPlayPause = 0xB3
	vkMediaStop      = 0xB2
	vkMediaNextTrack = 0xB0
	vkMediaPrevTrack = 0xB1
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procPeekMessageW     = user32.NewProc("PeekMessageW")
	procGetMessageW      = user32.NewProc("GetMessageW")
)

// Windows MSG; field order matches 32-bit and 64-bit layouts.
type msg struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       struct{ X, Y int32 }
	lPrivate uint32
}

type mediaHotKey struct {
	id   int
	vk   uintptr
	name string
	cmd  string
}

// listenMediaKeys registers the media keys and pumps WM_HOTKEY messages.
// Commands are sent through the channel into the same pipeline as console
// input, so playback state is never touched from two goroutines at once.
func listenMediaKeys(commands chan<- string) {
	runtime.LockOSThread() // RegisterHotKey and GetMessage must share one thread
	var queue msg
	_, _, _ = procPeekMessageW.Call(uintptr(unsafe.Pointer(&queue)), 0, 0, 0, 0)
	keys := []mediaHotKey{
		{1, vkMediaPlayPause, "Play/Pause", "__toggle_playback"},
		{2, vkMediaStop, "Stop", "stop"},
		{3, vkMediaNextTrack, "Next Track", "next"},
		{4, vkMediaPrevTrack, "Previous Track", "prev"},
	}
	registered := 0
	for _, key := range keys {
		ok, _, _ := procRegisterHotKey.Call(0, uintptr(key.id), modNoRepeat, key.vk)
		if ok == 0 {
			fmt.Printf("Media key %s is already used by another app, skipped\n", key.name)
			continue
		}
		registered++
	}
	if registered == 0 {
		return
	}
	defer func() {
		for _, key := range keys {
			_, _, _ = procUnregisterHotKey.Call(0, uintptr(key.id))
		}
	}()
	for {
		var message msg
		result, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if result == 0 { // WM_QUIT
			return
		}
		if message.message != wmHotKey {
			continue
		}
		for _, key := range keys {
			if int(message.wParam) == key.id {
				commands <- key.cmd
				break
			}
		}
	}
}