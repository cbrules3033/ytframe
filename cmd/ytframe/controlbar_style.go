//go:build windows

package main

import (
	"math"
	"time"
	"unsafe"
)

const (
	psGeometric       = 0x10000
	bsSolid           = 0
	gradientFillRectV = 0

	// COLORREF is 0x00BBGGRR. These are the remote's own palette, kept a
	// little cooler and softer than the launcher's.
	cbColBg            = 0x0016110F // #0f1116
	cbColBorder        = 0x003C312A // #2a313c
	cbColSurface       = 0x00241D19 // #191d24
	cbColSurfaceHover  = 0x00332822 // #222833
	cbColSurfacePress  = 0x003D312A // #2a313d
	cbColSurfaceBorder = 0x00453830 // #303845
	cbColDivider       = 0x002E2822 // #22282e
	cbColText          = 0x00F8F6F4 // #f4f6f8
	cbColMuted         = 0x00B2A59C // #9ca5b2
	cbColAccent        = 0x00FFA63E // #3ea6ff
	cbColAccentHover   = 0x00FFBC6B // #6bbcff
	cbColAccentPress   = 0x00E08F2D // #2d8fe0
	cbColTrack         = 0x00352C26 // #262c35
)

func cbMin(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

func cbInset(r RECT, d int32) RECT {
	return RECT{Left: r.Left + d, Top: r.Top + d, Right: r.Right - d, Bottom: r.Bottom - d}
}

func cbRound(dc uintptr, r RECT, radius int32, brush, pen uintptr) {
	if r.Right <= r.Left || r.Bottom <= r.Top {
		return
	}
	pSelectObject.Call(dc, pen)
	pSelectObject.Call(dc, brush)
	pRoundRect.Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom),
		uintptr(radius*2), uintptr(radius*2))
}

func cbLine(dc uintptr, x1, y1, x2, y2 int32, pen uintptr) {
	pSelectObject.Call(dc, pen)
	pMoveToEx.Call(dc, uintptr(x1), uintptr(y1), 0)
	pLineTo.Call(dc, uintptr(x2), uintptr(y2))
}

func cbPolyline(dc uintptr, pts []POINT, pen uintptr) {
	if len(pts) < 2 {
		return
	}
	pSelectObject.Call(dc, pen)
	pPolyline.Call(dc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
}

func cbFillCircle(dc uintptr, cx, cy, r int32, brush uintptr) {
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, brush)
	pEllipse.Call(dc, uintptr(cx-r), uintptr(cy-r), uintptr(cx+r), uintptr(cy+r))
}

func cbArcPoints(cx, cy, r float64, startDeg, endDeg float64) []POINT {
	sweep := endDeg - startDeg
	steps := int(math.Max(10, math.Abs(sweep)/14))
	pts := make([]POINT, 0, steps+1)
	for i := 0; i <= steps; i++ {
		a := (startDeg + sweep*float64(i)/float64(steps)) * math.Pi / 180
		pts = append(pts, POINT{
			X: int32(math.Round(cx + r*math.Cos(a))),
			Y: int32(math.Round(cy + r*math.Sin(a))),
		})
	}
	return pts
}

