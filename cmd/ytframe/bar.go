//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

const (
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

var (
	oldEditProc uintptr
	editProcCB  = syscall.NewCallback(func(hwnd, msg, w, l uintptr) uintptr {
		switch msg {
		case WM_KEYDOWN:
			switch w {
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
		case WM_CHAR:
			if w == 13 || w == 27 {
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
)

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
	a.fontTitle = mkFont(13.5, FW_SEMIBOLD)
	a.fontBody = mkFont(13, FW_NORMAL)
	a.fontHint = mkFont(11, FW_NORMAL)
	a.fontClose = mkFont(19, FW_NORMAL)

	a.brBg, _, _ = pCreateSolidBrush.Call(colBg)
	a.brField, _, _ = pCreateSolidBrush.Call(colField)
	a.brAccent, _, _ = pCreateSolidBrush.Call(colAccent)
	a.brAccentHover, _, _ = pCreateSolidBrush.Call(colAccentHover)
	a.penBorder, _, _ = pCreatePen.Call(PS_SOLID, 1, colBorder)
	a.penField, _, _ = pCreatePen.Call(PS_SOLID, 1, colFieldBorder)
	a.penClose, _, _ = pCreatePen.Call(PS_SOLID, uintptr(int32(1.6*s)), colMuted)
	a.gdiReady = true
}

func (a *App) freeGDI() {
	for _, h := range []uintptr{a.fontTitle, a.fontBody, a.fontHint, a.fontClose,
		a.brBg, a.brField, a.brAccent, a.brAccentHover, a.penBorder, a.penField, a.penClose} {
		if h != 0 {
			pDeleteObject.Call(h)
		}
	}
	if a.backDC != 0 {
		pDeleteDC.Call(a.backDC)
	}
	if a.backBmp != 0 {
		pDeleteObject.Call(a.backBmp)
	}
	a.gdiReady = false
}

var barProcCB = syscall.NewCallback(barWndProc)

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
		x, y := loword(lparam), hiword(lparam)
		overBtn := pointIn(a.btnRect, x, y)
		overClose := pointIn(a.closeRect, x, y)
		if overBtn != a.hoverBtn || overClose != a.hoverClose {
			a.hoverBtn, a.hoverClose = overBtn, overClose
			a.invalidateBar()
		}
		var tme TRACKMOUSEEVENT
		tme.CbSize = uint32(unsafe.Sizeof(tme))
		tme.DwFlags = TME_LEAVE
		tme.HwndTrack = hwnd
		pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
		return 0
	case WM_MOUSELEAVE:
		a.hoverBtn, a.hoverClose = false, false
		a.invalidateBar()
		return 0
	case WM_SETCURSOR:
		var pt POINT
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		var rc RECT
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		x, y := pt.X-rc.Left, pt.Y-rc.Top
		if pointIn(a.btnRect, x, y) || pointIn(a.closeRect, x, y) {
			h, _, _ := pLoadCursorW.Call(0, IDC_HAND)
			pSetCursor.Call(h)
			return 1
		}
	case WM_LBUTTONDOWN:
		x, y := loword(lparam), hiword(lparam)
		if pointIn(a.btnRect, x, y) {
			a.pressBtn = true
			pSetCapture.Call(hwnd)
			a.invalidateBar()
			return 0
		}
		if pointIn(a.closeRect, x, y) {
			a.hideBar(true)
			return 0
		}
		pReleaseCapture.Call()
		pSendMessageW.Call(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0)
		return 0
	case WM_LBUTTONUP:
		x, y := loword(lparam), hiword(lparam)
		pReleaseCapture.Call()
		if a.pressBtn {
			a.pressBtn = false
			a.invalidateBar()
			if pointIn(a.btnRect, x, y) {
				a.submit()
			}
		}
		return 0
	case WM_KEYDOWN:
		if wparam == VK_ESCAPE {
			a.hideBar(true)
			return 0
		}
		if wparam == VK_RETURN {
			a.submit()
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
	pSendMessageW.Call(editHwnd, WM_SETFONT, a.fontBody, 1)
	pSendMessageW.Call(editHwnd, EM_SETMARGINS, EC_LEFTMARGIN|EC_RIGHTMARGIN, makeLong(int32(10*a.scale), int32(10*a.scale)))
	pSendMessageW.Call(editHwnd, EM_SETCUEBANNER, 1, uintptr(unsafe.Pointer(u16("https://www.youtube.com/watch?v=…"))))
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

	a.editRect = RECT{
		Left:   int32(24 * s),
		Top:    int32(44 * s),
		Right:  int32(float64(w) - 148*s),
		Bottom: int32(84 * s),
	}
	a.btnRect = RECT{
		Left:   int32(float64(w) - 128*s),
		Top:    int32(44 * s),
		Right:  int32(float64(w) - 24*s),
		Bottom: int32(84 * s),
	}
	a.closeRect = RECT{
		Left:   int32(float64(w) - 48*s),
		Top:    int32(10 * s),
		Right:  int32(float64(w) - 14*s),
		Bottom: int32(42 * s),
	}

	if a.editHwnd != 0 {
		mx := int32(12 * s)
		my := int32(9 * s)
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

	// Title.
	pSelectObject.Call(mem, a.fontTitle)
	pSetTextColor.Call(mem, colText)
	pSetBkMode.Call(mem, TRANSPARENT)
	drawText(mem, "Paste a YouTube link",
		RECT{Left: int32(24 * s), Top: int32(14 * s), Right: w - int32(60*s), Bottom: int32(38 * s)},
		DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)

	// Field behind the edit control.
	round(a.editRect, a.penField, a.brField, 10*s)

	// Button.
	br := a.brAccent
	if a.hoverBtn {
		br = a.brAccentHover
	}
	if a.pressBtn {
		br = a.penField
		br = a.brField
	}
	round(a.btnRect, pGetStockObject2(NULL_PEN), br, 10*s)
	pSelectObject.Call(mem, a.fontBody)
	pSetTextColor.Call(mem, colDark)
	drawText(mem, "Play", a.btnRect, DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)

	// Close glyph.
	pSelectObject.Call(mem, a.fontClose)
	if a.hoverClose {
		pSetTextColor.Call(mem, colText)
	} else {
		pSetTextColor.Call(mem, colMuted)
	}
	drawText(mem, "×", a.closeRect, DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)

	// Hint / status.
	pSelectObject.Call(mem, a.fontHint)
	if a.status != "" {
		if a.statusErr {
			pSetTextColor.Call(mem, colError)
		} else {
			pSetTextColor.Call(mem, colMuted)
		}
		drawText(mem, a.status,
			RECT{Left: int32(24 * s), Top: int32(96 * s), Right: w - int32(24*s), Bottom: int32(126 * s)},
			DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)
	} else {
		pSetTextColor.Call(mem, colMuted)
		drawText(mem, "Enter play  ·  Esc close  ·  Ctrl+Alt+V paste  ·  Ctrl+Alt+B controls",
			RECT{Left: int32(24 * s), Top: int32(96 * s), Right: w - int32(24*s), Bottom: int32(126 * s)},
			DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
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
