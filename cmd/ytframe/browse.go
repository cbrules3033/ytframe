//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
)

const (
	browseClass    = "YTFrameBrowseHost"
	browseToolbarH = 52
	browseHome     = "https://www.youtube.com/"

	browseRefreshTimer = 1

	// WebView2 can leave a white surface after its host was hidden or if it
	// was created just before the first paint. A delayed visibility/resize
	// nudge forces Chromium to present the page again.
	browseRefreshDelay = 120
)

const (
	bwBtnBack = iota
	bwBtnFwd
	bwBtnReload
	bwBtnHome
	bwBtnMin
	bwBtnMax
	bwBtnClose
	bwBtnGet
	bwBtnCloseErr
	bwBtnCount
)

type browseWindow struct {
	hwnd      uintptr
	container uintptr
	w         webview2.WebView
	failed    bool
	hidden    bool
	closing   bool

	scale   float64
	title   string
	hover   int
	press   int
	maxed   bool
	restore RECT

	btn [bwBtnCount]RECT

	fontBody  uintptr
	fontTitle uintptr
	brBg      uintptr
	brPanel   uintptr
	brHover   uintptr
	brAccent  uintptr
	brText    uintptr
	penBorder uintptr
	penIcon   uintptr
	penMuted  uintptr
}

var (
	bw           *browseWindow
	browseProcCB uintptr
	browseMutex  uintptr
)

func init() { browseProcCB = syscall.NewCallback(browseWndProc) }

func hasArg(name string) bool {
	for _, a := range os.Args[1:] {
		if a == name {
			return true
		}
	}
	return false
}

func browseRectFromArgs() (int32, int32, int32, int32, bool) {
	for _, a := range os.Args[1:] {
		if !strings.HasPrefix(a, "--browse-rect=") {
			continue
		}
		var x, y, w, h int32
		if _, err := fmt.Sscanf(strings.TrimPrefix(a, "--browse-rect="), "%d,%d,%d,%d", &x, &y, &w, &h); err == nil && w >= 400 && h >= 300 {
			return x, y, w, h, true
		}
	}
	return 0, 0, 0, 0, false
}

func runBrowse() {
	name := u16(`Local\YTFrameBrowse`)
	h, _, lastErr := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	browseMutex = h
	if errno, ok := lastErr.(syscall.Errno); ok && errno == ERROR_ALREADY_EXISTS {
		if target, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16(browseClass))), 0); target != 0 {
			pPostMessageW.Call(target, WM_APP_BROWSESHOW, 0, 0)
		}
		return
	}

	bw = &browseWindow{
		scale: float64(systemDPI()) / 96.0,
		title: "YouTube",
		hover: -1,
		press: -1,
	}
	bw.ensureGDI()
	bw.createHost()
	if bw.hwnd == 0 {
		return
	}
	logf("browse: host created")
	bw.createContainer()
	bw.layout()

	if x, y, ww, hh, ok := browseRectFromArgs(); ok {
		r := RECT{Left: x, Top: y, Right: x + ww, Bottom: y + hh}
		work := workAreaAt(x+ww/2, y+hh/2)
		r = fitWindowRect(r, work)
		pSetWindowPos.Call(bw.hwnd, 0, uintptr(r.Left), uintptr(r.Top),
			uintptr(rectW(r)), uintptr(rectH(r)), SWP_NOZORDER|SWP_NOACTIVATE)
	}
	pShowWindow.Call(bw.hwnd, SW_SHOW)
	pUpdateWindow.Call(bw.hwnd)
	if bw.container != 0 {
		// WebView2 created while its parent is hidden can present a blank
		// white surface until something forces a repaint. Make the container
		// visible before creating the controller.
		pShowWindow.Call(bw.container, SW_SHOW)
	}
	logf("browse: host shown")

	version := ""
	if hasArg("--browse-fail") || os.Getenv("YTFRAME_BROWSE_FAKE_FAIL") == "1" {
		logf("browse: forced failure page")
	} else {
		logf("browse: checking WebView2 runtime")
		version = runWebviewVersionCheck()
		logf("browse: runtime result %q", version)
	}
	if version == "" {
		logf("webview: runtime not installed")
		bw.failed = true
		pShowWindow.Call(bw.container, SW_HIDE)
		bw.layout()
		bw.invalidate()
	} else {
		logf("webview: runtime %s", version)
		dataPath := filepath.Join(dataDirPath, "browse-data")
		os.MkdirAll(dataPath, 0o755)
		bw.w = webview2.NewWithOptions(webview2.WebViewOptions{
			Window:    unsafe.Pointer(bw.container),
			DataPath:  dataPath,
			AutoFocus: true,
			Debug:     false,
		})
		if bw.w == nil {
			logf("webview: create failed")
			bw.failed = true
			pShowWindow.Call(bw.container, SW_HIDE)
			bw.layout()
			bw.invalidate()
		}
	}
	if bw.w != nil {
		pShowWindow.Call(bw.container, SW_SHOW)
		bw.w.SetVisible(true)
		bw.w.Resize()
		bw.w.Bind("ytframePlay", func(u string) {
			bw.w.Dispatch(func() { browsePlay(u) })
		})
		bw.w.Bind("ytframeTitle", func(t string) {
			bw.w.Dispatch(func() { browseSetTitle(t) })
		})
		bw.w.Init(browseInitJS)
		bw.w.Navigate(browseHome)
		pSetTimer.Call(bw.hwnd, browseRefreshTimer, 700, 0)
		bw.w.Run()
	} else {
		bw.runLoop()
	}
}