func cbDrawArc(dc uintptr, cx, cy, r float64, startDeg, endDeg float64, pen uintptr) {
	cbPolyline(dc, cbArcPoints(cx, cy, r, startDeg, endDeg), pen)
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
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	R := func(x, y, ww, hh float64) RECT {
		return RECT{Left: px(x), Top: px(y), Right: px(x + ww), Bottom: px(y + hh)}
	}

	// Clear first: an unavailable/failed gradient must never leave the previous
	// frame's text on the backbuffer.
	fillRect(dc, rc, a.cbBrBg)
	// A soft vertical gradient clipped by the rounded window region.
	if pGradientFill.Find() == nil {
		verts := [2]TRIVERTEX{
			{X: 0, Y: 0, Red: 0x1B00, Green: 0x2000, Blue: 0x2800, Alpha: 0xFFFF},
			{X: 0, Y: h, Red: 0x0F00, Green: 0x1100, Blue: 0x1600, Alpha: 0xFFFF},
		}
		gr := GRADIENT_RECT{UpperLeft: 0, LowerRight: 1}
		pGradientFill.Call(dc, uintptr(unsafe.Pointer(&verts[0])), 2,
			uintptr(unsafe.Pointer(&gr)), 1, gradientFillRectV)
	}
	cbRound(dc, RECT{Left: 0, Top: 0, Right: w, Bottom: h},
		px(20), pGetStockObject2(NULL_BRUSH), a.cbPenBorder)
	cbLine(dc, px(24), px(1), w-px(24), px(1), a.cbPenSurface)

	// A small brand tile makes the title row feel intentional.
	brand := R(18, 14, 22, 22)
	cbRound(dc, brand, px(7), a.cbBrAccent, pGetStockObject2(NULL_PEN))
	bcx, bcy := (brand.Left+brand.Right)/2, (brand.Top+brand.Bottom)/2
	play := []POINT{
		{X: bcx - px(3), Y: bcy - px(5)},
		{X: bcx - px(3), Y: bcy + px(5)},
		{X: bcx + px(5), Y: bcy},
	}
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, a.cbBrBg)
	pPolygon.Call(dc, uintptr(unsafe.Pointer(&play[0])), 3)

	pSetBkMode.Call(dc, TRANSPARENT)
	pSelectObject.Call(dc, a.cbFontBody)
	pSetTextColor.Call(dc, cbColText)
	title := a.cbTitle
	if a.loading {
		title = "Opening video…"
	} else if title == "" {
		title = "Ready when you are"
	}
	drawText(dc, title, R(50, 8, 300, 24),
		DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)

	pSelectObject.Call(dc, a.cbFontTime)
	pSetTextColor.Call(dc, cbColMuted)
	drawText(dc, a.cbHint(), R(50, 31, 300, 17),
		DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)

	a.cbDrawTopPill(dc, cbBtnOpen, a.cbBtn[cbBtnOpen], "Open", 0)
	a.cbDrawTopPill(dc, cbBtnBrowse, a.cbBtn[cbBtnBrowse], "Browse", 1)
	a.cbDrawIconButton(dc, cbBtnMenu, a.cbBtn[cbBtnMenu], func() {
		a.cbDrawMenuIcon(dc, a.cbBtn[cbBtnMenu])
	})
	a.cbDrawIconButton(dc, cbBtnHide, a.cbBtn[cbBtnHide], func() {
		a.cbDrawChevronDown(dc, a.cbBtn[cbBtnHide])
	})

	a.cbDrawTime(dc)
	a.cbDrawSeek(dc)
	a.cbDrawTransport(dc)

	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), dc, 0, 0, SRCCOPY)
}

func (a *App) cbDrawTopPill(dc uintptr, id int, r RECT, label string, kind int) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	br := a.cbBrPanel
	if a.cbHover == id {
		br = a.cbBrHover
	}
	if a.cbPress == id {
		br = a.cbBrPress
	}
	cbRound(dc, r, px(9), br, a.cbPenSurface)

	cx := r.Left + px(15)
	cy := (r.Top + r.Bottom) / 2
	switch kind {
	case 0:
		pen := a.cbPenIcon
		cbLine(dc, cx-px(4), cy, cx+px(4), cy, pen)
		cbLine(dc, cx, cy-px(4), cx, cy+px(4), pen)
	case 1:
		a.cbDrawBrowseGlyph(dc, cx, cy, px(7), a.cbPenIcon, a.cbBrIcon)
	}

	pSelectObject.Call(dc, a.cbFontLabel)
	pSetTextColor.Call(dc, cbColText)
	left := r.Left + px(27)
	if kind == 1 {
		left = r.Left + px(31)
	}
	drawText(dc, label, RECT{Left: left, Top: r.Top, Right: r.Right - px(8), Bottom: r.Bottom},
		DT_LEFT|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
}

