//go:build windows

package ui

import (
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/event"
	"golang.org/x/sys/windows"
)

var (
	dwmapi                = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttrs = dwmapi.NewProc("DwmSetWindowAttribute")
)

const dwmwaUseImmersiveDarkMode = 20

// handleViewEvent gives the window a dark title bar, like the old tool.
func handleViewEvent(e event.Event) {
	v, ok := e.(app.Win32ViewEvent)
	if !ok || v.HWND == 0 {
		return
	}
	on := int32(1)
	// Older Windows versions lack the attribute; the call then fails harmlessly.
	_, _, _ = procDwmSetWindowAttrs.Call(v.HWND, dwmwaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&on)), unsafe.Sizeof(on))
}
