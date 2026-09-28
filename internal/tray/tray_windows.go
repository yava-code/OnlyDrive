//go:build windows

// Package tray shows a notification-area icon for gd with a small menu:
// open the control panel, pause (unmount disks and stop the daemon) or
// resume, and quit. It is implemented directly over user32/shell32 through
// the stdlib syscall package, so no GUI dependency is added.
package tray

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	wmApp            = 0x8000
	wmTrayCallback   = wmApp + 1
	wmClose          = 0x0010
	wmDestroy        = 0x0002
	wmAppTrayRefresh = wmApp + 2

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2

	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04

	imageIcon = 1
	lrShared  = 0x8000

	smCxsmicon = 49
	smCysmicon = 50

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	mfString = 0x0000
	mfGrayed = 0x0001
	mfSep    = 0x0800

	idmOpen   = 100
	idmPause  = 101
	idmResume = 102
	idmQuit   = 103

	wmLButtonUp   = 0x0202
	wmRButtonUp   = 0x0205
	wmContextMenu = 0x007B
	ninSelect     = 0x0400
)

// Callbacks wire the tray menu to the rest of the app. All are optional:
// a nil callback is skipped and its menu item left out. Pause and Resume
// are treated as succeeded only when they return nil.
type Callbacks struct {
	Status func() string // one-line tooltip / grayed menu status
	Open   func() error  // open the control panel in a browser
	Pause  func() error  // unmount disks and stop the daemon
	Resume func() error  // start the daemon and re-mount the disks
	Quit   func()        // stop the app
}

type Tray struct {
	cb     Callbacks
	hwnd   uintptr
	hIcon  uintptr
	nid    notifyIconData
	paused bool
}

var current *Tray

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procLoadImageW          = user32.NewProc("LoadImageW")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procExtractIconW     = shell32.NewProc("ExtractIconW")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  uintptr
	LpszClassName uintptr
	HIconSm       uintptr
}

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PtX     int32
	PtY     int32
}

type point struct{ X, Y int32 }

type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

// wndProcPtr is created once: syscall.NewCallback has a process-wide budget.
var wndProcPtr = syscall.NewCallback(wndProc)

// Run shows the tray icon and blocks in the Win32 message loop until Quit
// or Stop. Returns an error when the tray cannot be created so the caller
// can fall back to a tray-less mode.
func Run(cb Callbacks) error {
	t := &Tray{cb: cb}
	if err := t.start(); err != nil {
		return err
	}
	current = t
	t.loop()
	return nil
}

// Stop removes the tray icon and ends the Run message loop. Safe to call
// from any goroutine, including callbacks and signal handlers.
func Stop() {
	if current == nil {
		return
	}
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&current.nid)))
	if current.hwnd != 0 {
		procPostMessageW.Call(current.hwnd, wmClose, 0, 0)
	}
}

func (t *Tray) start() error {
	className, err := syscall.UTF16PtrFromString("gd-tray")
	if err != nil {
		return err
	}
	hInst, _, _ := procGetModuleHandleW.Call(0)
	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   wndProcPtr,
		HInstance:     hInst,
		LpszClassName: uintptr(unsafe.Pointer(className)),
	}
	if atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		return fmt.Errorf("RegisterClassExW: %v", callErr)
	}
	windowName, err := syscall.UTF16PtrFromString("gd tray window")
	if err != nil {
		return err
	}
	// A hidden top-level window: the tray callback messages need an hwnd,
	// but nothing is ever shown.
	hwnd, _, callErr := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0, 0, 0, 0, 0, 0, 0, hInst, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW: %v", callErr)
	}
	t.hwnd = hwnd
	t.hIcon = loadAppIcon(hInst)

	nid := notifyIconData{
		CbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:             hwnd,
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: wmTrayCallback,
		HIcon:            t.hIcon,
	}
	setTipBuf(&nid.SzTip, t.tipText())
	if ok, _, callErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid))); ok == 0 {
		return fmt.Errorf("Shell_NotifyIconW: %v", callErr)
	}
	t.nid = nid
	return nil
}

func (t *Tray) loop() {
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r == 0 || r == ^uintptr(0) { // WM_QUIT or error
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (t *Tray) tipText() string {
	if t.cb.Status != nil {
		if s := t.cb.Status(); s != "" {
			return s
		}
	}
	return "gd"
}

func (t *Tray) setTip() {
	setTipBuf(&t.nid.SzTip, t.tipText())
	t.nid.UFlags |= nifTip
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))
}