func (a *App) cbDrawIconButton(dc uintptr, id int, r RECT, draw func()) {
	if a.cbHover == id || a.cbPress == id {
		br := a.cbBrHover
		if a.cbPress == id {
			br = a.cbBrPress
		}
		cbRound(dc, cbInset(r, int32(math.Round(2*a.cbScale))), int32(math.Round(9*a.cbScale)),
			br, pGetStockObject2(NULL_PEN))
	}
	draw()
}

func (a *App) cbDrawTime(dc uintptr) {
	pSelectObject.Call(dc, a.cbFontTime)
	statusActive := a.cbStatus != "" && time.Now().Before(a.cbStatusUntil)
	text := ""
	color := uintptr(cbColMuted)
	switch {
	case statusActive:
		text, color = a.cbStatus, cbColAccent
	case a.loading:
		text, color = "Opening…", cbColAccent
	case a.cbIdle:
		text, color = "Ctrl+Alt+V to open", cbColMuted
	case a.cbLastDur > 0:
		cur := a.cbLastPos
		if a.cbSeekDrag {
			cur = a.cbSeekTarget
		}
		text = clockStr(cur) + " / " + clockStr(a.cbLastDur)
		if a.cbSeekDrag {
			color = cbColText
		}
	default:
		text, color = "LIVE", cbColAccent
	}
	pSetTextColor.Call(dc, color)
	drawText(dc, text, a.cbTimeRect,
		DT_RIGHT|DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS|DT_NOPREFIX)
}

func (a *App) cbDrawSeek(dc uintptr) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	seek := a.cbSeekRect
	h := rectH(seek)
	radius := h / 2
	br := a.cbBrTrack
	if a.cbSeekHover || a.cbSeekDrag {
		br = a.cbBrHover
	}
	cbRound(dc, seek, radius, br, pGetStockObject2(NULL_PEN))

	if a.loading {
		span := px(88)
		if span > rectW(seek) {
			span = rectW(seek)
		}
		phase := float64(time.Now().UnixMilli()%1400) / 1400
		left := seek.Left + int32(phase*float64(rectW(seek)-span))
		cbRound(dc, RECT{Left: left, Top: seek.Top, Right: left + span, Bottom: seek.Bottom},
			radius, a.cbBrAccentHi, pGetStockObject2(NULL_PEN))
		return
	}
	if a.cbIdle || a.cbLastDur <= 0 {
		return
	}
	frac := a.cbLastPos / a.cbLastDur
	if a.cbSeekDrag {
		frac = a.cbSeekTarget / math.Max(1, a.cbLastDur)
	}
	frac = math.Max(0, math.Min(1, frac))
	fillW := int32(float64(rectW(seek)) * frac)
	if fillW > 0 {
		paintBr := a.cbBrAccent
		if a.cbSeekDrag {
			paintBr = a.cbBrAccentHi
		}
		cbRound(dc, RECT{Left: seek.Left, Top: seek.Top, Right: seek.Left + fillW, Bottom: seek.Bottom},
			radius, paintBr, pGetStockObject2(NULL_PEN))
	}
	kr := px(4.5)
	if a.cbSeekHover || a.cbSeekDrag {
		kr = px(5.5)
	}
	kx := seek.Left + fillW
	cy := (seek.Top + seek.Bottom) / 2
	cbFillCircle(dc, kx, cy, kr+px(1.2), a.cbBrBg)
	cbFillCircle(dc, kx, cy, kr, a.cbBrIcon)
}