func (bw *browseWindow) showWebView() {
	if bw.w == nil {
		return
	}
	pShowWindow.Call(bw.container, SW_SHOW)
	bw.layout()
	bw.w.SetVisible(true)
	bw.w.Resize()
	pSetTimer.Call(bw.hwnd, browseRefreshTimer, uintptr(browseRefreshDelay), 0)
}

func (bw *browseWindow) refreshWebView() {
	if bw.w == nil || bw.hwnd == 0 || bw.hidden {
		return
	}
	// A plain SetVisible(true) is not enough for some WebView2 builds after
	// the host window was hidden; force a visibility transition and resize.
	bw.w.SetVisible(false)
	bw.w.SetVisible(true)
	bw.w.Resize()
	bw.invalidate()
	logf("browse: webview repaint")
}

// hideBrowse keeps the WebView2 process and page alive. Reopening Browse only
// shows this window again; it does not create a new browser or navigate home.
func (bw *browseWindow) hideBrowse() {
	if bw == nil || bw.hwnd == 0 {
		return
	}
	bw.saveRect()
	if bw.w != nil {
		bw.w.Eval("document.querySelectorAll('video,audio').forEach(function(v){v.pause()})")
		bw.w.SetVisible(false)
	}
	bw.hidden = true
	pShowWindow.Call(bw.hwnd, SW_HIDE)
	if target, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16("YTFrameHelper"))), 0); target != 0 {
		sendCopyDataKind(target, 3, "hidden")
	}
	logf("browse: hidden, session retained")
}

// closeBrowse destroys the retained browser process. This is only used for an
// explicit Close command, clearing data, or quitting YTFrame. The title-bar X,
// Alt+F4 and the taskbar close button all hide instead, so the page survives.
func (bw *browseWindow) closeBrowse() {
	if bw == nil || bw.hwnd == 0 {
		return
	}
	bw.closing = true
	pPostMessageW.Call(bw.hwnd, WM_CLOSE, 0, 0)
}

func (bw *browseWindow) runLoop() {
	var msg MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func (bw *browseWindow) ensureGDI() {
	if bw.fontBody != 0 {
		return
	}
	s := bw.scale
	mk := func(px float64, weight int32) uintptr {
		h, _, _ := pCreateFontW.Call(
			uintptr(int32(-px*s)), 0, 0, 0, uintptr(weight),
			0, 0, 0, DEFAULT_CHARSET, 0, 0, CLEARTYPE_QUALITY,
			uintptr(DEFAULT_PITCH|FF_DONTCARE),
			uintptr(unsafe.Pointer(u16("Segoe UI"))),
		)
		return h
	}
	bw.fontBody = mk(13, FW_NORMAL)
	bw.fontTitle = mk(22, FW_SEMIBOLD)
	bw.brBg, _, _ = pCreateSolidBrush.Call(colBg)
	bw.brPanel, _, _ = pCreateSolidBrush.Call(colField)
	bw.brHover, _, _ = pCreateSolidBrush.Call(rgb(37, 42, 49))
	bw.brAccent, _, _ = pCreateSolidBrush.Call(colAccent)
	bw.brText, _, _ = pCreateSolidBrush.Call(colText)
	bw.penBorder, _, _ = pCreatePen.Call(PS_SOLID, 1, colBorder)
	bw.penIcon, _, _ = pCreatePen.Call(PS_SOLID, uintptr(maxInt(1, int(1.6*s))), colText)
	bw.penMuted, _, _ = pCreatePen.Call(PS_SOLID, uintptr(maxInt(1, int(1.4*s))), colMuted)
}

func (bw *browseWindow) createHost() {
	hInst := getModuleHandle()
	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.Style = 0x0008 // CS_DBLCLKS
	wc.LpfnWndProc = browseProcCB
	wc.HInstance = hInst
	wc.HIcon = loadAppIcon()
	wc.HIconSm = wc.HIcon
	wc.HCursor = loadCursor(IDC_ARROW)
	wc.LpszClassName = u16(browseClass)
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	width := int32(1200 * bw.scale)
	height := int32(800 * bw.scale)
	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	work := workAreaForPoint(pt)
	if width > rectW(work) {
		width = rectW(work)
	}
	if height > rectH(work) {
		height = rectH(work)
	}
	x := work.Left + (rectW(work)-width)/2
	y := work.Top + (rectH(work)-height)/2

	hwnd, _, _ := pCreateWindowExW.Call(
		WS_EX_APPWINDOW,
		uintptr(unsafe.Pointer(u16(browseClass))),
		uintptr(unsafe.Pointer(u16("YouTube — YTFrame"))),
		WS_POPUP|WS_THICKFRAME|WS_MINIMIZEBOX|WS_MAXIMIZEBOX|WS_SYSMENU|WS_CLIPCHILDREN,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		logf("browse: host creation failed")
		return
	}
	bw.hwnd = hwnd
}

func (bw *browseWindow) createContainer() {
	var rc RECT
	pGetClientRect.Call(bw.hwnd, uintptr(unsafe.Pointer(&rc)))
	th := int32(browseToolbarH * bw.scale)
	hwnd, _, _ := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(u16("STATIC"))),
		0,
		WS_CHILD|WS_VISIBLE|WS_CLIPCHILDREN,
		0, uintptr(th), uintptr(rc.Right), uintptr(rc.Bottom-th),
		bw.hwnd, 0, getModuleHandle(), 0,
	)
	bw.container = hwnd
}

