//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

const (
	launcherW      = 640
	launcherH      = 400
	colBg          = 0x001A1614 // #14161a
	colBorder      = 0x00362F2B // #2b2f36
	colField       = 0x00241F1C // #1c1f24
	colFieldBorder = 0x003B3A38 // #383a3b
	colText        = 0x00F5F3F2 // #f2f3f5
	colMuted       = 0x00A6A09A // #9aa0a6
	colAccent      = 0x00FFA63E // #3ea6ff
	colAccentHover = 0x00FFBA66 // #66baff
	colDark        = 0x00100D0B // #0b0d10
	colError       = 0x00808AFF // #ff8a80
)

var oldEditProc, editProcCB uintptr

func init() {
	barProcCB = syscall.NewCallback(barWndProc)
	editProcCB = syscall.NewCallback(func(hwnd, msg, w, l uintptr) uintptr {
		switch msg {
		case WM_KEYDOWN:
			switch w {
			case VK_A:
				if ctrl, _, _ := pGetKeyState.Call(VK_CONTROL); int16(ctrl) < 0 {
					pSendMessageW.Call(hwnd, EM_SETSEL, 0, ^uintptr(0))
					return 0
				}
			case VK_TAB:
				if app != nil {
					app.cycleLauncherFocus()
				}
				return 0
			case VK_RETURN:
				if app != nil {
					app.submit()
				}
				return 0
			case VK_ESCAPE:
				if app != nil {
					app.hideBar(true)
				}
				return 0
			}
		case WM_SETFOCUS, WM_KILLFOCUS:
			if app != nil {
				app.invalidateBar()
			}
		case WM_CHAR:
			if w == 1 || w == 9 || w == 13 || w == 27 {
				return 0
			}
		}
		if oldEditProc != 0 {
			r, _, _ := pCallWindowProcW.Call(oldEditProc, hwnd, msg, w, l)
			return r
		}
		r, _, _ := pDefWindowProcW.Call(hwnd, msg, w, l)
		return r
	})
}

func (a *App) ensureGDI() {
	if a.gdiReady {
		return
	}
	s := a.scale
	mkFont := func(px float64, weight int32) uintptr {
		h, _, _ := pCreateFontW.Call(
			uintptr(int32(-px*s)),
			0, 0, 0,
			uintptr(weight),
			0, 0, 0,
			DEFAULT_CHARSET,
			0, 0, CLEARTYPE_QUALITY,
			uintptr(DEFAULT_PITCH|FF_DONTCARE),
			uintptr(unsafe.Pointer(u16("Segoe UI"))),
		)
		return h
	}
	a.fontBrand = mkFont(24, FW_SEMIBOLD)
	a.fontTitle = mkFont(13, FW_SEMIBOLD)
	a.fontBody = mkFont(14, FW_NORMAL)
	a.fontHint = mkFont(12, FW_NORMAL)
	a.fontClose = mkFont(19, FW_NORMAL)

	a.brBg, _, _ = pCreateSolidBrush.Call(colBg)
	a.brField, _, _ = pCreateSolidBrush.Call(colField)
	a.brAccent, _, _ = pCreateSolidBrush.Call(colAccent)
	a.brAccentHover, _, _ = pCreateSolidBrush.Call(colAccentHover)
	a.penBorder, _, _ = pCreatePen.Call(PS_SOLID, 1, colBorder)
	a.penField, _, _ = pCreatePen.Call(PS_SOLID, 1, colFieldBorder)
	a.penFocus, _, _ = pCreatePen.Call(PS_SOLID, uintptr(maxInt(1, int(2*s))), colAccent)
	a.penError, _, _ = pCreatePen.Call(PS_SOLID, 1, colError)
	a.penClose, _, _ = pCreatePen.Call(PS_SOLID, uintptr(int32(1.6*s)), colMuted)
	a.gdiReady = true
}

func (a *App) freeGDI() {
	if a.backDC != 0 {
		pDeleteDC.Call(a.backDC)
		a.backDC = 0
	}
	if a.backBmp != 0 {
		pDeleteObject.Call(a.backBmp)
		a.backBmp = 0
	}
	for _, h := range []uintptr{a.fontBrand, a.fontTitle, a.fontBody, a.fontHint, a.fontClose,
		a.brBg, a.brField, a.brAccent, a.brAccentHover, a.penBorder, a.penField, a.penClose, a.penFocus, a.penError} {
		if h != 0 {
			pDeleteObject.Call(h)
		}
	}
	a.gdiReady = false
}