func (a *App) cbDrawTransport(dc uintptr) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }

	a.cbDrawIconButton(dc, cbBtnBack, a.cbBtn[cbBtnBack], func() {
		pen, brush := a.cbIconStyle(cbBtnBack)
		a.cbDrawSkip(dc, a.cbBtn[cbBtnBack], -1, pen, brush)
	})
	a.cbDrawPlayButton(dc, a.cbBtn[cbBtnPlay])
	a.cbDrawIconButton(dc, cbBtnFwd, a.cbBtn[cbBtnFwd], func() {
		pen, brush := a.cbIconStyle(cbBtnFwd)
		a.cbDrawSkip(dc, a.cbBtn[cbBtnFwd], 1, pen, brush)
	})
	a.cbDrawIconButton(dc, cbBtnVol, a.cbBtn[cbBtnVol], func() {
		pen, brush := a.cbIconStyle(cbBtnVol)
		a.cbDrawVolume(dc, a.cbBtn[cbBtnVol], a.cbLastMute, pen, brush)
	})

	// A quiet divider separates transport from the display/layout controls.
	cbLine(dc, px(430), px(106), px(430), px(146), a.cbPenDivider)

	for _, id := range []int{cbBtnFullscreen, cbBtnEye, cbBtnLayouts, cbBtnPin} {
		id := id
		a.cbDrawIconButton(dc, id, a.cbBtn[id], func() {
			pen, brush := a.cbIconStyle(id)
			switch id {
			case cbBtnFullscreen:
				a.cbDrawFullscreen(dc, a.cbBtn[id], pen)
			case cbBtnEye:
				a.cbDrawEye(dc, a.cbBtn[id], pen, brush)
			case cbBtnLayouts:
				a.cbDrawLayouts(dc, a.cbBtn[id], pen)
			case cbBtnPin:
				a.cbDrawPin(dc, a.cbBtn[id], brush, a.settings.BarOntop)
			}
		})
	}
	a.cbDrawVolumeSlider(dc)
}

func (a *App) cbIconStyle(id int) (uintptr, uintptr) {
	if !a.cbEnabled(id) {
		return a.cbPenMuted, a.cbBrMuted
	}
	return a.cbPenIcon, a.cbBrIcon
}

func (a *App) cbDrawPlayButton(dc uintptr, r RECT) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	radius := cbMin(rectW(r), rectH(r)) / 2
	br := a.cbBrAccent
	if a.cbHover == cbBtnPlay {
		br = a.cbBrAccentHi
	}
	if a.cbPress == cbBtnPlay {
		br = a.cbBrAccentLo
	}
	cbFillCircle(dc, cx, cy, radius, br)
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, a.cbBrBg)
	if a.loading {
		d := px(5)
		pRoundRect.Call(dc, uintptr(cx-d), uintptr(cy-d), uintptr(cx+d), uintptr(cy+d),
			uintptr(px(2)), uintptr(px(2)))
		return
	}
	if a.cbLastPause {
		pts := []POINT{
			{X: cx - px(3), Y: cy - px(8)},
			{X: cx - px(3), Y: cy + px(8)},
			{X: cx + px(8), Y: cy},
		}
		pPolygon.Call(dc, uintptr(unsafe.Pointer(&pts[0])), 3)
		return
	}
	barW, gap, half := px(4), px(4), px(8)
	pRoundRect.Call(dc, uintptr(cx-gap/2-barW), uintptr(cy-half), uintptr(cx-gap/2), uintptr(cy+half),
		uintptr(px(1.5)), uintptr(px(1.5)))
	pRoundRect.Call(dc, uintptr(cx+gap/2), uintptr(cy-half), uintptr(cx+gap/2+barW), uintptr(cy+half),
		uintptr(px(1.5)), uintptr(px(1.5)))
}

func (a *App) cbDrawSkip(dc uintptr, r RECT, dir int, pen, brush uintptr) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	radius := float64(px(9))
	// A 280-degree arc leaves a gap on the right (back) or left (forward).
	pts := cbArcPoints(float64(cx), float64(cy), radius, 40, 320)
	if dir > 0 {
		for i := range pts {
			pts[i].X = 2*cx - pts[i].X
		}
	}
	cbPolyline(dc, pts, pen)

	// A broad arrowhead at the upper end makes the circular arrow read as a
	// rewind/forward control even at 100% scaling.
	arrow := []POINT{
		{X: cx + px(1), Y: cy - px(13)},
		{X: cx + px(10), Y: cy - px(10)},
		{X: cx + px(8), Y: cy - px(2)},
	}
	if dir > 0 {
		for i := range arrow {
			arrow[i].X = 2*cx - arrow[i].X
		}
	}
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, brush)
	pPolygon.Call(dc, uintptr(unsafe.Pointer(&arrow[0])), 3)

	pSelectObject.Call(dc, a.cbFontSmall)
	if pen == a.cbPenMuted {
		pSetTextColor.Call(dc, cbColMuted)
	} else {
		pSetTextColor.Call(dc, cbColText)
	}
	drawText(dc, "10", RECT{Left: cx - px(9), Top: cy - px(5), Right: cx + px(9), Bottom: cy + px(6)},
		DT_CENTER|DT_SINGLELINE|DT_VCENTER|DT_NOPREFIX)
}