func (bw *browseWindow) layout() {
	if bw.hwnd == 0 {
		return
	}
	var rc RECT
	pGetClientRect.Call(bw.hwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.Right, rc.Bottom
	s := bw.scale
	th := int32(browseToolbarH * s)
	if th > h-80 {
		th = 40
	}
	btn := int32(28 * s)
	by := (th - btn) / 2
	R := func(x, y, ww, hh float64) RECT {
		return RECT{Left: int32(x * s), Top: int32(y * s), Right: int32((x + ww) * s), Bottom: int32((y + hh) * s)}
	}
	bw.btn[bwBtnBack] = R(10, float64(by)/s, 28, 28)
	bw.btn[bwBtnFwd] = R(42, float64(by)/s, 28, 28)
	bw.btn[bwBtnReload] = R(74, float64(by)/s, 28, 28)
	bw.btn[bwBtnHome] = R(106, float64(by)/s, 28, 28)

	right := float64(w)/s - 10
	for _, id := range []int{bwBtnClose, bwBtnMax, bwBtnMin} {
		right -= 28
		bw.btn[id] = R(right, float64(by)/s, 28, 28)
		right -= 4
	}

	if bw.container != 0 {
		pMoveWindow.Call(bw.container, 0, uintptr(th), uintptr(w), uintptr(h-th), 1)
	}
	if bw.w != nil {
		bw.w.Resize()
	}
	if bw.failed {
		c := bw.errorContent()
		bw.btn[bwBtnGet] = RECT{Left: c.Left, Top: c.Top + int32(142*s), Right: c.Left + int32(160*s), Bottom: c.Top + int32(184*s)}
		bw.btn[bwBtnCloseErr] = RECT{Left: c.Left + int32(172*s), Top: c.Top + int32(142*s), Right: c.Left + int32(272*s), Bottom: c.Top + int32(184*s)}
	}
}

func (bw *browseWindow) invalidate() {
	if bw.hwnd != 0 {
		pInvalidateRect.Call(bw.hwnd, 0, 0)
	}
}

func (bw *browseWindow) paint(hdc uintptr) {
	var rc RECT
	pGetClientRect.Call(bw.hwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.Right, rc.Bottom
	s := bw.scale
	th := int32(browseToolbarH * s)

	toolbar := RECT{Left: 0, Top: 0, Right: w, Bottom: th}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&toolbar)), bw.brBg)
	pSelectObject.Call(hdc, bw.penBorder)
	pMoveToEx.Call(hdc, 0, uintptr(th-1), 0)
	pLineTo.Call(hdc, uintptr(w), uintptr(th-1))

	bw.drawButton(hdc, bwBtnBack, false, bw.failed)
	bw.drawButton(hdc, bwBtnFwd, false, bw.failed)
	bw.drawButton(hdc, bwBtnReload, false, bw.failed)
	bw.drawButton(hdc, bwBtnHome, false, bw.failed)
	bw.drawButton(hdc, bwBtnMin, false, false)
	bw.drawButton(hdc, bwBtnMax, false, false)
	bw.drawButton(hdc, bwBtnClose, false, false)

	pSelectObject.Call(hdc, bw.fontBody)
	pSetTextColor.Call(hdc, colText)
	pSetBkMode.Call(hdc, TRANSPARENT)
	titleRect := RECT{Left: int32(144 * s), Top: 0, Right: bw.btn[bwBtnMin].Left - int32(10*s), Bottom: th}
	drawText(hdc, bw.title, titleRect, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)

	if !bw.failed && bw.w == nil {
		fillRect(hdc, RECT{Left: 0, Top: th, Right: w, Bottom: h}, bw.brBg)
		c := bw.errorContent()
		pSelectObject.Call(hdc, bw.fontTitle)
		pSetTextColor.Call(hdc, colText)
		drawText(hdc, "Opening YouTube…", RECT{Left: c.Left, Top: c.Top, Right: c.Right, Bottom: c.Top + int32(40*s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
		pSelectObject.Call(hdc, bw.fontBody)
		pSetTextColor.Call(hdc, colMuted)
		drawText(hdc, "Your browsing session will be ready in a moment.", RECT{Left: c.Left, Top: c.Top + int32(52*s), Right: c.Right, Bottom: c.Top + int32(80*s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
	}

	if bw.failed {
		content := RECT{Left: 0, Top: th, Right: w, Bottom: h}
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&content)), bw.brBg)
		c := bw.errorContent()
		pad := int32(28 * s)
		pSelectObject.Call(hdc, bw.penBorder)
		pSelectObject.Call(hdc, bw.brPanel)
		pRoundRect.Call(hdc, uintptr(c.Left-pad), uintptr(c.Top-pad), uintptr(c.Right+pad), uintptr(c.Bottom+pad), uintptr(int32(24*s)), uintptr(int32(24*s)))
		pSelectObject.Call(hdc, bw.fontBody)
		pSetTextColor.Call(hdc, colAccent)
		drawText(hdc, "BROWSER SETUP", RECT{Left: c.Left, Top: c.Top, Right: c.Right, Bottom: c.Top + int32(20*s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
		pSelectObject.Call(hdc, bw.fontTitle)
		pSetTextColor.Call(hdc, colText)
		drawText(hdc, "One quick setup for browsing", RECT{Left: c.Left, Top: c.Top + int32(28*s), Right: c.Right, Bottom: c.Top + int32(62*s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
		pSelectObject.Call(hdc, bw.fontBody)
		pSetTextColor.Call(hdc, colMuted)
		drawText(hdc, "YouTube browsing needs Microsoft Edge WebView2. Install the free Runtime once, then reopen Browse. You can keep playing links in YTFrame.", RECT{Left: c.Left, Top: c.Top + int32(76*s), Right: c.Right, Bottom: c.Top + int32(128*s)}, DT_LEFT|DT_WORDBREAK|DT_NOPREFIX)
		bw.drawTextButton(hdc, bwBtnGet, "Get WebView2", true)
		bw.drawTextButton(hdc, bwBtnCloseErr, "Close", false)
	}
}

func (bw *browseWindow) drawButton(hdc uintptr, id int, active bool, disabled bool) {
	r := bw.btn[id]
	if r.Right <= r.Left {
		return
	}
	if !disabled && (bw.hover == id || bw.press == id) {
		br := bw.brHover
		if bw.press == id {
			br = bw.brPanel
		}
		pSelectObject.Call(hdc, pGetStockObject2(NULL_PEN))
		pSelectObject.Call(hdc, br)
		pRoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom),
			uintptr(int32(6*bw.scale)), uintptr(int32(6*bw.scale)))
	}
	pen := bw.penIcon
	if disabled {
		pen = bw.penMuted
	}
	pSelectObject.Call(hdc, pen)
	pSelectObject.Call(hdc, pGetStockObject2(NULL_BRUSH))
	cx := (r.Left + r.Right) / 2
	cy := (r.Top + r.Bottom) / 2
	s := bw.scale
	switch id {
	case bwBtnBack:
		pMoveToEx.Call(hdc, uintptr(cx+int32(3*s)), uintptr(cy-int32(5*s)), 0)
		pLineTo.Call(hdc, uintptr(cx-int32(3*s)), uintptr(cy))
		pLineTo.Call(hdc, uintptr(cx+int32(3*s)), uintptr(cy+int32(5*s)))
	case bwBtnFwd:
		pMoveToEx.Call(hdc, uintptr(cx-int32(3*s)), uintptr(cy-int32(5*s)), 0)
		pLineTo.Call(hdc, uintptr(cx+int32(3*s)), uintptr(cy))
		pLineTo.Call(hdc, uintptr(cx-int32(3*s)), uintptr(cy+int32(5*s)))
	case bwBtnReload:
		pArc.Call(hdc, uintptr(cx-int32(7*s)), uintptr(cy-int32(7*s)),
			uintptr(cx+int32(7*s)), uintptr(cy+int32(7*s)),
			uintptr(cx+int32(7*s)), uintptr(cy-int32(2*s)),
			uintptr(cx+int32(1*s)), uintptr(cy+int32(7*s)))
		pts := []POINT{{cx + int32(4*s), cy - int32(7*s)}, {cx + int32(9*s), cy - int32(6*s)}, {cx + int32(8*s), cy - int32(1*s)}}
		pSelectObject.Call(hdc, pGetStockObject2(NULL_PEN))
		pSelectObject.Call(hdc, bw.brAccent)
		pPolygon.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	case bwBtnHome:
		pts := []POINT{
			{cx, cy - int32(8*s)}, {cx + int32(8*s), cy}, {cx + int32(5*s), cy},
			{cx + int32(5*s), cy + int32(7*s)}, {cx - int32(5*s), cy + int32(7*s)},
			{cx - int32(5*s), cy}, {cx - int32(8*s), cy},
		}
		pSelectObject.Call(hdc, pGetStockObject2(NULL_PEN))
		pSelectObject.Call(hdc, bw.brText)
		pPolygon.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	case bwBtnMin:
		pMoveToEx.Call(hdc, uintptr(cx-int32(6*s)), uintptr(cy+int32(5*s)), 0)
		pLineTo.Call(hdc, uintptr(cx+int32(6*s)), uintptr(cy+int32(5*s)))
	case bwBtnMax:
		pRectangle.Call(hdc, uintptr(cx-int32(5*s)), uintptr(cy-int32(5*s)),
			uintptr(cx+int32(5*s)), uintptr(cy+int32(5*s)))
	case bwBtnClose:
		d := int32(5 * s)
		pMoveToEx.Call(hdc, uintptr(cx-d), uintptr(cy-d), 0)
		pLineTo.Call(hdc, uintptr(cx+d), uintptr(cy+d))
		pMoveToEx.Call(hdc, uintptr(cx+d), uintptr(cy-d), 0)
		pLineTo.Call(hdc, uintptr(cx-d), uintptr(cy+d))
	}
}

func (bw *browseWindow) drawTextButton(hdc uintptr, id int, label string, primary bool) {
	r := bw.btn[id]
	if r.Right <= r.Left {
		return
	}
	br := bw.brPanel
	if primary {
		br = bw.brAccent
	}
	if bw.hover == id {
		if primary {
			br = bw.brAccent
		} else {
			br = bw.brHover
		}
	}
	pSelectObject.Call(hdc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(hdc, br)
	pRoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom),
		uintptr(int32(6*bw.scale)), uintptr(int32(6*bw.scale)))
	pSelectObject.Call(hdc, bw.fontBody)
	if primary {
		pSetTextColor.Call(hdc, colDark)
	} else {
		pSetTextColor.Call(hdc, colText)
	}
	pSetBkMode.Call(hdc, TRANSPARENT)
	drawText(hdc, label, r, DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
}

func (bw *browseWindow) onClick(id int) {
	switch id {
	case bwBtnBack:
		if bw.w != nil {
			bw.w.Eval("history.back()")
		}
	case bwBtnFwd:
		if bw.w != nil {
			bw.w.Eval("history.forward()")
		}
	case bwBtnReload:
		if bw.w != nil {
			bw.w.Eval("location.reload()")
		}
	case bwBtnHome:
		if bw.w != nil {
			bw.w.Navigate(browseHome)
		}
	case bwBtnMin:
		pShowWindow.Call(bw.hwnd, SW_MINIMIZE)
	case bwBtnMax:
		bw.toggleMax()
	case bwBtnClose:
		pPostMessageW.Call(bw.hwnd, WM_APP_BROWSEHIDE, 0, 0)
	case bwBtnGet:
		pShellExecuteW.Call(0,
			uintptr(unsafe.Pointer(u16("open"))),
			uintptr(unsafe.Pointer(u16("https://developer.microsoft.com/en-us/microsoft-edge/webview2/"))),
			0, 0, 1)
	case bwBtnCloseErr:
		pPostMessageW.Call(bw.hwnd, WM_CLOSE, 0, 0)
	}
}

func (bw *browseWindow) toggleMax() {
	if bw.hwnd == 0 {
		return
	}
	if bw.maxed {
		pSetWindowPos.Call(bw.hwnd, 0, uintptr(bw.restore.Left), uintptr(bw.restore.Top),
			uintptr(rectW(bw.restore)), uintptr(rectH(bw.restore)), SWP_NOZORDER|SWP_NOACTIVATE)
		bw.maxed = false
		return
	}
	var rc RECT
	if ok, _, _ := pGetWindowRect.Call(bw.hwnd, uintptr(unsafe.Pointer(&rc))); ok == 0 {
		return
	}
	bw.restore = rc
	work := monitorWork(bw.hwnd)
	pSetWindowPos.Call(bw.hwnd, 0, uintptr(work.Left), uintptr(work.Top),
		uintptr(rectW(work)), uintptr(rectH(work)), SWP_NOZORDER|SWP_NOACTIVATE)
	bw.maxed = true
}

func (bw *browseWindow) saveRect() {
	if bw.hwnd == 0 {
		return
	}
	var rc RECT
	if bw.maxed {
		rc = bw.restore
	} else if ok, _, _ := pGetWindowRect.Call(bw.hwnd, uintptr(unsafe.Pointer(&rc))); ok == 0 {
		return
	}
	if rectW(rc) < 300 || rectH(rc) < 200 {
		return
	}
	if target, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16("YTFrameHelper"))), 0); target != 0 {
		payload := fmt.Sprintf("%d,%d,%d,%d", rc.Left, rc.Top, rectW(rc), rectH(rc))
		sendCopyDataKind(target, 2, payload)
	}
}

func browseWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logf("browse panic: %v", r)
		}
	}()
	if bw == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, msg, wparam, lparam)
		return r
	}
	switch msg {
	case WM_APP_BROWSESHOW:
		bw.hidden = false
		pShowWindow.Call(hwnd, SW_RESTORE)
		bw.showWebView()
		bw.invalidate()
		pSetForegroundWindow.Call(hwnd)
		logf("browse: session shown")
		return 0
	case WM_APP_BROWSEHIDE:
		if bw.failed {
			bw.closeBrowse()
			return 0
		}
		bw.hideBrowse()
		return 0
	case WM_APP_BROWSECLOSE:
		bw.closeBrowse()
		return 0
	case WM_CREATE:
		bw.hwnd = hwnd
		return 0
	case WM_SHOWWINDOW:
		if wparam != 0 && bw.w != nil {
			bw.showWebView()
		}
		return 0
	case WM_TIMER:
		if wparam == browseRefreshTimer {
			pKillTimer.Call(hwnd, browseRefreshTimer)
			bw.refreshWebView()
			return 0
		}
	case WM_DPICHANGED:
		bw.scale = float64(loword(wparam)) / 96
		bw.freeGDI()
		bw.ensureGDI()
		r := (*RECT)(unsafe.Pointer(lparam))
		pSetWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(rectW(*r)), uintptr(rectH(*r)), SWP_NOZORDER|SWP_NOACTIVATE)
		bw.layout()
		bw.invalidate()
		return 0
	case WM_SIZE:
		bw.layout()
		bw.invalidate()
		return 0
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if hdc != 0 {
			bw.paint(hdc)
			pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		}
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_MOUSEMOVE:
		x, y := mouseX(lparam), mouseY(lparam)
		h := -1
		for i, r := range bw.btn {
			if i < bwBtnGet && pointIn(r, x, y) {
				h = i
				break
			}
		}
		if bw.failed {
			if pointIn(bw.btn[bwBtnGet], x, y) {
				h = bwBtnGet
			} else if pointIn(bw.btn[bwBtnCloseErr], x, y) {
				h = bwBtnCloseErr
			}
		}
		if h != bw.hover {
			bw.hover = h
			bw.invalidate()
		}
		var tme TRACKMOUSEEVENT
		tme.CbSize = uint32(unsafe.Sizeof(tme))
		tme.DwFlags = TME_LEAVE
		tme.HwndTrack = hwnd
		pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
		return 0
	case WM_MOUSELEAVE:
		if bw.hover != -1 {
			bw.hover = -1
			bw.invalidate()
		}
		return 0
	case WM_SETCURSOR:
		var pt POINT
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		var rc RECT
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		x, y := pt.X-rc.Left, pt.Y-rc.Top
		for i, r := range bw.btn {
			if i < bwBtnGet && pointIn(r, x, y) {
				h, _, _ := pLoadCursorW.Call(0, IDC_HAND)
				pSetCursor.Call(h)
				return 1
			}
		}
		if bw.failed && (pointIn(bw.btn[bwBtnGet], x, y) || pointIn(bw.btn[bwBtnCloseErr], x, y)) {
			h, _, _ := pLoadCursorW.Call(0, IDC_HAND)
			pSetCursor.Call(h)
			return 1
		}
	case WM_LBUTTONDOWN:
		x, y := mouseX(lparam), mouseY(lparam)
		for i, r := range bw.btn {
			if i < bwBtnGet && pointIn(r, x, y) {
				bw.press = i
				pSetCapture.Call(hwnd)
				bw.invalidate()
				return 0
			}
		}
		if bw.failed {
			if pointIn(bw.btn[bwBtnGet], x, y) {
				bw.press = bwBtnGet
				pSetCapture.Call(hwnd)
				return 0
			}
			if pointIn(bw.btn[bwBtnCloseErr], x, y) {
				bw.press = bwBtnCloseErr
				pSetCapture.Call(hwnd)
				return 0
			}
		}
		// Drag by the toolbar background.
		if y < int32(browseToolbarH*bw.scale) {
			pReleaseCapture.Call()
			pSendMessageW.Call(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0)
			return 0
		}
		return 0
	case WM_LBUTTONUP:
		x, y := mouseX(lparam), mouseY(lparam)
		if bw.press >= 0 {
			p := bw.press
			bw.press = -1
			pReleaseCapture.Call()
			bw.invalidate()
			r := bw.btn[p]
			if pointIn(r, x, y) {
				bw.onClick(p)
			}
		}
		return 0
	case WM_LBUTTONDBLCLK:
		y := hiword(lparam)
		if y < int32(browseToolbarH*bw.scale) {
			bw.toggleMax()
		}
		return 0
	case WM_CAPTURECHANGED, WM_CANCELMODE:
		bw.press = -1
		bw.invalidate()
		return 0
	case 0x0083: // WM_NCCALCSIZE: keep the custom browser chrome borderless.
		return 0
	case 0x0084: // WM_NCHITTEST: retain edge/corner resizing.
		var rc RECT
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		x, y := mouseX(lparam)-rc.Left, mouseY(lparam)-rc.Top
		if !bw.maxed {
			d := int32(6 * bw.scale)
			left, right := x < d, x >= rectW(rc)-d
			top, bottom := y < d, y >= rectH(rc)-d
			switch {
			case top && left:
				return 13
			case top && right:
				return 14
			case bottom && left:
				return 16
			case bottom && right:
				return 17
			case left:
				return 10
			case right:
				return 11
			case top:
				return 12
			case bottom:
				return 15
			}
		}
		return 1 // HTCLIENT
	case WM_GETMINMAXINFO:
		mm := (*MINMAXINFO)(unsafe.Pointer(lparam))
		mm.MinTrackSize = POINT{X: int32(640 * bw.scale), Y: int32(480 * bw.scale)}
		return 0
	case WM_CLOSE:
		if !bw.closing && !bw.failed {
			bw.hideBrowse()
			return 0
		}
		bw.saveRect()
		pDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		bw.freeGDI()
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