var barProcCB uintptr

func barWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logf("bar panic: %v", r)
		}
	}()
	if app == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, msg, wparam, lparam)
		return r
	}
	a := app
	switch msg {
	case WM_CREATE:
		a.barHwnd = hwnd
		a.createEdit()
		return 0
	case WM_DPICHANGED:
		a.scale = float64(loword(wparam)) / 96
		a.freeGDI()
		a.ensureGDI()
		pSendMessageW.Call(a.editHwnd, WM_SETFONT, a.fontBody, 1)
		r := (*RECT)(unsafe.Pointer(lparam))
		pSetWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(int32(launcherW*a.scale)), uintptr(int32(a.launcherHeight()*a.scale)), SWP_NOZORDER|SWP_NOACTIVATE)
		a.layoutBar()
		a.invalidateBar()
		return 0
	case WM_SIZE:
		a.layoutBar()
		a.invalidateBar()
		return 0
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if hdc != 0 {
			a.paintBar(hdc)
			pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		}
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_MOUSEMOVE:
		x, y := mouseX(lparam), mouseY(lparam)
		hit := a.launcherHit(x, y)
		if hit != a.launcherHover {
			a.launcherHover = hit
			a.hoverBtn = hit == 1
			a.hoverBrowse = hit == 2
			a.hoverClose = hit == 6
			a.invalidateBar()
		}
		tme := TRACKMOUSEEVENT{CbSize: uint32(unsafe.Sizeof(TRACKMOUSEEVENT{})), DwFlags: TME_LEAVE, HwndTrack: hwnd}
		pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
		return 0
	case WM_MOUSELEAVE:
		a.launcherHover = 0
		a.hoverBtn, a.hoverBrowse, a.hoverClose = false, false, false
		a.invalidateBar()
		return 0
	case WM_SETCURSOR:
		var pt POINT
		var rc RECT
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		if a.launcherHit(pt.X-rc.Left, pt.Y-rc.Top) > 0 {
			pSetCursor.Call(loadCursor(IDC_HAND))
			return 1
		}
	case WM_LBUTTONDOWN:
		a.launcherPress = a.launcherHit(mouseX(lparam), mouseY(lparam))
		if a.launcherPress > 0 {
			pSetCapture.Call(hwnd)
			a.invalidateBar()
			return 0
		}
		pReleaseCapture.Call()
		pSendMessageW.Call(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0)
		return 0
	case WM_LBUTTONUP:
		pressed := a.launcherPress
		a.launcherPress = 0
		pReleaseCapture.Call()
		a.invalidateBar()
		if pressed > 0 && pressed == a.launcherHit(mouseX(lparam), mouseY(lparam)) {
			a.launcherAction(pressed)
		}
		return 0
	case WM_CAPTURECHANGED, WM_CANCELMODE:
		a.launcherPress = 0
		a.invalidateBar()
		return 0
	case WM_SETFOCUS, WM_KILLFOCUS:
		a.invalidateBar()
		return 0
	case WM_KEYDOWN:
		if wparam == VK_TAB {
			a.cycleLauncherFocus()
			return 0
		}
		if wparam == VK_SPACE || wparam == VK_RETURN {
			a.launcherAction(a.launcherFocus)
			return 0
		}
		if wparam == VK_ESCAPE {
			a.hideBar(true)
			return 0
		}
	case WM_CTLCOLOREDIT:
		hdc := wparam
		pSetTextColor.Call(hdc, colText)
		pSetBkColor.Call(hdc, colField)
		if a.brField != 0 {
			return a.brField
		}
	case WM_CLOSE:
		a.hideBar(true)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

func (a *App) createEdit() {
	hwnd, _, _ := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(u16("EDIT"))),
		0,
		WS_CHILD|WS_VISIBLE|ES_LEFT|ES_AUTOHSCROLL|WS_TABSTOP,
		0, 0, 10, 10,
		a.barHwnd, 0, getModuleHandle(), 0,
	)
	if hwnd == 0 {
		logf("failed to create edit control")
		return
	}
	a.editHwnd = hwnd
	editHwnd := hwnd
	pSendMessageW.Call(editHwnd, EM_SETLIMITTEXT, 8000, 0)
	pSendMessageW.Call(editHwnd, WM_SETFONT, a.fontBody, 1)
	pSendMessageW.Call(editHwnd, EM_SETMARGINS, EC_LEFTMARGIN|EC_RIGHTMARGIN, makeLong(int32(10*a.scale), int32(10*a.scale)))
	pSendMessageW.Call(editHwnd, EM_SETCUEBANNER, 1, uintptr(unsafe.Pointer(u16("Paste a YouTube link…"))))
	r, _, _ := pSetWindowLongPtrW.Call(editHwnd, GWLP_WNDPROC, editProcCB)
	if r != 0 {
		oldEditProc = r
	}
}