func (a *App) cbDrawVolume(dc uintptr, r RECT, muted bool, pen, brush uintptr) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	pts := []POINT{
		{X: cx - px(8), Y: cy - px(3)},
		{X: cx - px(4), Y: cy - px(3)},
		{X: cx + px(1), Y: cy - px(8)},
		{X: cx + px(1), Y: cy + px(8)},
		{X: cx - px(4), Y: cy + px(3)},
		{X: cx - px(8), Y: cy + px(3)},
	}
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, brush)
	pPolygon.Call(dc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	if muted {
		cbLine(dc, cx+px(4), cy-px(6), cx+px(11), cy+px(6), pen)
		cbLine(dc, cx+px(11), cy-px(6), cx+px(4), cy+px(6), pen)
		return
	}
	cbDrawArc(dc, float64(cx+px(1)), float64(cy), float64(px(5)), -45, 45, pen)
	cbDrawArc(dc, float64(cx+px(1)), float64(cy), float64(px(8)), -45, 45, pen)
}

func (a *App) cbDrawFullscreen(dc uintptr, r RECT, pen uintptr) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	d := px(7)
	arm := px(5)
	cbLine(dc, cx-d, cy-d, cx-d+arm, cy-d, pen)
	cbLine(dc, cx-d, cy-d, cx-d, cy-d+arm, pen)
	cbLine(dc, cx+d, cy-d, cx+d-arm, cy-d, pen)
	cbLine(dc, cx+d, cy-d, cx+d, cy-d+arm, pen)
	cbLine(dc, cx-d, cy+d, cx-d+arm, cy+d, pen)
	cbLine(dc, cx-d, cy+d, cx-d, cy+d-arm, pen)
	cbLine(dc, cx+d, cy+d, cx+d-arm, cy+d, pen)
	cbLine(dc, cx+d, cy+d, cx+d, cy+d-arm, pen)
}

func (a *App) cbDrawEye(dc uintptr, r RECT, pen, brush uintptr) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	w := float64(px(10))
	h := float64(px(6))
	var top, bottom []POINT
	for i := 0; i <= 12; i++ {
		t := -1 + 2*float64(i)/12
		x := float64(cx) + t*w
		y := float64(cy) - h*(1-t*t)
		top = append(top, POINT{X: int32(math.Round(x)), Y: int32(math.Round(y))})
		bottom = append(bottom, POINT{X: int32(math.Round(x)), Y: int32(math.Round(2*float64(cy) - y))})
	}
	cbPolyline(dc, top, pen)
	cbPolyline(dc, bottom, pen)
	cbFillCircle(dc, cx, cy, px(2.4), brush)

	mode := a.settings.VideoMode
	if mode == "hidden" {
		cbLine(dc, cx-px(9), cy+px(7), cx+px(9), cy-px(7), a.cbPenAccent)
		return
	}
	if mode == "auto" {
		bx, by := cx+px(8), cy+px(7)
		cbFillCircle(dc, bx, by, px(4.4), a.cbBrAccent)
		pSelectObject.Call(dc, a.cbBrBg)
		pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
		bw := px(2)
		bh := px(2)
		pRoundRect.Call(dc, uintptr(bx-px(2)), uintptr(by-bh), uintptr(bx-px(2)+bw), uintptr(by+bh), uintptr(px(1)), uintptr(px(1)))
		pRoundRect.Call(dc, uintptr(bx+px(0)), uintptr(by-bh), uintptr(bx+px(0)+bw), uintptr(by+bh), uintptr(px(1)), uintptr(px(1)))
	}
}