func webviewVersion() string {
	v, err := webviewloader.GetInstalledVersion()
	if err != nil {
		return ""
	}
	return v
}

func runWebviewVersionCheck() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "--webview-version")
	cmd.Dir = dataDirPath
	out, err := cmd.Output()
	if ctx.Err() != nil {
		logf("webview: version check timed out")
		return ""
	}
	if err != nil {
		logf("webview: version child: %v", err)
		return ""
	}
	return strings.TrimSpace(string(out))
}

func browsePlay(u string) {
	u = strings.TrimSpace(u)
	if youtubeID(u) == "" {
		return
	}
	logf("browse: video selected %s", u)
	target, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16("YTFrameHelper"))), 0)
	if target != 0 {
		var pid uint32
		pGetWindowThreadProcID.Call(target, uintptr(unsafe.Pointer(&pid)))
		pAllowSetForegroundWindow.Call(uintptr(pid))
		sendCopyData(target, u)
	}
	if bw != nil && bw.hwnd != 0 {
		pPostMessageW.Call(bw.hwnd, WM_APP_BROWSEHIDE, 0, 0)
	}
}

func browseSetTitle(t string) {
	t = strings.TrimSpace(t)
	if t == "" {
		t = "YouTube"
	}
	if bw == nil {
		return
	}
	if len([]rune(t)) > 120 {
		r := []rune(t)
		t = string(r[:120]) + "…"
	}
	bw.title = t
	logf("browse title: %s", t)
	if bw.hwnd != 0 {
		setWindowText(bw.hwnd, t+" — YTFrame")
		bw.invalidate()
	}
}

