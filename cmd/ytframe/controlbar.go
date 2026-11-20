//go:build windows

package main

import (
	"fmt"
	"math"
	"syscall"
	"time"
	"unsafe"
)

const (
	cbW = 600
	cbH = 152
)

const (
	cbBtnPlay = iota
	cbBtnBack
	cbBtnFwd
	cbBtnVol
	cbBtnFullscreen
	cbBtnEye
	cbBtnLayouts
	cbBtnPin
	cbBtnBrowse
	cbBtnHide
	cbBtnMenu
	cbBtnOpen
	cbBtnCount
)

var cbProcCB uintptr

func init() { cbProcCB = syscall.NewCallback(cbWndProc) }

func (a *App) createControlBar() {
	if a.cbScale == 0 {
		a.cbScale = a.scale
	}
	a.cbEnsureGDI()
	hInst := getModuleHandle()
	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = cbProcCB
	wc.HInstance = hInst
	wc.HIcon = a.icon
	wc.HCursor = loadCursor(IDC_ARROW)
	wc.LpszClassName = u16("YTFrameControls")
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	w := int32(cbW * a.cbScale)
	h := int32(cbH * a.cbScale)
	hwnd, _, _ := pCreateWindowExW.Call(
		WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE,
		uintptr(unsafe.Pointer(u16("YTFrameControls"))),
		uintptr(unsafe.Pointer(u16("YTFrame controls"))),
		WS_POPUP,
		0, 0, uintptr(w), uintptr(h),
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		logf("failed to create control bar")
		return
	}
	a.cbHwnd = hwnd
	a.cbHover = -1
	a.cbPress = -1
	a.cbLayout()
	logf("control bar created")
}

func (a *App) cbEnsureGDI() {
	if a.cbReady {
		return
	}
	s := a.cbScale
	mk := func(px float64, weight int32) uintptr {
		h, _, _ := pCreateFontW.Call(
			uintptr(int32(-px*s)), 0, 0, 0, uintptr(weight),
			0, 0, 0, DEFAULT_CHARSET, 0, 0, CLEARTYPE_QUALITY,
			uintptr(DEFAULT_PITCH|FF_DONTCARE),
			uintptr(unsafe.Pointer(u16("Segoe UI"))),
		)
		return h
	}
	a.cbFontBody = mk(15, FW_SEMIBOLD)
	a.cbFontSmall = mk(11, FW_NORMAL)
	a.cbFontTime = mk(12.5, FW_NORMAL)
	a.cbBrBg, _, _ = pCreateSolidBrush.Call(colBg)
	a.cbBrPanel, _, _ = pCreateSolidBrush.Call(colField)
	a.cbBrHover, _, _ = pCreateSolidBrush.Call(rgb(37, 42, 49))
	a.cbBrAccent, _, _ = pCreateSolidBrush.Call(colAccent)
	a.cbBrIcon, _, _ = pCreateSolidBrush.Call(colText)
	a.cbBrMuted, _, _ = pCreateSolidBrush.Call(colMuted)
	a.cbPenBorder, _, _ = pCreatePen.Call(PS_SOLID, 1, colBorder)
	a.cbPenIcon, _, _ = pCreatePen.Call(PS_SOLID, uintptr(maxInt(1, int(1.6*s))), colText)
	a.cbPenMuted, _, _ = pCreatePen.Call(PS_SOLID, uintptr(maxInt(1, int(1.4*s))), colMuted)
	a.cbPenAccent, _, _ = pCreatePen.Call(PS_SOLID, uintptr(maxInt(1, int(2*s))), colAccent)
	a.cbReady = true
}