func (a *App) layoutBar() {
	if a.barHwnd == 0 {
		return
	}
	var rc RECT
	pGetClientRect.Call(a.barHwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.Right, rc.Bottom
	s := a.scale

	a.editRect = RECT{Left: int32(24 * s), Top: int32(104 * s), Right: w - int32(136*s), Bottom: int32(152 * s)}
	a.btnRect = RECT{Left: w - int32(124*s), Top: int32(104 * s), Right: w - int32(24*s), Bottom: int32(152 * s)}
	a.closeRect = RECT{Left: w - int32(50*s), Top: int32(14 * s), Right: w - int32(18*s), Bottom: int32(46 * s)}
	a.browseRect = RECT{Left: int32(24 * s), Top: int32(190 * s), Right: int32(188 * s), Bottom: int32(228 * s)}
	a.controlsRect = RECT{Left: int32(200 * s), Top: int32(190 * s), Right: int32(348 * s), Bottom: int32(228 * s)}
	for i := range a.recentRect {
		y := float64(278 + i*54)
		a.recentRect[i] = RECT{Left: int32(24 * s), Top: int32(y * s), Right: w - int32(24*s), Bottom: int32((y + 48) * s)}
	}

	if a.editHwnd != 0 {
		mx := int32(12 * s)
		my := int32(13 * s)
		pMoveWindow.Call(a.editHwnd,
			uintptr(a.editRect.Left+mx), uintptr(a.editRect.Top+my),
			uintptr(rectW(a.editRect)-2*mx), uintptr(rectH(a.editRect)-2*my), 1)
	}

	r := int32(16 * s)
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(h+1), uintptr(r*2), uintptr(r*2))
	if rgn != 0 {
		pSetWindowRgn.Call(a.barHwnd, rgn, 1)
	}
}

func (a *App) invalidateBar() {
	if a.barHwnd != 0 {
		pInvalidateRect.Call(a.barHwnd, 0, 0)
	}
}

