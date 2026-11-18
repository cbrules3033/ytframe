//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// ---- DLLs ----------------------------------------------------------------

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	shcore   = syscall.NewLazyDLL("shcore.dll")
)

var (
	pRegisterClassExW       = user32.NewProc("RegisterClassExW")
	pCreateWindowExW        = user32.NewProc("CreateWindowExW")
	pDefWindowProcW         = user32.NewProc("DefWindowProcW")
	pGetMessageW            = user32.NewProc("GetMessageW")
	pTranslateMessage       = user32.NewProc("TranslateMessage")
	pDispatchMessageW       = user32.NewProc("DispatchMessageW")
	pPostQuitMessage        = user32.NewProc("PostQuitMessage")
	pDestroyWindow          = user32.NewProc("DestroyWindow")
	pShowWindow             = user32.NewProc("ShowWindow")
	pSetWindowPos           = user32.NewProc("SetWindowPos")
	pGetWindowRect          = user32.NewProc("GetWindowRect")
	pGetClientRect          = user32.NewProc("GetClientRect")
	pSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	pGetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	pSetFocus               = user32.NewProc("SetFocus")
	pGetFocus               = user32.NewProc("GetFocus")
	pSendMessageW           = user32.NewProc("SendMessageW")
	pPostMessageW           = user32.NewProc("PostMessageW")
	pLoadCursorW            = user32.NewProc("LoadCursorW")
	pLoadIconW              = user32.NewProc("LoadIconW")
	pLoadImageW             = user32.NewProc("LoadImageW")
	pSetCursor              = user32.NewProc("SetCursor")
	pRegisterHotKey         = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey       = user32.NewProc("UnregisterHotKey")
	pGetCursorPos           = user32.NewProc("GetCursorPos")
	pTrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	pCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	pAppendMenuW            = user32.NewProc("AppendMenuW")
	pDestroyMenu            = user32.NewProc("DestroyMenu")
	pSetWindowLongPtrW      = user32.NewProc("SetWindowLongPtrW")
	pCallWindowProcW        = user32.NewProc("CallWindowProcW")
	pSetTimer               = user32.NewProc("SetTimer")
	pKillTimer              = user32.NewProc("KillTimer")
	pInvalidateRect         = user32.NewProc("InvalidateRect")
	pGetDC                  = user32.NewProc("GetDC")
	pReleaseDC              = user32.NewProc("ReleaseDC")
	pBeginPaint             = user32.NewProc("BeginPaint")
	pEndPaint               = user32.NewProc("EndPaint")
	pFillRect               = user32.NewProc("FillRect")
	pDrawTextW              = user32.NewProc("DrawTextW")
	pOpenClipboard          = user32.NewProc("OpenClipboard")
	pCloseClipboard         = user32.NewProc("CloseClipboard")
	pGetClipboardData       = user32.NewProc("GetClipboardData")
	pIsClipboardFormatAvail = user32.NewProc("IsClipboardFormatAvailable")
	pEnumWindows            = user32.NewProc("EnumWindows")
	pFindWindowW            = user32.NewProc("FindWindowW")
	pGetWindowThreadProcID  = user32.NewProc("GetWindowThreadProcessId")
	pIsWindowVisible        = user32.NewProc("IsWindowVisible")
	pIsWindow               = user32.NewProc("IsWindow")
	pIsIconic               = user32.NewProc("IsIconic")
	pSetWindowTextW         = user32.NewProc("SetWindowTextW")
	pGetWindowTextW         = user32.NewProc("GetWindowTextW")
	pGetWindowTextLengthW   = user32.NewProc("GetWindowTextLengthW")
	pMessageBoxW            = user32.NewProc("MessageBoxW")
	pMonitorFromWindow      = user32.NewProc("MonitorFromWindow")
	pMonitorFromPoint       = user32.NewProc("MonitorFromPoint")
	pGetMonitorInfoW        = user32.NewProc("GetMonitorInfoW")
	pSetWindowRgn           = user32.NewProc("SetWindowRgn")
	pGetDpiForWindow        = user32.NewProc("GetDpiForWindow")
	pGetDpiForSystem        = user32.NewProc("GetDpiForSystem")
	pSetProcessDPIAwareCtx  = user32.NewProc("SetProcessDpiAwarenessContext")
	pSystemParametersInfoW  = user32.NewProc("SystemParametersInfoW")
	pSetWindowLongPtrW32    = user32.NewProc("SetWindowLongW")
	pMoveWindow             = user32.NewProc("MoveWindow")
	pUpdateWindow           = user32.NewProc("UpdateWindow")
	pGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	pReleaseCapture         = user32.NewProc("ReleaseCapture")
	pSetCapture             = user32.NewProc("SetCapture")
	pTrackMouseEvent        = user32.NewProc("TrackMouseEvent")

	pCreateMutexW     = kernel32.NewProc("CreateMutexW")
	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pGlobalLock       = kernel32.NewProc("GlobalLock")
	pGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	pGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	pGlobalFree       = kernel32.NewProc("GlobalFree")
	pCreateFileW      = kernel32.NewProc("CreateFileW")
	pGetLastError     = kernel32.NewProc("GetLastError")
	pGetTickCount     = kernel32.NewProc("GetTickCount")

	pCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	pCreatePen           = gdi32.NewProc("CreatePen")
	pDeleteObject        = gdi32.NewProc("DeleteObject")
	pSelectObject        = gdi32.NewProc("SelectObject")
	pSetTextColor        = gdi32.NewProc("SetTextColor")
	pSetBkColor          = gdi32.NewProc("SetBkColor")
	pSetBkMode           = gdi32.NewProc("SetBkMode")
	pCreateFontW         = gdi32.NewProc("CreateFontW")
	pCreateRoundRectRgn  = gdi32.NewProc("CreateRoundRectRgn")
	pRoundRect           = gdi32.NewProc("RoundRect")
	pGetStockObject      = gdi32.NewProc("GetStockObject")
	pCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBmp = gdi32.NewProc("CreateCompatibleBitmap")
	pBitBlt              = gdi32.NewProc("BitBlt")
	pDeleteDC            = gdi32.NewProc("DeleteDC")
	pCreateDIBSection    = gdi32.NewProc("CreateDIBSection")
	pCreateBitmap        = gdi32.NewProc("CreateBitmap")
	pCreateIconIndirect  = user32.NewProc("CreateIconIndirect")
	pGetDeviceCaps       = gdi32.NewProc("GetDeviceCaps")
	pSetStretchBltMode   = gdi32.NewProc("SetStretchBltMode")

	pShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW    = shell32.NewProc("ShellExecuteW")

	pGetDpiForMonitor = shcore.NewProc("GetDpiForMonitor")
)