func (a *App) cbDrawLayouts(dc uintptr, r RECT, pen uintptr) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	sz := px(7)
	gap := px(2)
	left := cx - sz - gap/2
	top := cy - sz - gap/2
	for yy := 0; yy < 2; yy++ {
		for xx := 0; xx < 2; xx++ {
			x := left + int32(xx)*(sz+gap)
			y := top + int32(yy)*(sz+gap)
			cbRound(dc, RECT{Left: x, Top: y, Right: x + sz, Bottom: y + sz},
				px(2), pGetStockObject2(NULL_BRUSH), pen)
		}
	}
}

func (a *App) cbDrawPin(dc uintptr, r RECT, brush uintptr, on bool) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	if on {
		brush = a.cbBrAccent
	}
	cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	// A map-pin silhouette: round head, tapered point, and a small hole.
	cbFillCircle(dc, cx, cy-px(4), px(4.4), brush)
	tip := []POINT{
		{X: cx - px(3.8), Y: cy - px(2)},
		{X: cx + px(3.8), Y: cy - px(2)},
		{X: cx, Y: cy + px(8)},
	}
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, brush)
	pPolygon.Call(dc, uintptr(unsafe.Pointer(&tip[0])), 3)
	cbFillCircle(dc, cx, cy-px(4), px(1.6), a.cbBrBg)
}

func (a *App) cbDrawBrowseGlyph(dc uintptr, cx, cy, half int32, pen, brush uintptr) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	r := RECT{Left: cx - half, Top: cy - px(5), Right: cx + half, Bottom: cy + px(5)}
	cbRound(dc, r, px(3), pGetStockObject2(NULL_BRUSH), pen)
	pts := []POINT{
		{X: cx - px(2), Y: cy - px(3)},
		{X: cx - px(2), Y: cy + px(3)},
		{X: cx + px(3), Y: cy},
	}
	pSelectObject.Call(dc, pGetStockObject2(NULL_PEN))
	pSelectObject.Call(dc, brush)
	pPolygon.Call(dc, uintptr(unsafe.Pointer(&pts[0])), 3)
}

func (a *App) cbDrawMenuIcon(dc uintptr, r RECT) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	for i := -1; i <= 1; i++ {
		cbFillCircle(dc, cx+int32(i)*px(5), cy, px(1.5), a.cbBrIcon)
	}
}

func (a *App) cbDrawChevronDown(dc uintptr, r RECT) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	cbLine(dc, cx-px(5), cy-px(2), cx, cy+px(3), a.cbPenMuted)
	cbLine(dc, cx, cy+px(3), cx+px(5), cy-px(2), a.cbPenMuted)
}

func (a *App) cbDrawVolumeSlider(dc uintptr) {
	s := a.cbScale
	px := func(v float64) int32 { return int32(math.Round(v * s)) }
	vr := a.cbVolRect
	cbRound(dc, vr, rectH(vr)/2, a.cbBrTrack, pGetStockObject2(NULL_PEN))
	if a.cbIdle || a.loading {
		return
	}
	vf := math.Max(0, math.Min(1, float64(a.cbLastVol)/130.0))
	if a.cbLastMute {
		vf = 0
	}
	vw := int32(float64(rectW(vr)) * vf)
	if vw > 0 {
		cbRound(dc, RECT{Left: vr.Left, Top: vr.Top, Right: vr.Left + vw, Bottom: vr.Bottom},
			rectH(vr)/2, a.cbBrAccent, pGetStockObject2(NULL_PEN))
	}
	kr := px(3.5)
	if a.cbVolHover {
		kr = px(4.5)
	}
	kx := vr.Left + vw
	cy := (vr.Top + vr.Bottom) / 2
	cbFillCircle(dc, kx, cy, kr+px(1.2), a.cbBrBg)
	cbFillCircle(dc, kx, cy, kr, a.cbBrIcon)
}