func (a *App) cbFreeGDI() {
	if a.cbBackDC != 0 {
		pDeleteDC.Call(a.cbBackDC)
		a.cbBackDC = 0
	}
	if a.cbBackBmp != 0 {
		pDeleteObject.Call(a.cbBackBmp)
		a.cbBackBmp = 0
	}
	for _, h := range []uintptr{a.cbFontBody, a.cbFontSmall, a.cbFontTime,
		a.cbBrBg, a.cbBrPanel, a.cbBrHover, a.cbBrAccent, a.cbBrIcon, a.cbBrMuted,
		a.cbPenBorder, a.cbPenIcon, a.cbPenMuted, a.cbPenAccent} {
		if h != 0 {
			pDeleteObject.Call(h)
		}
	}
	a.cbReady = false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (a *App) cbLayout() {
	if a.cbHwnd == 0 {
		return
	}
	var rc RECT
	pGetClientRect.Call(a.cbHwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.Right, rc.Bottom
	s := a.cbScale
	R := func(x, y, ww, hh float64) RECT {
		return RECT{
			Left:   int32(math.Round(x * s)),
			Top:    int32(math.Round(y * s)),
			Right:  int32(math.Round((x + ww) * s)),
			Bottom: int32(math.Round((y + hh) * s)),
		}
	}
	// Clear top-level actions, a full-width timeline and a balanced transport row.
	a.cbSeekRect = R(22, 78, 556, 4)
	a.cbTimeRect = R(372, 43, 206, 22)
	a.cbBtn[cbBtnBack] = R(30, 103, 36, 36)
	a.cbBtn[cbBtnPlay] = R(74, 99, 44, 44)
	a.cbBtn[cbBtnFwd] = R(126, 103, 36, 36)
	a.cbBtn[cbBtnVol] = R(186, 103, 36, 36)
	a.cbVolRect = R(232, 119, 100, 4)
	a.cbBtn[cbBtnFullscreen] = R(374, 103, 36, 36)
	a.cbBtn[cbBtnEye] = R(422, 103, 36, 36)
	a.cbBtn[cbBtnLayouts] = R(470, 103, 36, 36)
	a.cbBtn[cbBtnPin] = R(518, 103, 36, 36)
	a.cbBtn[cbBtnOpen] = R(356, 11, 64, 30)
	a.cbBtn[cbBtnBrowse] = R(426, 11, 84, 30)
	a.cbBtn[cbBtnMenu] = R(516, 11, 30, 30)
	a.cbBtn[cbBtnHide] = R(552, 11, 30, 30)

	r := int32(16 * s)
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(h+1), uintptr(r*2), uintptr(r*2))
	if rgn != 0 {
		pSetWindowRgn.Call(a.cbHwnd, rgn, 1)
	}
}

func (a *App) cbPosition(w, h int32) (int32, int32) {
	x, y := a.settings.BarX, a.settings.BarY
	if !a.settings.BarHasPos {
		var pt POINT
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		work := workAreaForPoint(pt)
		x = work.Right - w - int32(24*a.cbScale)
		y = work.Bottom - h - int32(24*a.cbScale)
	}
	work := workAreaAt(x+w/2, y+h/2)
	x = clamp(x, work.Left, work.Right-w)
	y = clamp(y, work.Top, work.Bottom-h)
	return x, y
}

func (a *App) showControlBar() {
	if a.cbHwnd == 0 {
		a.createControlBar()
	}
	if a.cbHwnd == 0 {
		return
	}
	w := int32(cbW * a.cbScale)
	h := int32(cbH * a.cbScale)
	x, y := a.cbPosition(w, h)
	z := ^uintptr(1) // HWND_NOTOPMOST
	if a.settings.BarOntop {
		z = ^uintptr(0) // HWND_TOPMOST
	}
	pSetWindowPos.Call(a.cbHwnd, z, uintptr(x), uintptr(y), uintptr(w), uintptr(h), SWP_NOACTIVATE)
	a.cbReadState()
	a.cbVisible = true
	a.cbLayout()
	pShowWindow.Call(a.cbHwnd, SW_SHOWNOACTIVATE)
	a.cbInvalidate()
	a.applyVideoVisibility()
}

func (a *App) hideControlBar() {
	if a.cbHwnd == 0 || !a.cbVisible {
		return
	}
	pShowWindow.Call(a.cbHwnd, SW_HIDE)
	a.cbVisible = false
	a.cbDrag = false
	a.cbSeekDrag = false
	a.cbVolDrag = false
	pReleaseCapture.Call()
	a.cbPress = -1
	a.applyVideoVisibility()
}

func (a *App) setControlBarVisible(on bool) {
	a.settings.BarVisible = on
	saveSettings(a.settings)
	if on {
		a.showControlBar()
	} else {
		a.hideControlBar()
	}
	a.applyVideoVisibility()
	a.invalidateBar()
}

func (a *App) toggleControlBar() {
	a.setControlBarVisible(!a.settings.BarVisible)
}

func (a *App) setBarOntop(on bool) {
	a.settings.BarOntop = on
	saveSettings(a.settings)
	if a.cbHwnd != 0 {
		z := ^uintptr(1)
		if on {
			z = ^uintptr(0)
		}
		pSetWindowPos.Call(a.cbHwnd, z, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	}
	a.cbInvalidate()
}

func (a *App) cbInvalidate() {
	a.cbInvalidations++
	if a.cbHwnd != 0 {
		pInvalidateRect.Call(a.cbHwnd, 0, 0)
	}
}

func (a *App) setBarStatus(s string) {
	a.cbStatus = s
	a.cbStatusUntil = time.Now().Add(1500 * time.Millisecond)
	a.cbInvalidate()
}

func (a *App) cbHitButton(x, y int32) int {
	for i, r := range a.cbBtn {
		if pointIn(r, x, y) {
			return i
		}
	}
	return -1
}

func (a *App) cbHitSeek(x, y int32) bool {
	if a.cbIdle || a.loading || a.cbLastDur <= 0 {
		return false
	}
	r := a.cbSeekRect
	return x >= r.Left && x <= r.Right && y >= r.Top-int32(8*a.cbScale) && y <= r.Bottom+int32(8*a.cbScale)
}

func (a *App) cbHitVolume(x, y int32) bool {
	if a.cbIdle || a.loading {
		return false
	}
	r := a.cbVolRect
	m := int32(8 * a.cbScale)
	return x >= r.Left && x <= r.Right && y >= r.Top-m && y <= r.Bottom+m
}

func (a *App) cbSeekAt(x int32) float64 {
	r := a.cbSeekRect
	if r.Right <= r.Left {
		return 0
	}
	frac := float64(x-r.Left) / float64(r.Right-r.Left)
	frac = math.Max(0, math.Min(1, frac))
	return frac * a.cbLastDur
}

func (a *App) cbSetVolumeAt(x int32) {
	r := a.cbVolRect
	if r.Right <= r.Left {
		return
	}
	frac := float64(x-r.Left) / float64(r.Right-r.Left)
	frac = math.Max(0, math.Min(1, frac))
	vol := int(math.Round(frac * 130))
	a.cbLastVol = vol
	if p := a.player; p != nil {
		p.command("set_property", "volume", vol)
		if vol > 0 {
			p.command("set_property", "mute", false)
		}
	}
	a.cbInvalidate()
}

func cbWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logf("control bar panic: %v", r)
		}
	}()
	if app == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, msg, wparam, lparam)
		return r
	}
	a := app
	switch msg {
	case WM_CREATE:
		a.cbHwnd = hwnd
		a.cbEnsureGDI()
		a.cbLayout()
		return 0
	case WM_DPICHANGED:
		a.cbScale = float64(loword(wparam)) / 96
		a.cbFreeGDI()
		a.cbEnsureGDI()
		r := (*RECT)(unsafe.Pointer(lparam))
		pSetWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(int32(cbW*a.cbScale)), uintptr(int32(cbH*a.cbScale)), SWP_NOZORDER|SWP_NOACTIVATE)
		a.cbLayout()
		a.cbInvalidate()
		return 0
	case WM_SIZE:
		a.cbLayout()
		return 0
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if hdc != 0 {
			a.cbPaint(hdc)
			pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		}
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_MOUSEMOVE:
		x, y := mouseX(lparam), mouseY(lparam)
		if a.cbDrag {
			var pt POINT
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			nx := a.cbDragRect.Left + (pt.X - a.cbDragStart.X)
			ny := a.cbDragRect.Top + (pt.Y - a.cbDragStart.Y)
			pSetWindowPos.Call(a.cbHwnd, 0, uintptr(nx), uintptr(ny), 0, 0,
				SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE)
			return 0
		}
		if a.cbSeekDrag {
			a.cbSeekTarget = a.cbSeekAt(x)
			a.cbInvalidate()
			return 0
		}
		if a.cbVolDrag {
			a.cbSetVolumeAt(x)
			return 0
		}
		h := a.cbHitButton(x, y)
		if h != a.cbHover {
			a.cbHover = h
			a.cbInvalidate()
		}
		var tme TRACKMOUSEEVENT
		tme.CbSize = uint32(unsafe.Sizeof(tme))
		tme.DwFlags = TME_LEAVE
		tme.HwndTrack = hwnd
		pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
		return 0
	case WM_MOUSELEAVE:
		if a.cbHover != -1 {
			a.cbHover = -1
			a.cbInvalidate()
		}
		return 0
	case WM_LBUTTONDOWN:
		x, y := mouseX(lparam), mouseY(lparam)
		if a.cbHitSeek(x, y) {
			a.cbSeekDrag = true
			a.cbSeekTarget = a.cbSeekAt(x)
			pSetCapture.Call(hwnd)
			a.cbInvalidate()
			return 0
		}
		if a.cbHitVolume(x, y) {
			a.cbVolDrag = true
			a.cbSetVolumeAt(x)
			pSetCapture.Call(hwnd)
			return 0
		}
		if b := a.cbHitButton(x, y); b >= 0 {
			a.cbPress = b
			pSetCapture.Call(hwnd)
			a.cbInvalidate()
			return 0
		}
		a.cbDrag = true
		var pt POINT
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		a.cbDragStart = pt
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&a.cbDragRect)))
		pSetCapture.Call(hwnd)
		return 0
	case WM_LBUTTONUP:
		x, y := mouseX(lparam), mouseY(lparam)
		if a.cbSeekDrag {
			a.cbSeekDrag = false
			pReleaseCapture.Call()
			if p := a.player; p != nil && a.cbLastDur > 0 {
				p.seekAbsolute(a.cbSeekTarget)
			}
			a.cbInvalidate()
			return 0
		}
		if a.cbVolDrag {
			a.cbVolDrag = false
			pReleaseCapture.Call()
			return 0
		}
		if a.cbDrag {
			a.cbDrag = false
			pReleaseCapture.Call()
			a.cbSavePosition()
			return 0
		}
		if a.cbPress >= 0 {
			b := a.cbPress
			a.cbPress = -1
			pReleaseCapture.Call()
			a.cbInvalidate()
			if a.cbHitButton(x, y) == b && a.cbEnabled(b) {
				a.cbAction(b)
			}
		}
		return 0
	case WM_CAPTURECHANGED, WM_CANCELMODE:
		a.cbDrag, a.cbSeekDrag, a.cbVolDrag = false, false, false
		a.cbPress = -1
		a.cbInvalidate()
		return 0
	case WM_MOUSEWHEEL:
		delta := int16(hiword(wparam))
		if p := a.player; p != nil {
			vol := a.cbLastVol
			if delta > 0 {
				vol += 5
			} else {
				vol -= 5
			}
			vol = maxInt(0, vol)
			if vol > 130 {
				vol = 130
			}
			a.cbLastVol = vol
			p.command("set_property", "volume", vol)
			if vol > 0 {
				p.command("set_property", "mute", false)
			}
			a.cbInvalidate()
		}
		return 0
	case WM_RBUTTONUP:
		a.cbShowMenu()
		return 0
	case WM_SETCURSOR:
		var pt POINT
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		var rc RECT
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		cx, cy := pt.X-rc.Left, pt.Y-rc.Top
		if a.cbHitButton(cx, cy) >= 0 || a.cbHitSeek(cx, cy) || a.cbHitVolume(cx, cy) {
			h, _, _ := pLoadCursorW.Call(0, IDC_HAND)
			pSetCursor.Call(h)
			return 1
		}
	case WM_CLOSE:
		a.setControlBarVisible(false)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