// ---- constants -----------------------------------------------------------

const (
	WS_OVERLAPPED    = 0x00000000
	WS_POPUP         = 0x80000000
	WS_CHILD         = 0x40000000
	WS_VISIBLE       = 0x10000000
	WS_CLIPCHILDREN  = 0x02000000
	WS_CLIPSIBLINGS  = 0x04000000
	WS_TABSTOP       = 0x00010000
	WS_EX_TOPMOST    = 0x00000008
	WS_EX_TOOLWINDOW = 0x00000080
	WS_EX_NOACTIVATE = 0x08000000

	ES_LEFT         = 0x0000
	ES_AUTOHSCROLL  = 0x0080
	EM_SETSEL       = 0x00B1
	EM_SETMARGINS   = 0x00D3
	EM_SETCUEBANNER = 0x1501
	WM_SETFONT      = 0x0030
	EC_LEFTMARGIN   = 0x0001
	EC_RIGHTMARGIN  = 0x0002

	SW_HIDE = 0
	SW_SHOW = 5

	SWP_NOSIZE       = 0x0001
	SWP_NOMOVE       = 0x0002
	SWP_NOZORDER     = 0x0004
	SWP_NOACTIVATE   = 0x0010
	SWP_FRAMECHANGED = 0x0020

	WM_DESTROY       = 0x0002
	WM_CREATE        = 0x0001
	WM_SIZE          = 0x0005
	WM_ACTIVATE      = 0x0006
	WM_PAINT         = 0x000F
	WM_CLOSE         = 0x0010
	WM_TIMER         = 0x0113
	WM_KEYDOWN       = 0x0100
	WM_CHAR          = 0x0102
	WM_COMMAND       = 0x0111
	WM_CTLCOLOREDIT  = 0x0133
	WM_MOUSEMOVE     = 0x0200
	WM_LBUTTONDOWN   = 0x0201
	WM_LBUTTONUP     = 0x0202
	WM_LBUTTONDBLCLK = 0x0203
	WM_RBUTTONUP     = 0x0205
	WM_CONTEXTMENU   = 0x007B
	WM_SETCURSOR     = 0x0020
	WM_MOUSELEAVE    = 0x02A3
	WM_DPICHANGED    = 0x02E0
	WM_HOTKEY        = 0x0312
	WM_COPYDATA      = 0x004A
	WM_NCLBUTTONDOWN = 0x00A1
	WM_NULL          = 0x0000
	WM_ERASEBKGND    = 0x0014

	HTCAPTION = 2

	VK_RETURN = 0x0D
	VK_ESCAPE = 0x1B
	VK_V      = 0x56

	MOD_ALT     = 0x0001
	MOD_CONTROL = 0x0002
	MOD_SHIFT   = 0x0004

	CF_UNICODETEXT = 13

	GMEM_MOVEABLE = 0x0002

	GWLP_WNDPROC = ^uintptr(3) // -4

	DT_LEFT         = 0x0000
	DT_CENTER       = 0x0001
	DT_VCENTER      = 0x0004
	DT_SINGLELINE   = 0x0020
	DT_END_ELLIPSIS = 0x8000
	DT_NOPREFIX     = 0x0800

	PS_SOLID          = 0
	NULL_BRUSH        = 5
	NULL_PEN          = 8
	TRANSPARENT       = 1
	CLEARTYPE_QUALITY = 5
	DEFAULT_CHARSET   = 1
	FW_NORMAL         = 400
	FW_SEMIBOLD       = 600
	DEFAULT_PITCH     = 0
	FF_DONTCARE       = 0

	SRCCOPY = 0x00CC0020

	DIB_RGB_COLORS = 0

	MONITOR_DEFAULTTONEAREST = 2

	NIM_ADD     = 0
	NIM_MODIFY  = 1
	NIM_DELETE  = 2
	NIF_MESSAGE = 0x1
	NIF_ICON    = 0x2
	NIF_TIP     = 0x4

	MF_STRING       = 0x0000
	MF_SEPARATOR    = 0x0800
	MF_CHECKED      = 0x0008
	MF_GRAYED       = 0x0001
	TPM_RETURNCMD   = 0x0100
	TPM_RIGHTBUTTON = 0x0002

	IDC_ARROW       = 32512
	IDC_HAND        = 32649
	IDI_APPLICATION = 32512
	IMAGE_ICON      = 1
	LR_DEFAULTSIZE  = 0x40
	LR_SHARED       = 0x8000

	MB_OK              = 0
	MB_ICONERROR       = 0x10
	MB_ICONINFORMATION = 0x40

	ERROR_ALREADY_EXISTS = 183

	TME_LEAVE = 0x00000002

	LOGPIXELSX = 88

	CREATE_NO_WINDOW  = 0x08000000
	FILE_SHARE_READ   = 0x1
	FILE_SHARE_WRITE  = 0x2
	FILE_SHARE_DELETE = 0x4

	WM_APP = 0x8000
)