const browseInitJS = `
(function () {
  if (window.__ytframeInit) return;
  window.__ytframeInit = true;

  var lastKey="", lastTitle="";
  function key(u) {
    try {
      var x = new URL(u, location.href);
      var v = x.searchParams.get('v');
      if (/^\/watch$/.test(x.pathname) && v) return 'v:' + v;
      var m = x.pathname.match(/^\/shorts\/([^\/?#]+)/);
      if (m) return 's:' + m[1];
      return u;
    } catch (e) { return u; }
  }
  function send(u) {
    var k = key(u);
    if (k === lastKey) return;
    lastKey = k;
    try { window.ytframePlay(u); } catch (e) {}
  }
  function title() {
    try { var t=document.title || 'YouTube';if(t===lastTitle)return;lastTitle=t;window.ytframeTitle(t); } catch (e) {}
  }

  document.addEventListener('click', function (e) {
    var a = e.target && e.target.closest ? e.target.closest('a[href*="/watch"], a[href*="/shorts/"]') : null;
    if (!a) return;
    try {
      var u = new URL(a.href, location.href);
      if (!/^(www\.|m\.|music\.)?youtube\.com$/.test(u.hostname)) return;
      if ((u.pathname === '/watch' && u.searchParams.get('v')) ||
          u.pathname.indexOf('/shorts/') === 0) {
        e.preventDefault();
        e.stopPropagation();
        send(u.href);
      }
    } catch (err) {}
  }, true);

  window.addEventListener('yt-navigate-finish', function () {
    try {
      if ((location.pathname === '/watch' && new URLSearchParams(location.search).get('v')) || location.pathname.indexOf('/shorts/') === 0) {
        send(location.href);
      } else {
        // Leaving a video resets the guard, so choosing the same video again
        // later still opens it.
        lastKey = "";
      }
    } catch (e) {}
    title();
  });

  document.addEventListener('keydown', function (e) {
    if (!e.altKey) return;
    if (e.key === 'ArrowLeft') { e.preventDefault(); history.back(); }
    if (e.key === 'ArrowRight') { e.preventDefault(); history.forward(); }
    if (e.key === 'Home') { e.preventDefault(); location.href = 'https://www.youtube.com/'; }
  }, true);

  window.addEventListener('load', title);
  setTimeout(title, 800);
  setInterval(title, 2000);
})();
`