func (a *App) cbSavePosition() {
	if a.cbHwnd == 0 {
		return
	}
	var rc RECT
	if ok, _, _ := pGetWindowRect.Call(a.cbHwnd, uintptr(unsafe.Pointer(&rc))); ok == 0 {
		return
	}
	work := monitorWork(a.cbHwnd)
	x := clamp(rc.Left, work.Left, work.Right-(rc.Right-rc.Left))
	y := clamp(rc.Top, work.Top, work.Bottom-(rc.Bottom-rc.Top))
	if x != rc.Left || y != rc.Top {
		pSetWindowPos.Call(a.cbHwnd, 0, uintptr(x), uintptr(y), 0, 0,
			SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE)
	}
	a.settings.BarX = x
	a.settings.BarY = y
	a.settings.BarHasPos = true
	saveSettings(a.settings)
}

func (a *App) cbAction(b int) {
	p := a.player
	switch b {
	case cbBtnPlay:
		if a.loading {
			a.cancelLoading()
			return
		}
		if a.cbIdle {
			a.showBar()
			return
		}
		if p != nil {
			p.togglePause()
		}
	case cbBtnBack:
		if p != nil {
			p.seekRelative(-10)
		}
	case cbBtnFwd:
		if p != nil {
			p.seekRelative(10)
		}
	case cbBtnVol:
		if p != nil {
			p.toggleMute()
		}
	case cbBtnFullscreen:
		if p != nil {
			if a.videoHidden {
				a.setVideoMode("visible")
			}
			p.toggleFullscreen()
		}
	case cbBtnEye:
		switch a.settings.VideoMode {
		case "visible":
			a.setVideoMode("auto")
		case "auto":
			a.setVideoMode("hidden")
		default:
			a.setVideoMode("visible")
		}
	case cbBtnLayouts:
		a.showLayoutsMenu()
	case cbBtnPin:
		a.setBarOntop(!a.settings.BarOntop)
	case cbBtnOpen:
		a.showBar()
	case cbBtnBrowse:
		a.openBrowse()
	case cbBtnHide:
		a.setControlBarVisible(false)
	case cbBtnMenu:
		a.cbShowMenu()
	}
}

func (a *App) cbReadState() bool {
	st := PlayerState{Pause: true, Volume: a.settings.Volume}
	idle := a.player == nil || !a.player.isRunning() || a.player.urlNow() == ""
	if !idle {
		st = a.player.snapshot()
	}
	changed := a.cbIdle != idle || a.cbTitle != st.Title || a.cbLastPause != st.Pause ||
		int(a.cbLastPos*10) != int(st.Pos*10) || a.cbLastDur != st.Duration || a.cbLastVol != st.Volume ||
		a.cbLastMute != st.Mute || a.cbBuffering != st.Buffering || a.cbBufferPercent != int(st.BufferPercent) || a.cbLastLoading != a.loading
	a.cbIdle, a.cbTitle, a.cbLastPause = idle, st.Title, st.Pause
	a.cbLastPos, a.cbLastDur, a.cbLastVol, a.cbLastMute = st.Pos, st.Duration, st.Volume, st.Mute
	a.cbBuffering, a.cbBufferPercent, a.cbLastLoading = st.Buffering, int(st.BufferPercent), a.loading
	return changed
}

func (a *App) cbTick() {
	if !a.cbVisible || a.cbHwnd == 0 {
		return
	}
	pause := a.cbLastPause
	changed := a.cbReadState()
	if pause != a.cbLastPause {
		a.applyVideoVisibility()
	}
	if a.cbStatus != "" && time.Now().After(a.cbStatusUntil) {
		a.cbStatus = ""
		changed = true
	}
	// The indeterminate loading indicator advances only while a request is active.
	if a.loading {
		changed = true
		a.invalidateBar()
	}
	if changed {
		a.cbInvalidate()
	}
}

func (a *App) cbEnabled(id int) bool {
	switch id {
	case cbBtnBack, cbBtnFwd:
		return !a.loading && !a.cbIdle && a.cbLastDur > 0
	case cbBtnVol, cbBtnFullscreen:
		return !a.loading && !a.cbIdle
	}
	return true
}