const (
	WM_APP_TRAY        = WM_APP + 1
	WM_APP_SHOWBAR     = WM_APP + 2
	WM_APP_LOADED      = WM_APP + 3
	WM_APP_ENDFILE     = WM_APP + 4
	WM_APP_MPVEXIT     = WM_APP + 5
	WM_APP_WINDOWFOUND = WM_APP + 6
	WM_APP_UPDATEDONE  = WM_APP + 7
	WM_APP_SNAP        = WM_APP + 8

	TIMER_POLL     = 1
	TIMER_SNAP     = 2
	TIMER_RETRY    = 3
	TIMER_LOADWAIT = 4

	HK_PASTE = 1
)

// ---- structs -------------------------------------------------------------

type POINT struct{ X, Y int32 }

type RECT struct{ Left, Top, Right, Bottom int32 }

type MSG struct {
	HWnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type MONITORINFO struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

type NOTIFYICONDATAW struct {
	CbSize            uint32
	HWnd              uintptr
	UID               uint32
	UFlags            uint32
	UCallbackMessage  uint32
	HIcon             uintptr
	SzTip             [128]uint16
	DwState           uint32
	DwStateMask       uint32
	SzInfo            [256]uint16
	UTimeoutOrVersion uint32
	SzInfoTitle       [64]uint16
	DwInfoFlags       uint32
	GuidItem          syscall.GUID
	HBalloonIcon      uintptr
}

type COPYDATASTRUCT struct {
	DwData uintptr
	CbData uint32
	LpData uintptr
}

type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type BITMAPINFO struct {
	BmiHeader BITMAPINFOHEADER
	BmiColors [1]uint32
}

type ICONINFO struct {
	FIcon    int32
	HbmMask  uintptr
	HbmColor uintptr
}

type TRACKMOUSEEVENT struct {
	CbSize      uint32
	DwFlags     uint32
	HwndTrack   uintptr
	DwHoverTime uint32
}

// ---- helpers -------------------------------------------------------------

func u16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return nil
	}
	return p
}