func (a *App) openBrowse() {
	// Have mpv ready while the user picks a video, so a click only has to wait
	// for YouTube's resolver, not for a cold player process.
	a.warmPlayer()
	if target, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16(browseClass))), 0); target != 0 {
		logf("browse: reusing retained session")
		pPostMessageW.Call(target, WM_APP_BROWSESHOW, 0, 0)
		a.browseOpen = true
		return
	}
	if a.browseCmd != nil {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		messageBox(0, "Could not locate YTFrame.exe.", "Browse YouTube", MB_OK|MB_ICONERROR)
		return
	}
	args := []string{"--browse"}
	if a.settings.BrowseHasRect {
		args = append(args, fmt.Sprintf("--browse-rect=%d,%d,%d,%d",
			a.settings.BrowseX, a.settings.BrowseY, a.settings.BrowseWidth, a.settings.BrowseHeight))
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = dataDirPath
	if err := cmd.Start(); err != nil {
		logf("browse start failed: %v", err)
		messageBox(0, "Could not open the YouTube browser.\n\n"+err.Error(), "Browse YouTube", MB_OK|MB_ICONERROR)
		return
	}
	a.browseOpen = true
	a.browseCmd = cmd
	a.browsePid = cmd.Process.Pid
	go func() {
		cmd.Wait()
		postMsg(WM_APP_BROWSEEXIT, 0, 0)
	}()
}