func (a *App) cbPaint(hdc uintptr) {
	a.cbPaintCount++
	var rc RECT
	pGetClientRect.Call(a.cbHwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.Right, rc.Bottom
	if w <= 0 || h <= 0 {
		return
	}
	if a.cbBackDC == 0 || a.cbBackW != w || a.cbBackH != h {
		if a.cbBackDC != 0 {
			pDeleteDC.Call(a.cbBackDC)
		}
		if a.cbBackBmp != 0 {
			pDeleteObject.Call(a.cbBackBmp)
		}
		a.cbBackDC, _, _ = pCreateCompatibleDC.Call(hdc)
		a.cbBackBmp, _, _ = pCreateCompatibleBmp.Call(hdc, uintptr(w), uintptr(h))
		a.cbBackW, a.cbBackH = w, h
		pSelectObject.Call(a.cbBackDC, a.cbBackBmp)
	}
	dc := a.cbBackDC
	if dc == 0 {
		return
	}
	s := a.cbScale
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&rc)), a.cbBrBg)
	pSelectObject.Call(dc, pGetStockObject2(NULL_BRUSH))
	pSelectObject.Call(dc, a.cbPenBorder)
	pRoundRect.Call(dc, 1, 1, uintptr(w-1), uintptr(h-1),
		uintptr(int32(32*s)), uintptr(int32(32*s)))

	// Current video, with enough room for long titles.
	pSelectObject.Call(dc, a.cbFontBody)
	pSetTextColor.Call(dc, colText)
	pSetBkMode.Call(dc, TRANSPARENT)
	title := a.cbTitle
	if a.loading {
		title = "Opening video…"
	} else if title == "" {
		title = "Ready when you are"
	}
	drawText(dc, title, RECT{Left: int32(22 * s), Top: int32(10 * s), Right: int32(340 * s), Bottom: int32(38 * s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)
	// Seek bar.
	seek := a.cbSeekRect
	frac := 0.0
	if a.cbLastDur > 0 {
		frac = a.cbLastPos / a.cbLastDur
	}
	if a.cbSeekDrag {
		frac = a.cbSeekTarget / math.Max(1, a.cbLastDur)
	}
	frac = math.Max(0, math.Min(1, frac))
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, a.cbBrPanel)
	pRoundRect.Call(dc, uintptr(seek.Left), uintptr(seek.Top), uintptr(seek.Right), uintptr(seek.Bottom), 4, 4)
	fillW := int32(float64(seek.Right-seek.Left) * frac)
	if a.loading {
		span := int32(80 * s)
		phase := float64(time.Now().UnixMilli()%1400) / 1400
		left := seek.Left + int32(phase*float64(rectW(seek)-span))
		fillRect(dc, RECT{Left: left, Top: seek.Top, Right: left + span, Bottom: seek.Bottom}, a.cbBrAccent)
	}
	if fillW > 0 {
		pSelectObject.Call(dc, a.cbBrAccent)
		pRoundRect.Call(dc, uintptr(seek.Left), uintptr(seek.Top), uintptr(seek.Left+fillW), uintptr(seek.Bottom), 4, 4)
		kr := int32(4 * s)
		cx := seek.Left + fillW
		cy := (seek.Top + seek.Bottom) / 2
		pSelectObject.Call(dc, a.cbBrAccent)
		pEllipse.Call(dc, uintptr(cx-kr), uintptr(cy-kr), uintptr(cx+kr), uintptr(cy+kr))
	}

	// Time / status.
	pSelectObject.Call(dc, a.cbFontTime)
	statusActive := a.cbStatus != "" && time.Now().Before(a.cbStatusUntil)
	var text string
	if statusActive {
		text = a.cbStatus
		pSetTextColor.Call(dc, colAccent)
	} else if a.loading {
		text = "Opening…"
		pSetTextColor.Call(dc, colAccent)
	} else if a.cbIdle {
		text = "Ctrl+Alt+V · Open a link"
		pSetTextColor.Call(dc, colMuted)
	} else if a.cbLastDur > 0 {
		cur := a.cbLastPos
		if a.cbSeekDrag {
			cur = a.cbSeekTarget
		}
		text = clockStr(cur) + " / " + clockStr(a.cbLastDur)
		pSetTextColor.Call(dc, colMuted)
	} else if a.loading {
		text = "Loading…"
		pSetTextColor.Call(dc, colMuted)
	} else {
		text = "LIVE"
		pSetTextColor.Call(dc, colMuted)
	}
	pSetBkMode.Call(dc, TRANSPARENT)
	drawText(dc, text, a.cbTimeRect, DT_RIGHT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)

	// Contextual hints make the icon controls discoverable without tooltips
	// that can cover a small video or take focus from another application.
	pSelectObject.Call(dc, a.cbFontTime)
	pSetTextColor.Call(dc, colMuted)
	hint := a.cbHint()
	drawText(dc, hint, RECT{Left: int32(22 * s), Top: int32(43 * s), Right: int32(366 * s), Bottom: int32(65 * s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)

	// Buttons.
	for i := 0; i < cbBtnCount; i++ {
		r := a.cbBtn[i]
		if i == cbBtnPlay || i == cbBtnBrowse || i == cbBtnOpen || a.cbHover == i || a.cbPress == i {
			br := a.cbBrHover
			if i == cbBtnPlay {
				br = a.cbBrAccent
			}
			if (i == cbBtnBrowse || i == cbBtnOpen) && a.cbHover != i {
				br = a.cbBrPanel
			}
			if a.cbPress == i && i != cbBtnPlay {
				br = a.cbBrPanel
			}
			pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
			pSelectObject.Call(dc, br)
			pRoundRect.Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom),
				uintptr(int32(6*s)), uintptr(int32(6*s)))
		}
		iconBrush, iconPen := a.cbBrIcon, a.cbPenIcon
		if !a.cbEnabled(i) {
			a.cbBrIcon, a.cbPenIcon = a.cbBrMuted, a.cbPenMuted
		}
		switch i {
		case cbBtnPlay:
			if a.loading {
				pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
				pSelectObject.Call(dc, a.cbBrBg)
				cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
				d := int32(6 * s)
				pRoundRect.Call(dc, uintptr(cx-d), uintptr(cy-d), uintptr(cx+d), uintptr(cy+d), 2, 2)
			} else {
				a.cbIconPlay(dc, r, a.cbLastPause)
			}
		case cbBtnBack:
			a.cbIconStep(dc, r, -1)
		case cbBtnFwd:
			a.cbIconStep(dc, r, 1)
		case cbBtnVol:
			a.cbIconVolume(dc, r, a.cbLastMute)
		case cbBtnFullscreen:
			a.cbIconFullscreen(dc, r)
		case cbBtnEye:
			a.cbIconEye(dc, r)
		case cbBtnLayouts:
			a.cbIconLayouts(dc, r)
		case cbBtnPin:
			a.cbIconPin(dc, r)
		case cbBtnOpen, cbBtnBrowse:
			pSelectObject.Call(dc, a.cbFontTime)
			pSetTextColor.Call(dc, colText)
			label := "Browse"
			if i == cbBtnOpen {
				label = "+ Open"
			}
			drawText(dc, label, r, DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
		case cbBtnHide:
			pSelectObject.Call(dc, a.cbPenMuted)
			cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
			pMoveToEx.Call(dc, uintptr(cx-int32(6*s)), uintptr(cy), 0)
			pLineTo.Call(dc, uintptr(cx+int32(6*s)), uintptr(cy))
		case cbBtnMenu:
			a.cbIconMenu(dc, r)
		}
		a.cbBrIcon, a.cbPenIcon = iconBrush, iconPen
	}

	// Volume slider.
	if !a.cbIdle {
		vr := a.cbVolRect
		pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
		pSelectObject.Call(dc, a.cbBrPanel)
		pRoundRect.Call(dc, uintptr(vr.Left), uintptr(vr.Top), uintptr(vr.Right), uintptr(vr.Bottom), 3, 3)
		vf := math.Max(0, math.Min(1, float64(a.cbLastVol)/130.0))
		if a.cbLastMute {
			vf = 0
		}
		vw := int32(float64(vr.Right-vr.Left) * vf)
		if vw > 0 {
			pSelectObject.Call(dc, a.cbBrAccent)
			pRoundRect.Call(dc, uintptr(vr.Left), uintptr(vr.Top), uintptr(vr.Left+vw), uintptr(vr.Bottom), 3, 3)
		}
	}

	// Grip.
	pSelectObject.Call(dc, a.cbBrIcon)
	for row := 0; row < 3; row++ {
		for col := 0; col < 2; col++ {
			cx := int32((12 + float64(col)*4) * s)
			cy := int32((114 + float64(row)*6) * s)
			rr := int32(1.3 * s)
			pEllipse.Call(dc, uintptr(cx-rr), uintptr(cy-rr), uintptr(cx+rr), uintptr(cy+rr))
		}
	}

	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), dc, 0, 0, SRCCOPY)
}