func rgb(r, g, b uint32) uintptr {
	return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16
}

func loword(v uintptr) int32 { return int32(v & 0xffff) }
func hiword(v uintptr) int32 { return int32((v >> 16) & 0xffff) }
func makeLong(a, b int32) uintptr {
	return uintptr(uint16(a)) | uintptr(uint16(b))<<16
}

func procOK(p *syscall.LazyProc) bool { return p.Find() != nil }

func getModuleHandle() uintptr {
	h, _, _ := pGetModuleHandleW.Call(0)
	return h
}

func initDPI() {
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = (HANDLE)-4
	if pSetProcessDPIAwareCtx.Find() == nil {
		pSetProcessDPIAwareCtx.Call(^uintptr(3))
	}
}

func systemDPI() int32 {
	if pGetDpiForSystem.Find() == nil {
		if dpi, _, _ := pGetDpiForSystem.Call(); dpi > 0 {
			return int32(dpi)
		}
	}
	hdc, _, _ := pGetDC.Call(0)
	if hdc != 0 {
		defer pReleaseDC.Call(0, hdc)
		if dpi, _, _ := pGetDeviceCaps.Call(hdc, LOGPIXELSX); dpi > 0 {
			return int32(dpi)
		}
	}
	return 96
}

func dpiForWindow(hwnd uintptr) int32 {
	if hwnd != 0 && pGetDpiForWindow.Find() == nil {
		if dpi, _, _ := pGetDpiForWindow.Call(hwnd); dpi > 0 {
			return int32(dpi)
		}
	}
	return systemDPI()
}

func monitorWork(hwnd uintptr) RECT {
	mon, _, _ := pMonitorFromWindow.Call(hwnd, MONITOR_DEFAULTTONEAREST)
	if mon == 0 {
		mon, _, _ = pMonitorFromPoint.Call(0, MONITOR_DEFAULTTONEAREST)
	}
	var mi MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	if mon != 0 {
		pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	}
	return mi.RcWork
}

func workAreaForPoint(pt POINT) RECT {
	mon, _, _ := pMonitorFromPoint.Call(uintptr(unsafe.Pointer(&pt)), MONITOR_DEFAULTTONEAREST)
	var mi MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	if mon != 0 {
		pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	}
	return mi.RcWork
}

func rectW(r RECT) int32 { return r.Right - r.Left }
func rectH(r RECT) int32 { return r.Bottom - r.Top }

func clamp(v, lo, hi int32) int32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func containsRect(outer, inner RECT) bool {
	return inner.Left >= outer.Left && inner.Top >= outer.Top &&
		inner.Right <= outer.Right && inner.Bottom <= outer.Bottom
}

func pointIn(r RECT, x, y int32) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

func getWindowText(hwnd uintptr) string {
	n, _, _ := pGetWindowTextLengthW.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

func setWindowText(hwnd uintptr, s string) {
	pSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(u16(s))))
}

func messageBox(owner uintptr, text, caption string, flags uint32) {
	pMessageBoxW.Call(owner, uintptr(unsafe.Pointer(u16(text))), uintptr(unsafe.Pointer(u16(caption))), uintptr(flags))
}

func clipboardText() string {
	r, _, _ := pOpenClipboard.Call(0)
	if r == 0 {
		return ""
	}
	defer pCloseClipboard.Call()
	ok, _, _ := pIsClipboardFormatAvail.Call(CF_UNICODETEXT)
	if ok == 0 {
		return ""
	}
	h, _, _ := pGetClipboardData.Call(CF_UNICODETEXT)
	if h == 0 {
		return ""
	}
	ptr, _, _ := pGlobalLock.Call(h)
	if ptr == 0 {
		return ""
	}
	defer pGlobalUnlock.Call(h)
	// Find the terminating NUL.
	var n uintptr
	for {
		c := *(*uint16)(unsafe.Pointer(ptr + n*2))
		if c == 0 {
			break
		}
		n++
		if n > 1<<20 {
			return ""
		}
	}
	buf := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), n)
	return syscall.UTF16ToString(buf)
}

func sendCopyData(dst uintptr, payload string) bool {
	b := append([]byte(payload), 0)
	cds := COPYDATASTRUCT{DwData: 1, CbData: uint32(len(b)), LpData: uintptr(unsafe.Pointer(&b[0]))}
	r, _, _ := pSendMessageW.Call(dst, WM_COPYDATA, 0, uintptr(unsafe.Pointer(&cds)))
	return r != 0
}