func (a *App) paintBar(hdc uintptr) {
	var rc RECT
	pGetClientRect.Call(a.barHwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.Right, rc.Bottom
	if w <= 0 || h <= 0 {
		return
	}
	if a.backDC == 0 || a.backW != w || a.backH != h {
		if a.backDC != 0 {
			pDeleteDC.Call(a.backDC)
		}
		if a.backBmp != 0 {
			pDeleteObject.Call(a.backBmp)
		}
		a.backDC, _, _ = pCreateCompatibleDC.Call(hdc)
		a.backBmp, _, _ = pCreateCompatibleBmp.Call(hdc, uintptr(w), uintptr(h))
		a.backW, a.backH = w, h
		pSelectObject.Call(a.backDC, a.backBmp)
	}
	mem := a.backDC
	if mem == 0 {
		logf("paint: no backDC")
		return
	}

	// Background.
	pFillRect.Call(mem, uintptr(unsafe.Pointer(&rc)), a.brBg)

	s := a.scale
	round := func(r RECT, pen, brush uintptr, rad float64) {
		pSelectObject.Call(mem, pen)
		pSelectObject.Call(mem, brush)
		pRoundRect.Call(mem, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom),
			uintptr(int32(rad*2)), uintptr(int32(rad*2)))
	}

	// Outer border.
	pSelectObject.Call(mem, pGetStockObject2(NULL_BRUSH))
	round(RECT{Left: 1, Top: 1, Right: w - 1, Bottom: h - 1}, a.penBorder, pGetStockObject2(NULL_BRUSH), 16*s)

	// A quiet header, one clear primary action and a separate browsing path.
	pSetBkMode.Call(mem, TRANSPARENT)
	pSelectObject.Call(mem, a.brAccent)
	pSelectObject.Call(mem, pGetStockObject2(NULL_PEN))
	pRoundRect.Call(mem, uintptr(int32(24*s)), uintptr(int32(25*s)), uintptr(int32(56*s)), uintptr(int32(57*s)), uintptr(int32(10*s)), uintptr(int32(10*s)))
	pSelectObject.Call(mem, a.brBg)
	pts := []POINT{{int32(36 * s), int32(33 * s)}, {int32(36 * s), int32(49 * s)}, {int32(47 * s), int32(41 * s)}}
	pPolygon.Call(mem, uintptr(unsafe.Pointer(&pts[0])), 3)
	pSelectObject.Call(mem, a.fontBrand)
	pSetTextColor.Call(mem, colText)
	drawText(mem, "YTFrame", RECT{Left: int32(68 * s), Top: int32(20 * s), Right: w - int32(60*s), Bottom: int32(58 * s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
	pSelectObject.Call(mem, a.fontBody)
	pSetTextColor.Call(mem, colMuted)
	drawText(mem, "Play a link. Keep your screen clear.", RECT{Left: int32(24 * s), Top: int32(66 * s), Right: w - int32(24*s), Bottom: int32(91 * s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)

	fieldPen := a.penField
	if focus, _, _ := pGetFocus.Call(); focus == a.editHwnd {
		fieldPen = a.penFocus
	}
	if a.statusErr {
		fieldPen = a.penError
	}
	round(a.editRect, fieldPen, a.brField, 10*s)
	br := a.brAccent
	if a.hoverBtn {
		br = a.brAccentHover
	}
	if a.launcherPress == 1 {
		br = a.brField
	}
	round(a.btnRect, pGetStockObject2(NULL_PEN), br, 10*s)
	pSelectObject.Call(mem, a.fontTitle)
	pSetTextColor.Call(mem, colDark)
	if a.launcherPress == 1 {
		pSetTextColor.Call(mem, colText)
	}
	label := "Play  →"
	if a.loading {
		label = "Cancel  ■"
	}
	drawText(mem, label, a.btnRect, DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)

	pSelectObject.Call(mem, a.fontClose)
	pSetTextColor.Call(mem, colMuted)
	if a.hoverClose {
		pSetTextColor.Call(mem, colText)
	}
	drawText(mem, "×", a.closeRect, DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)

	br = a.brBg
	if a.hoverBrowse || a.launcherPress == 2 {
		br = a.brField
	}
	round(a.browseRect, a.penBorder, br, 9*s)
	pSelectObject.Call(mem, a.fontTitle)
	pSetTextColor.Call(mem, colText)
	drawText(mem, "Browse YouTube", a.browseRect, DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
	br = a.brBg
	if a.launcherHover == 3 || a.launcherPress == 3 {
		br = a.brField
	}
	round(a.controlsRect, a.penBorder, br, 9*s)
	pSelectObject.Call(mem, a.fontTitle)
	pSetTextColor.Call(mem, colText)
	drawText(mem, "Show controls", a.controlsRect, DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
	pSelectObject.Call(mem, a.fontHint)
	pSetTextColor.Call(mem, colMuted)
	drawText(mem, "Ctrl+Alt+B", RECT{Left: int32(362 * s), Top: int32(190 * s), Right: w - int32(24*s), Bottom: int32(228 * s)}, DT_RIGHT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
	fillRect(mem, RECT{Left: int32(24 * s), Top: int32(248 * s), Right: w - int32(24*s), Bottom: int32(249 * s)}, a.brField)
	pSelectObject.Call(mem, a.fontHint)
	pSetTextColor.Call(mem, colMuted)
	drawText(mem, "RECENT VIDEOS", RECT{Left: int32(24 * s), Top: int32(252 * s), Right: w - int32(24*s), Bottom: int32(274 * s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
	if a.recentCount == 0 {
		drawText(mem, "Your recently played videos will appear here.", RECT{Left: int32(24 * s), Top: int32(286 * s), Right: w - int32(24*s), Bottom: int32(314 * s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)

	}
	for i := 0; i < a.recentCount; i++ {
		r := a.recentRect[i]
		br := a.brBg
		if a.launcherHover == 4+i || a.launcherPress == 4+i {
			br = a.brField
		}
		round(r, pGetStockObject2(NULL_PEN), br, 8*s)
		e := a.recent[i]
		title := e.Title
		if title == "" {
			title = e.URL
		}
		pSelectObject.Call(mem, a.fontTitle)
		pSetTextColor.Call(mem, colText)
		drawText(mem, title, RECT{Left: r.Left + int32(12*s), Top: r.Top + int32(3*s), Right: r.Right - int32(180*s), Bottom: r.Top + int32(25*s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)
		pSelectObject.Call(mem, a.fontHint)
		pSetTextColor.Call(mem, colMuted)
		label := "Play again"
		if e.Pos >= 10 && e.Pos < e.Dur-20 {
			label = "Continue at " + clockStr(e.Pos)
		}
		drawText(mem, label, RECT{Left: r.Left + int32(12*s), Top: r.Top + int32(25*s), Right: r.Right - int32(12*s), Bottom: r.Bottom - int32(2*s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
		pSetTextColor.Call(mem, colAccent)
		drawText(mem, "Play  →", RECT{Left: r.Right - int32(100*s), Top: r.Top, Right: r.Right - int32(12*s), Bottom: r.Bottom}, DT_RIGHT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
	}

	if a.status != "" {
		pSetTextColor.Call(mem, colMuted)
		if a.statusErr {
			pSetTextColor.Call(mem, colError)
		}
		drawText(mem, a.status, RECT{Left: int32(24 * s), Top: int32(156 * s), Right: w - int32(24*s), Bottom: int32(182 * s)}, DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)
	}
	if focus, _, _ := pGetFocus.Call(); focus == a.barHwnd && a.launcherFocus > 0 {
		r := a.launcherFocusRect(a.launcherFocus)
		r.Left -= int32(3 * s)
		r.Right += int32(3 * s)
		r.Top -= int32(3 * s)
		r.Bottom += int32(3 * s)
		round(r, a.penFocus, pGetStockObject2(NULL_BRUSH), 11*s)
	}

	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), mem, 0, 0, SRCCOPY)
}

func pGetStockObject2(kind int) uintptr {
	h, _, _ := pGetStockObject.Call(uintptr(kind))
	return h
}

func drawText(hdc uintptr, s string, r RECT, flags uint32) {
	u, _ := syscall.UTF16FromString(s)
	if len(u) <= 1 {
		return
	}
	pDrawTextW.Call(hdc,
		uintptr(unsafe.Pointer(&u[0])),
		uintptr(len(u)-1),
		uintptr(unsafe.Pointer(&r)),
		uintptr(flags))
}

// The drawn launcher actions participate in keyboard navigation, including Shift+Tab.
func (a *App) cycleLauncherFocus() {
	shift, _, _ := pGetKeyState.Call(VK_SHIFT)
	if focus, _, _ := pGetFocus.Call(); focus == a.editHwnd {
		a.launcherFocus = 0
	}
	ids := []int{0, 1, 2, 3}
	for i := 0; i < a.recentCount; i++ {
		ids = append(ids, 4+i)
	}
	ids = append(ids, 6)
	at := 0
	for i, id := range ids {
		if id == a.launcherFocus {
			at = i
			break
		}
	}
	step := 1
	if int16(shift) < 0 {
		step = -1
	}
	a.launcherFocus = ids[(at+step+len(ids))%len(ids)]
	if a.launcherFocus == 0 {
		pSetFocus.Call(a.editHwnd)
	} else {
		pSetFocus.Call(a.barHwnd)
	}
	a.invalidateBar()
}

func (a *App) launcherFocusRect(id int) RECT {
	switch id {
	case 1:
		return a.btnRect
	case 2:
		return a.browseRect
	case 3:
		return a.controlsRect
	case 4, 5:
		return a.recentRect[id-4]
	case 6:
		return a.closeRect
	}
	return RECT{}
}
func (a *App) launcherHit(x, y int32) int {
	for _, id := range []int{1, 2, 3, 6, 4, 5} {
		if id >= 4 && id <= 5 && id-4 >= a.recentCount {
			continue
		}
		if pointIn(a.launcherFocusRect(id), x, y) {
			return id
		}
	}
	return 0
}
func (a *App) launcherAction(id int) {
	switch id {
	case 2:
		a.hideBar(true)
		a.openBrowse()
	case 3:
		a.setControlBarVisible(true)
		a.showControlBar()
	case 4, 5:
		if id-4 < a.recentCount {
			a.startPlayback(a.recent[id-4].URL, true)
		}
	case 6:
		a.hideBar(true)
	default:
		a.submit()
	}
}
func (a *App) refreshRecent() {
	entries := recentEntries(a.history, 2)
	a.recentCount = len(entries)
	for i, e := range entries {
		a.recent[i] = e
	}
}

func (a *App) launcherHeight() float64 {
	switch a.recentCount {
	case 0:
		return 326
	case 1:
		return 348
	default:
		return launcherH
	}
}