func (a *App) cbIconPlay(dc uintptr, r RECT, paused bool) {
	s := a.cbScale
	cx := (r.Left + r.Right) / 2
	cy := (r.Top + r.Bottom) / 2
	if paused {
		sz := int32(7 * s)
		pts := []POINT{
			{cx - sz/2, cy - sz}, {cx - sz/2, cy + sz}, {cx + sz, cy},
		}
		pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
		pSelectObject.Call(dc, a.cbBrBg)
		pPolygon.Call(dc, uintptr(unsafe.Pointer(&pts[0])), 3)
	} else {
		bw := int32(4 * s)
		gap := int32(4 * s)
		hh := int32(8 * s)
		pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
		pSelectObject.Call(dc, a.cbBrBg)
		pRoundRect.Call(dc, uintptr(cx-gap/2-bw), uintptr(cy-hh), uintptr(cx-gap/2), uintptr(cy+hh), 2, 2)
		pRoundRect.Call(dc, uintptr(cx+gap/2), uintptr(cy-hh), uintptr(cx+gap/2+bw), uintptr(cy+hh), 2, 2)
	}
}

func (a *App) cbIconStep(dc uintptr, r RECT, dir int) {
	s := a.cbScale
	cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	d := int32(11 * s)
	pSelectObject.Call(dc, a.cbPenIcon)
	pSelectObject.Call(dc, pGetStockObject2(NULL_BRUSH))
	if dir < 0 {
		pArc.Call(dc, uintptr(cx-d), uintptr(cy-d), uintptr(cx+d), uintptr(cy+d), uintptr(cx-int32(5*s)), uintptr(cy-d), uintptr(cx-int32(9*s)), uintptr(cy-int32(3*s)))
	} else {
		pArc.Call(dc, uintptr(cx-d), uintptr(cy-d), uintptr(cx+d), uintptr(cy+d), uintptr(cx+int32(9*s)), uintptr(cy-int32(3*s)), uintptr(cx+int32(5*s)), uintptr(cy-d))
	}
	x := cx + int32(dir)*int32(8*s)
	y := cy - int32(9*s)
	pts := []POINT{{x, y - int32(4*s)}, {x - int32(dir)*int32(6*s), y}, {x, y + int32(4*s)}}
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, a.cbBrIcon)
	pPolygon.Call(dc, uintptr(unsafe.Pointer(&pts[0])), 3)
	pSelectObject.Call(dc, a.cbFontSmall)
	pSetTextColor.Call(dc, colText)
	pSetBkMode.Call(dc, TRANSPARENT)
	drawText(dc, "10", RECT{Left: cx - d, Top: cy - int32(7*s), Right: cx + d, Bottom: cy + int32(9*s)}, DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
}

func (a *App) cbIconVolume(dc uintptr, r RECT, muted bool) {
	s := a.cbScale
	cx := (r.Left + r.Right) / 2
	cy := (r.Top + r.Bottom) / 2
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	pts := []POINT{
		{cx - px(7), cy - px(3)}, {cx - px(3), cy - px(3)}, {cx + px(2), cy - px(7)},
		{cx + px(2), cy + px(7)}, {cx - px(3), cy + px(3)}, {cx - px(7), cy + px(3)},
	}
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, a.cbBrIcon)
	pPolygon.Call(dc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	pSelectObject.Call(dc, a.cbPenIcon)
	if muted {
		pMoveToEx.Call(dc, uintptr(cx+px(2)), uintptr(cy-px(7)), 0)
		pLineTo.Call(dc, uintptr(cx+px(10)), uintptr(cy+px(7)))
	} else {
		pMoveToEx.Call(dc, uintptr(cx+px(6)), uintptr(cy-px(5)), 0)
		pLineTo.Call(dc, uintptr(cx+px(10)), uintptr(cy-px(2)))
		pMoveToEx.Call(dc, uintptr(cx+px(6)), uintptr(cy+px(5)), 0)
		pLineTo.Call(dc, uintptr(cx+px(10)), uintptr(cy+px(2)))
	}
}

func (a *App) cbIconFullscreen(dc uintptr, r RECT) {
	s := a.cbScale
	cx := (r.Left + r.Right) / 2
	cy := (r.Top + r.Bottom) / 2
	d := int32(7 * s)
	pSelectObject.Call(dc, a.cbPenIcon)
	pSelectObject.Call(dc, pGetStockObject2(NULL_BRUSH))
	corner := func(x, y, dx, dy int32) {
		pMoveToEx.Call(dc, uintptr(x), uintptr(y), 0)
		pLineTo.Call(dc, uintptr(x+dx), uintptr(y))
		pMoveToEx.Call(dc, uintptr(x), uintptr(y), 0)
		pLineTo.Call(dc, uintptr(x), uintptr(y+dy))
	}
	corner(cx-d, cy-d, d/2, d/2)
	corner(cx+d, cy-d, -d/2, d/2)
	corner(cx-d, cy+d, d/2, -d/2)
	corner(cx+d, cy+d, -d/2, -d/2)
}