// RefreshStatus re-renders the tooltip from the Status callback. Safe to
// call from any goroutine: the work is posted to the tray window so the
// Shell_NotifyIconW call happens on the message-loop thread.
func RefreshStatus() {
	if current == nil || current.hwnd == 0 {
		return
	}
	procPostMessageW.Call(current.hwnd, wmAppTrayRefresh, 0, 0)
}

func setTipBuf(dst *[128]uint16, s string) {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return
	}
	if len(u) > 128 {
		u = u[:128]
		u[len(u)-1] = 0
	}
	copy(dst[:], u)
}

func wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	if t := current; t != nil {
		switch message {
		case wmTrayCallback:
			t.onTray(lParam)
			return 0
		case wmAppTrayRefresh:
			t.setTip()
			return 0
		case wmClose:
			procDestroyWindow.Call(hwnd)
			return 0
		case wmDestroy:
			procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&t.nid)))
			procPostQuitMessage.Call(0)
			return 0
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return r
}

func (t *Tray) onTray(lParam uintptr) {
	switch lParam & 0xFFFF {
	case wmLButtonUp, ninSelect:
		// Left click: straight to the panel when there is one to open.
		if t.cb.Open != nil {
			_ = t.cb.Open()
			return
		}
		t.showMenu()
	case wmRButtonUp, wmContextMenu:
		t.showMenu()
	}
}

func (t *Tray) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	addItem := func(flags, id uintptr, text string) {
		p, err := syscall.UTF16PtrFromString(text)
		if err != nil {
			return
		}
		procAppendMenuW.Call(menu, flags, id, uintptr(unsafe.Pointer(p)))
	}
	if s := t.tipText(); s != "gd" || t.cb.Status != nil {
		addItem(mfGrayed|mfString, 0, s)
		addItem(mfSep, 0, "")
	}
	if t.cb.Open != nil {
		addItem(mfString, idmOpen, "Open panel")
	}
	if t.cb.Pause != nil && !t.paused {
		addItem(mfString, idmPause, "Pause disks (unmount, stop daemon)")
	}
	if t.cb.Resume != nil && t.paused {
		addItem(mfString, idmResume, "Resume disks (start daemon, mount)")
	}
	addItem(mfSep, 0, "")
	addItem(mfString, idmQuit, "Quit")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	// TrackPopupMenu requires the foreground window, and a WM_NULL after,
	// or the menu does not dismiss on an outside click.
	procSetForegroundWindow.Call(t.hwnd)
	sel, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmReturnCmd,
		uintptr(int32(pt.X)), uintptr(int32(pt.Y)), 0, t.hwnd, 0)
	procPostMessageW.Call(t.hwnd, 0 /* WM_NULL */, 0, 0)

	switch sel {
	case idmOpen:
		if t.cb.Open != nil {
			_ = t.cb.Open()
		}
	case idmPause:
		if t.cb.Pause != nil && t.cb.Pause() == nil {
			t.paused = true
			t.setTip()
		}
	case idmResume:
		if t.cb.Resume != nil && t.cb.Resume() == nil {
			t.paused = false
			t.setTip()
		}
	case idmQuit:
		if t.cb.Quit != nil {
			t.cb.Quit()
		} else {
			procPostQuitMessage.Call(0)
		}
	}
}

// loadAppIcon prefers the .ico linked into the exe as a resource (see
// cmd/*/rsrc_windows_amd64.syso) and falls back to the generic application
// icon for local builds built without the resource file.
func loadAppIcon(hInst uintptr) uintptr {
	cx, _, _ := procGetSystemMetrics.Call(smCxsmicon)
	cy, _, _ := procGetSystemMetrics.Call(smCysmicon)
	if h, _, _ := procLoadImageW.Call(hInst, 1, imageIcon, cx, cy, lrShared); h != 0 {
		return h
	}
	if exe, err := os.Executable(); err == nil {
		if p, err := syscall.UTF16PtrFromString(exe); err == nil {
			if h, _, _ := procExtractIconW.Call(hInst, uintptr(unsafe.Pointer(p)), 0); h != 0 && h != ^uintptr(0) {
				return h
			}
		}
	}
	h, _, _ := procLoadIconW.Call(0, 32512) // IDI_APPLICATION
	return h
}