func (a *App) onBrowseExit() {
	a.browseOpen = false
	a.browseCmd = nil
	a.browsePid = 0
	if !a.quitting && a.settings.BarVisible {
		a.showControlBar()
	}
}

func browseWindowExists() bool {
	hwnd, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16(browseClass))), 0)
	return hwnd != 0
}

// closeBrowse releases the retained WebView2 session on an explicit quit/close.
func (a *App) closeBrowse() {
	if hwnd, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16(browseClass))), 0); hwnd != 0 {
		pPostMessageW.Call(hwnd, WM_APP_BROWSECLOSE, 0, 0)
	}
}

func (a *App) clearBrowseData() {
	if messageBox(0, "Delete the local YouTube browsing profile?\n\nThis removes saved logins, cookies and the page cache. It cannot be undone.",
		"Clear YouTube browsing data", MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	a.closeBrowse()
	for i := 0; i < 30; i++ {
		if hwnd, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16(browseClass))), 0); hwnd == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	dir := filepath.Join(dataDirPath, "browse-data")
	var err error
	for i := 0; i < 5; i++ {
		err = os.RemoveAll(dir)
		if err == nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if err != nil {
		messageBox(0, "Could not remove the browsing profile:\n"+err.Error(), "Clear YouTube browsing data", MB_OK|MB_ICONERROR)
		return
	}
	a.settings.BrowseHasRect = false
	saveSettings(a.settings)
	messageBox(0, "YouTube browsing data cleared.", "YTFrame", MB_OK|MB_ICONINFORMATION)
}

func (bw *browseWindow) freeGDI() {
	for _, h := range []uintptr{bw.fontBody, bw.fontTitle, bw.brBg, bw.brPanel, bw.brHover, bw.brAccent, bw.brText, bw.penBorder, bw.penIcon, bw.penMuted} {
		if h != 0 {
			pDeleteObject.Call(h)
		}
	}
	bw.fontBody = 0
}

func (bw *browseWindow) errorContent() RECT {
	var rc RECT
	pGetClientRect.Call(bw.hwnd, uintptr(unsafe.Pointer(&rc)))
	s := bw.scale
	w := int32(536 * s)
	if max := rectW(rc) - int32(88*s); w > max {
		w = max
	}
	h := int32(190 * s)
	x := (rectW(rc) - w) / 2
	th := int32(browseToolbarH * s)
	y := th + (rectH(rc)-th-h)/2
	return RECT{Left: x, Top: y, Right: x + w, Bottom: y + h}
}