func (a *App) cbIconEye(dc uintptr, r RECT) {
	s := a.cbScale
	cx := (r.Left + r.Right) / 2
	cy := (r.Top + r.Bottom) / 2
	w := int32(9 * s)
	h := int32(5 * s)
	pSelectObject.Call(dc, a.cbPenIcon)
	pSelectObject.Call(dc, pGetStockObject2(NULL_BRUSH))
	pEllipse.Call(dc, uintptr(cx-w), uintptr(cy-h), uintptr(cx+w), uintptr(cy+h))
	pSelectObject.Call(dc, a.cbBrIcon)
	pr := int32(2.2 * s)
	pEllipse.Call(dc, uintptr(cx-pr), uintptr(cy-pr), uintptr(cx+pr), uintptr(cy+pr))
	mode := a.settings.VideoMode
	if mode == "auto" {
		pSelectObject.Call(dc, a.cbPenMuted)
		pMoveToEx.Call(dc, uintptr(cx+w-int32(3*s)), uintptr(cy+h-int32(4*s)), 0)
		pLineTo.Call(dc, uintptr(cx+w+int32(1*s)), uintptr(cy+h-int32(1*s)))
		pMoveToEx.Call(dc, uintptr(cx+w+int32(1*s)), uintptr(cy+h-int32(4*s)), 0)
		pLineTo.Call(dc, uintptr(cx+w-int32(3*s)), uintptr(cy+h-int32(1*s)))
	} else if mode == "hidden" {
		pSelectObject.Call(dc, a.cbPenAccent)
		pMoveToEx.Call(dc, uintptr(cx-w), uintptr(cy+h+int32(2*s)), 0)
		pLineTo.Call(dc, uintptr(cx+w), uintptr(cy-h-int32(2*s)))
	}
}

func (a *App) cbIconLayouts(dc uintptr, r RECT) {
	s := a.cbScale
	cx := (r.Left + r.Right) / 2
	cy := (r.Top + r.Bottom) / 2
	sz := int32(5 * s)
	gap := int32(2 * s)
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, a.cbBrIcon)
	for yy := 0; yy < 2; yy++ {
		for xx := 0; xx < 2; xx++ {
			x := cx - sz - gap/2 + int32(xx)*(sz+gap)
			y := cy - sz - gap/2 + int32(yy)*(sz+gap)
			pRoundRect.Call(dc, uintptr(x), uintptr(y), uintptr(x+sz), uintptr(y+sz), 2, 2)
		}
	}
}

func (a *App) cbIconPin(dc uintptr, r RECT) {
	s := a.cbScale
	cx := (r.Left + r.Right) / 2
	cy := (r.Top + r.Bottom) / 2
	pen := a.cbPenIcon
	col := uintptr(colText)
	if a.settings.BarOntop {
		pen = a.cbPenAccent
		col = colAccent
	}
	pSelectObject.Call(dc, pen)
	pSelectObject.Call(dc, pGetStockObject2(NULL_BRUSH))
	pEllipse.Call(dc, uintptr(cx-int32(4*s)), uintptr(cy-int32(9*s)),
		uintptr(cx+int32(4*s)), uintptr(cy-int32(1*s)))
	pMoveToEx.Call(dc, uintptr(cx), uintptr(cy-int32(1*s)), 0)
	pLineTo.Call(dc, uintptr(cx), uintptr(cy+int32(8*s)))
	pMoveToEx.Call(dc, uintptr(cx-int32(5*s)), uintptr(cy+int32(9*s)), 0)
	pLineTo.Call(dc, uintptr(cx+int32(5*s)), uintptr(cy+int32(9*s)))
	_ = col
}

func (a *App) cbIconBrowse(dc uintptr, r RECT) {
	s := a.cbScale
	cx := (r.Left + r.Right) / 2
	cy := (r.Top + r.Bottom) / 2
	pSelectObject.Call(dc, a.cbPenIcon)
	pSelectObject.Call(dc, pGetStockObject2(NULL_BRUSH))
	pRoundRect.Call(dc, uintptr(cx-int32(9*s)), uintptr(cy-int32(6*s)),
		uintptr(cx+int32(9*s)), uintptr(cy+int32(6*s)),
		uintptr(int32(4*s)), uintptr(int32(4*s)))
	pts := []POINT{{cx - int32(2*s), cy - int32(4*s)}, {cx - int32(2*s), cy + int32(4*s)}, {cx + int32(5*s), cy}}
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, a.cbBrIcon)
	pPolygon.Call(dc, uintptr(unsafe.Pointer(&pts[0])), 3)
}

func (a *App) cbIconClose(dc uintptr, r RECT) {
	s := a.cbScale
	cx := (r.Left + r.Right) / 2
	cy := (r.Top + r.Bottom) / 2
	d := int32(5 * s)
	pSelectObject.Call(dc, a.cbPenIcon)
	pMoveToEx.Call(dc, uintptr(cx-d), uintptr(cy-d), 0)
	pLineTo.Call(dc, uintptr(cx+d), uintptr(cy+d))
	pMoveToEx.Call(dc, uintptr(cx+d), uintptr(cy-d), 0)
	pLineTo.Call(dc, uintptr(cx-d), uintptr(cy+d))
}

func (a *App) cbIconMenu(dc uintptr, r RECT) {
	s := a.cbScale
	cx := (r.Left + r.Right) / 2
	cy := (r.Top + r.Bottom) / 2
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, a.cbBrIcon)
	for i := -1; i <= 1; i++ {
		x := cx + int32(i)*int32(6*s)
		rr := int32(1.6 * s)
		pEllipse.Call(dc, uintptr(x-rr), uintptr(cy-rr), uintptr(x+rr), uintptr(cy+rr))
	}
}

func clockStr(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	t := int(sec)
	h := t / 3600
	m := (t % 3600) / 60
	s := t % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// ---- video visibility ----------------------------------------------------

func videoModeLabel(m string) string {
	switch m {
	case "visible":
		return "Show always"
	case "hidden":
		return "Always hidden (audio only)"
	default:
		return "Hide on pause"
	}
}

func (a *App) setVideoMode(m string) {
	switch m {
	case "auto", "visible", "hidden":
	default:
		m = "auto"
	}
	a.settings.VideoMode = m
	saveSettings(a.settings)
	a.applyVideoVisibility()
	a.setBarStatus("Video: " + videoModeLabel(m))
	logf("video mode -> %s", m)
}

func (a *App) applyVideoVisibility() {
	p := a.player
	if p == nil || !p.isRunning() {
		return
	}
	hwnd := p.hwndNow()
	if hwnd == 0 {
		return
	}
	st := p.snapshot()
	mode := a.settings.VideoMode
	hide := p.urlNow() == ""
	if !hide {
		switch mode {
		case "hidden":
			hide = true
		case "visible":
			hide = false
		default:
			hide = st.Pause && !st.Eof && a.cbVisible && !a.barVisible && !a.eofShown
		}
	}
	if hide == a.videoHidden {
		return
	}
	if hide {
		pShowWindow.Call(hwnd, SW_HIDE)
		pSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
			SWP_HIDEWINDOW|SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE)
		a.videoHidden = true
		logf("video hidden")
	} else {
		pSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
			SWP_SHOWWINDOW|SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE)
		pShowWindow.Call(hwnd, SW_SHOW)
		a.videoHidden = false
		pSetForegroundWindow.Call(hwnd)
		logf("video shown")
	}
}

// ---- layouts -------------------------------------------------------------

func (a *App) layoutLabel(i int) string {
	l := a.settings.Layouts[i]
	if !l.Used {
		return fmt.Sprintf("Layout %d — empty", i+1)
	}
	return fmt.Sprintf("Layout %d — %d×%d at %d,%d", i+1, l.Width, l.Height, l.X, l.Y)
}

func (a *App) captureLayout() Layout {
	l := Layout{}
	if p := a.player; p != nil {
		st := p.snapshot()
		hwnd := p.hwndNow()
		var rc RECT
		if hwnd != 0 && !st.Fullscreen &&
			func() bool { ok, _, _ := pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc))); return ok != 0 }() {
			l.X, l.Y = rc.Left, rc.Top
			l.Width, l.Height = rectW(rc), rectH(rc)
		}
	}
	if l.Width < 80 || l.Height < 60 {
		l.X, l.Y = a.settings.X, a.settings.Y
		l.Width, l.Height = a.settings.Width, a.settings.Height
	}
	if a.cbHwnd != 0 {
		var rc RECT
		if ok, _, _ := pGetWindowRect.Call(a.cbHwnd, uintptr(unsafe.Pointer(&rc))); ok != 0 {
			l.BarX, l.BarY = rc.Left, rc.Top
			l.BarSet = true
		}
	}
	if !l.BarSet && a.settings.BarHasPos {
		l.BarX, l.BarY = a.settings.BarX, a.settings.BarY
		l.BarSet = true
	}
	l.Used = true
	l.SavedAt = nowUnix()
	return l
}

func (a *App) saveLayout(i int) {
	if i < 0 || i >= layoutSlots {
		return
	}
	a.settings.Layouts[i] = a.captureLayout()
	saveSettings(a.settings)
	a.setBarStatus(fmt.Sprintf("Layout %d saved", i+1))
	logf("layout %d saved", i+1)
}

func (a *App) applyLayout(i int) {
	if i < 0 || i >= layoutSlots {
		return
	}
	l := a.settings.Layouts[i]
	if !l.Used {
		a.setBarStatus(fmt.Sprintf("Layout %d is empty", i+1))
		return
	}
	p := a.player
	if p == nil || !p.isRunning() {
		a.settings.X, a.settings.Y = l.X, l.Y
		a.settings.Width, a.settings.Height = l.Width, l.Height
		a.settings.HasRect = true
		if l.BarSet {
			a.settings.BarX, a.settings.BarY = l.BarX, l.BarY
			a.settings.BarHasPos = true
		}
		saveSettings(a.settings)
		a.setBarStatus(fmt.Sprintf("Layout %d will apply to the next video", i+1))
		return
	}
	st := p.snapshot()
	if st.Fullscreen {
		p.command("set_property", "fullscreen", false)
		a.pendingLayout = i
		pSetTimer.Call(helperHwnd, TIMER_LAYOUT, 250, 0)
		return
	}
	a.applyLayoutNow(i)
}

func (a *App) applyLayoutNow(i int) {
	l := a.settings.Layouts[i]
	p := a.player
	if p == nil || !l.Used {
		return
	}
	hwnd := p.hwndNow()
	if hwnd == 0 {
		return
	}
	st := p.snapshot()
	w := l.Width
	h := l.Height
	if w < 160 {
		w = 320
	}
	if st.Dwidth > 0 && st.Dheight > 0 {
		h = int32(math.Round(float64(w) * float64(st.Dheight) / float64(st.Dwidth)))
	}
	if h < 90 {
		h = 180
	}
	work := workAreaAt(l.X+w/2, l.Y+h/2)
	if w > rectW(work) {
		w = rectW(work)
		h = int32(math.Round(float64(w) * float64(st.Dheight) / math.Max(1, float64(st.Dwidth))))
	}
	if h > rectH(work) {
		h = rectH(work)
		w = int32(math.Round(float64(h) * float64(st.Dwidth) / math.Max(1, float64(st.Dheight))))
	}
	x := clamp(l.X, work.Left, work.Right-w)
	y := clamp(l.Y, work.Top, work.Bottom-h)
	pSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		SWP_NOZORDER|SWP_NOACTIVATE)
	a.settings.X, a.settings.Y, a.settings.Width, a.settings.Height = x, y, w, h
	a.settings.HasRect = true
	if l.BarSet {
		a.moveBarTo(l.BarX, l.BarY)
	}
	saveSettings(a.settings)
	a.applyVideoVisibility()
	a.setBarStatus(fmt.Sprintf("Layout %d applied", i+1))
	logf("layout %d applied", i+1)
}

func (a *App) moveBarTo(x, y int32) {
	a.settings.BarX, a.settings.BarY = x, y
	a.settings.BarHasPos = true
	if a.cbHwnd == 0 {
		return
	}
	w := int32(cbW * a.cbScale)
	h := int32(cbH * a.cbScale)
	work := workAreaAt(x+w/2, y+h/2)
	x = clamp(x, work.Left, work.Right-w)
	y = clamp(y, work.Top, work.Bottom-h)
	pSetWindowPos.Call(a.cbHwnd, 0, uintptr(x), uintptr(y), 0, 0,
		SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE)
	a.cbInvalidate()
}

func (a *App) setDefaultLayout(i int) {
	if i < 0 || i >= layoutSlots || !a.settings.Layouts[i].Used {
		a.setBarStatus("Save that layout first")
		return
	}
	a.settings.DefaultLayout = i
	saveSettings(a.settings)
	a.setBarStatus(fmt.Sprintf("Default layout: %d", i+1))
}

func (a *App) clearLayouts() {
	a.settings.Layouts = [layoutSlots]Layout{}
	a.settings.DefaultLayout = -1
	saveSettings(a.settings)
	a.setBarStatus("Layouts cleared")
}

func (a *App) buildLayoutsMenu(menu uintptr) {
	add := func(flags uintptr, id int, text string) {
		pAppendMenuW.Call(menu, flags, uintptr(id), uintptr(unsafe.Pointer(u16(text))))
	}
	for i := 0; i < layoutSlots; i++ {
		flags := uintptr(MF_STRING)
		if !a.settings.Layouts[i].Used {
			flags |= MF_GRAYED
		}
		add(flags, 300+i, a.layoutLabel(i))
	}
	pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	for i := 0; i < layoutSlots; i++ {
		add(MF_STRING, 310+i, fmt.Sprintf("Save current to layout %d", i+1))
	}
	pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	for i := 0; i < layoutSlots; i++ {
		flags := uintptr(MF_STRING)
		if a.settings.DefaultLayout == i {
			flags |= MF_CHECKED
		}
		if !a.settings.Layouts[i].Used {
			flags |= MF_GRAYED
		}
		add(flags, 320+i, fmt.Sprintf("Default: layout %d", i+1))
	}
	pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	add(MF_STRING, 330, "Clear all layouts")
}

func (a *App) showLayoutsMenu() {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu)
	a.buildLayoutsMenu(menu)
	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(helperHwnd)
	cmd, _, _ := pTrackPopupMenu.Call(menu, TPM_RETURNCMD|TPM_RIGHTBUTTON,
		uintptr(pt.X), uintptr(pt.Y), 0, helperHwnd, 0)
	pPostMessageW.Call(helperHwnd, WM_NULL, 0, 0)
	if cmd != 0 {
		a.onMenuCommand(int(cmd))
	}
}

func (a *App) cbShowMenu() {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu)
	add := func(flags uintptr, id int, text string) {
		pAppendMenuW.Call(menu, flags, uintptr(id), uintptr(unsafe.Pointer(u16(text))))
	}
	for _, m := range []struct {
		id   int
		mode string
	}{{120, "visible"}, {121, "auto"}, {122, "hidden"}} {
		flags := uintptr(MF_STRING)
		if a.settings.VideoMode == m.mode {
			flags |= MF_CHECKED
		}
		add(flags, m.id, videoModeLabel(m.mode))
	}
	pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	flags := uintptr(MF_STRING)
	if a.settings.BarOntop {
		flags |= MF_CHECKED
	}
	add(flags, 116, "Control bar always on top")
	ontop := a.settings.Ontop
	if p := a.player; p != nil {
		ontop = p.snapshot().Ontop
	}
	flags = uintptr(MF_STRING)
	if ontop {
		flags |= MF_CHECKED
	}
	add(flags, 104, "Video always on top")

	quality, _, _ := pCreatePopupMenu.Call()
	if quality != 0 {
		for i, q := range []string{"best", "1080", "720", "480"} {
			label := q + "p"
			if q == "best" {
				label = "Best available"
			}
			flags := uintptr(MF_STRING)
			if a.settings.Quality == q {
				flags |= MF_CHECKED
			}
			pAppendMenuW.Call(quality, flags, uintptr(200+i), uintptr(unsafe.Pointer(u16(label))))
		}
		pAppendMenuW.Call(menu, MF_POPUP|MF_STRING, quality, uintptr(unsafe.Pointer(u16("Quality"))))
	}
	lmenu, _, _ := pCreatePopupMenu.Call()
	if lmenu != 0 {
		a.buildLayoutsMenu(lmenu)
		pAppendMenuW.Call(menu, MF_POPUP|MF_STRING, lmenu,
			uintptr(unsafe.Pointer(u16("Layouts"))))
	}

	flags = uintptr(MF_STRING)
	if a.settings.Controls {
		flags |= MF_CHECKED
	}
	add(flags, 115, "In-video controls")
	add(MF_STRING, 140, "Browse YouTube…")
	if a.browseCmd != nil {
		add(MF_STRING, 142, "Close YouTube browser")
	}
	add(MF_STRING, 112, "Hide control bar")

	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(helperHwnd)
	cmd, _, _ := pTrackPopupMenu.Call(menu, TPM_RETURNCMD|TPM_RIGHTBUTTON,
		uintptr(pt.X), uintptr(pt.Y), 0, helperHwnd, 0)
	pPostMessageW.Call(helperHwnd, WM_NULL, 0, 0)
	if cmd != 0 {
		a.onMenuCommand(int(cmd))
	}
}

func (a *App) cbHint() string {
	if a.cbHover == cbBtnPlay && a.loading {
		return "Cancel loading"
	}
	if a.cbHover == cbBtnPlay && a.cbIdle {
		return "Open a video"
	}
	switch a.cbHover {
	case cbBtnPlay:
		if a.cbLastPause {
			return "Play · Ctrl+Alt+Space"
		}
		return "Pause · Ctrl+Alt+Space"
	case cbBtnBack:
		return "Back 10 seconds · Ctrl+Alt+J"
	case cbBtnFwd:
		return "Forward 10 seconds · Ctrl+Alt+L"
	case cbBtnVol:
		if a.cbLastMute {
			return "Unmute · Ctrl+Alt+M"
		}
		return fmt.Sprintf("Volume %d%% · Click to mute", a.cbLastVol)
	case cbBtnFullscreen:
		return "Toggle fullscreen"
	case cbBtnEye:
		return "Video: " + map[string]string{"visible": "always visible", "auto": "hide on pause", "hidden": "audio only"}[a.settings.VideoMode]
	case cbBtnLayouts:
		return "Saved layouts · Ctrl+Alt+1…5"
	case cbBtnPin:
		if a.settings.BarOntop {
			return "Unpin controls"
		}
		return "Keep controls on top"
	case cbBtnOpen:
		return "Paste a link · Ctrl+Alt+V"
	case cbBtnBrowse:
		return "Find another video on YouTube"
	case cbBtnMenu:
		return "More options"
	case cbBtnHide:
		return "Hide controls · Ctrl+Alt+B to restore"
	}
	if a.cbVolDrag {
		return fmt.Sprintf("Volume %d%%", a.cbLastVol)
	}
	if a.cbSeekDrag {
		return "Release to seek"
	}
	if a.loading {
		return "Resolving your video · Click ■ to cancel"
	}
	if a.cbIdle {
		return "Open a link or browse YouTube"
	}
	if a.cbBuffering {
		return fmt.Sprintf("Buffering · %d%%", a.cbBufferPercent)
	}
	if a.cbLastMute {
		return "Muted"
	}
	if a.cbLastPause {
		return "Paused"
	}
	if a.loading {
		return "Connecting to the player"
	}
	return "Playing"
}
