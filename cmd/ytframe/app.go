//go:build windows

package main

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	app         *App
	helperHwnd  uintptr
	mutexHandle uintptr
)

type App struct {
	dir       string
	settings  Settings
	player    *Player
	playerSeq int

	barHwnd  uintptr
	editHwnd uintptr
	icon     uintptr

	trayAdded bool
	hotkeyOK  bool

	barVisible    bool
	pausedByBar   bool
	loading       bool
	loadingURL    string
	retries       int
	quitting      bool
	cleanupDone   bool
	eofShown      bool
	loadRestarted bool

	status    string
	statusErr bool

	scale float64

	gdiReady      bool
	fontTitle     uintptr
	fontBody      uintptr
	fontHint      uintptr
	fontClose     uintptr
	brBg          uintptr
	brField       uintptr
	brAccent      uintptr
	brAccentHover uintptr
	penBorder     uintptr
	penField      uintptr
	penClose      uintptr

	btnRect    RECT
	closeRect  RECT
	editRect   RECT
	hoverBtn   bool
	hoverClose bool
	pressBtn   bool

	backDC  uintptr
	backBmp uintptr
	backW   int32
	backH   int32

	updateResult string
	pollCount    int
}

func main() {
	runtime.LockOSThread()
	initDPI()

	dataDirPath = dataDir()
	initLog()
	defer closeLog()

	app = &App{dir: dataDirPath}
	app.settings = loadSettings()
	app.scale = float64(systemDPI()) / 96.0

	// Single instance.
	name := u16(`Local\YTFrameSingleInstance`)
	h, _, lastErr := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	mutexHandle = h
	if errno, ok := lastErr.(syscall.Errno); ok && errno == ERROR_ALREADY_EXISTS {
		target, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16("YTFrameHelper"))), 0)
		if u, ok := urlArg(); ok && target != 0 {
			sendCopyData(target, u)
		} else if target != 0 {
			pPostMessageW.Call(target, WM_APP_SHOWBAR, 0, 0)
		}
		return
	}

	app.icon = loadAppIcon()
	app.createHelperWindow()
	app.createTray()
	app.registerHotkey()

	if u, ok := urlArg(); ok {
		app.startPlayback(u, false)
	} else {
		app.showBarWith("", "", false)
	}

	app.runLoop()
	app.cleanup()
}

func urlArg() (string, bool) {
	for _, arg := range os.Args[1:] {
		if arg == "" {
			continue
		}
		if strings.HasPrefix(arg, "--") {
			if arg == "--ontop" {
				app.settings.Ontop = true
			}
			continue
		}
		return arg, true
	}
	return "", false
}

// ---- windows -------------------------------------------------------------

var helperProcCB = syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logf("helper panic: %v", r)
		}
	}()
	if app == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, msg, wparam, lparam)
		return r
	}
	a := app
	switch msg {
	case WM_APP_TRAY:
		a.onTray(lparam)
		return 0
	case WM_HOTKEY:
		if wparam == HK_PASTE {
			a.showBar()
		}
		return 0
	case WM_APP_SHOWBAR:
		a.showBar()
		return 0
	case WM_APP_LOADED:
		a.handleLoaded(int(wparam))
		return 0
	case WM_APP_ENDFILE:
		a.handleEndFile(int(wparam))
		return 0
	case WM_APP_MPVEXIT:
		a.handleMPVExit(int(wparam))
		return 0
	case WM_APP_WINDOWFOUND:
		a.placePlayerWindow(int(wparam))
		return 0
	case WM_APP_SNAP:
		pSetTimer.Call(helperHwnd, TIMER_SNAP, 150, 0)
		return 0
	case WM_APP_UPDATEDONE:
		msg := a.updateResult
		if len(msg) > 1400 {
			msg = msg[:1400] + "…"
		}
		messageBox(0, msg, "Update yt-dlp", MB_OK|MB_ICONINFORMATION)
		return 0
	case WM_TIMER:
		a.onTimer(wparam)
		return 0
	case WM_COPYDATA:
		a.onCopyData(lparam)
		return 1
	case WM_DESTROY:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
})

func (a *App) createHelperWindow() {
	hInst := getModuleHandle()
	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = helperProcCB
	wc.HInstance = hInst
	wc.HIcon = a.icon
	wc.LpszClassName = u16("YTFrameHelper")
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, _ := pCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(u16("YTFrameHelper"))),
		uintptr(unsafe.Pointer(u16("YTFrame"))),
		0, 0, 0, 0, 0, 0, 0, hInst, 0,
	)
	if hwnd == 0 {
		messageBox(0, "Could not create the YTFrame window.", "YTFrame", MB_OK|MB_ICONERROR)
		os.Exit(1)
	}
	helperHwnd = hwnd
	pSetTimer.Call(hwnd, TIMER_POLL, 2000, 0)
}

func (a *App) createTray() {
	var nid NOTIFYICONDATAW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = helperHwnd
	nid.UID = 1
	nid.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.UCallbackMessage = WM_APP_TRAY
	nid.HIcon = a.icon
	copy(nid.SzTip[:], syscall.StringToUTF16("YTFrame — video only"))
	r, _, _ := pShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
	a.trayAdded = r != 0
	if !a.trayAdded {
		logf("tray icon could not be added")
	}
}

func (a *App) removeTray() {
	if !a.trayAdded {
		return
	}
	var nid NOTIFYICONDATAW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = helperHwnd
	nid.UID = 1
	pShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
	a.trayAdded = false
}

func (a *App) registerHotkey() {
	r, _, _ := pRegisterHotKey.Call(helperHwnd, HK_PASTE, MOD_CONTROL|MOD_ALT, VK_V)
	a.hotkeyOK = r != 0
	if !a.hotkeyOK {
		logf("global hotkey Ctrl+Alt+V could not be registered")
	}
}

func (a *App) runLoop() {
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

// ---- tray ----------------------------------------------------------------

func (a *App) onTray(l uintptr) {
	switch l {
	case WM_RBUTTONUP, WM_CONTEXTMENU:
		a.showTrayMenu()
	case WM_LBUTTONDBLCLK, WM_LBUTTONUP:
		a.showBar()
	}
}

func (a *App) showTrayMenu() {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu)
	add := func(flags uintptr, id int, text string) {
		pAppendMenuW.Call(menu, flags, uintptr(id), uintptr(unsafe.Pointer(u16(text))))
	}
	sep := func() { pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0) }

	add(MF_STRING, 100, "Paste link…\tCtrl+Alt+V")
	if p := a.player; p != nil {
		st := p.snapshot()
		pauseLabel := "Pause"
		if st.Pause {
			pauseLabel = "Play"
		}
		muteLabel := "Mute"
		if st.Mute {
			muteLabel = "Unmute"
		}
		fsLabel := "Fullscreen"
		if st.Fullscreen {
			fsLabel = "Exit fullscreen"
		}
		add(MF_STRING, 101, pauseLabel)
		add(MF_STRING, 102, muteLabel)
		add(MF_STRING, 103, fsLabel)
		sep()
	}
	ontop := a.settings.Ontop
	if p := a.player; p != nil {
		ontop = p.snapshot().Ontop
	}
	flags := uintptr(MF_STRING)
	if ontop {
		flags |= MF_CHECKED
	}
	add(flags, 104, "Always on top")
	sep()
	add(MF_STRING, 105, "Update yt-dlp")
	add(MF_STRING, 106, "Open log")
	add(MF_STRING, 107, "About YTFrame")
	sep()
	add(MF_STRING, 108, "Exit")

	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(helperHwnd)
	cmd, _, _ := pTrackPopupMenu.Call(menu,
		TPM_RETURNCMD|TPM_RIGHTBUTTON, uintptr(pt.X), uintptr(pt.Y), 0, helperHwnd, 0)
	pPostMessageW.Call(helperHwnd, WM_NULL, 0, 0)
	if cmd != 0 {
		a.onMenuCommand(int(cmd))
	}
}

func (a *App) onMenuCommand(id int) {
	switch id {
	case 100:
		a.showBar()
	case 101:
		if p := a.player; p != nil {
			p.togglePause()
		}
	case 102:
		if p := a.player; p != nil {
			p.toggleMute()
		}
	case 103:
		if p := a.player; p != nil {
			p.toggleFullscreen()
		}
	case 104:
		if p := a.player; p != nil {
			p.setOntop(!p.snapshot().Ontop)
			a.settings.Ontop = p.snapshot().Ontop
		} else {
			a.settings.Ontop = !a.settings.Ontop
		}
		saveSettings(a.settings)
	case 105:
		a.updateYtdlp()
	case 106:
		a.openLog()
	case 107:
		a.showAbout()
	case 108:
		a.requestExit()
	}
}

// ---- bar -----------------------------------------------------------------

func (a *App) createBarWindow() {
	a.ensureGDI()
	hInst := getModuleHandle()
	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = barProcCB
	wc.HInstance = hInst
	wc.HIcon = a.icon
	wc.HCursor = loadCursor(IDC_ARROW)
	wc.LpszClassName = u16("YTFrameBar")
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	w := int32(640 * a.scale)
	h := int32(140 * a.scale)
	hwnd, _, _ := pCreateWindowExW.Call(
		WS_EX_TOPMOST|WS_EX_TOOLWINDOW,
		uintptr(unsafe.Pointer(u16("YTFrameBar"))),
		uintptr(unsafe.Pointer(u16("YTFrame"))),
		WS_POPUP|WS_CLIPCHILDREN,
		0, 0, uintptr(w), uintptr(h),
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		logf("failed to create paste bar")
		return
	}
	a.barHwnd = hwnd
	if a.icon != 0 {
		pSendMessageW.Call(hwnd, 0x0080 /* WM_SETICON, ICON_BIG */, 0, a.icon)
	}
	a.layoutBar()
}

func (a *App) showBar() {
	a.showBarWith("", "", false)
}

func (a *App) showBarWith(prefill, status string, statusErr bool) {
	if a.barHwnd == 0 {
		a.createBarWindow()
	}
	if a.barHwnd == 0 {
		return
	}
	if prefill == "" {
		prefill = a.pickPrefill()
	}
	if prefill != "" {
		setWindowText(a.editHwnd, prefill)
		pSendMessageW.Call(a.editHwnd, EM_SETSEL, 0, ^uintptr(0))
	}
	a.status = status
	a.statusErr = statusErr

	w := int32(640 * a.scale)
	h := int32(140 * a.scale)
	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	work := workAreaForPoint(pt)
	if a.player != nil {
		if hw := a.player.hwndNow(); hw != 0 {
			work = monitorWork(hw)
		}
	}
	x := work.Left + (rectW(work)-w)/2
	y := work.Top + (rectH(work)-h)/2
	pSetWindowPos.Call(a.barHwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		SWP_NOACTIVATE)
	a.layoutBar()
	pShowWindow.Call(a.barHwnd, SW_SHOW)
	pSetForegroundWindow.Call(a.barHwnd)
	pSetFocus.Call(a.editHwnd)
	a.barVisible = true
	a.invalidateBar()

	if p := a.player; p != nil {
		st := p.snapshot()
		if !st.Pause {
			p.pauseSet(true)
			a.pausedByBar = true
		}
	}
}

func (a *App) hideBar(resume bool) {
	if a.barHwnd == 0 {
		return
	}
	pShowWindow.Call(a.barHwnd, SW_HIDE)
	a.barVisible = false
	if resume && a.pausedByBar {
		if p := a.player; p != nil {
			p.pauseSet(false)
		}
	}
	a.pausedByBar = false
	if p := a.player; p != nil {
		if hw := p.hwndNow(); hw != 0 {
			pSetForegroundWindow.Call(hw)
		}
	}
}

func (a *App) setStatus(s string, isErr bool) {
	a.status = s
	a.statusErr = isErr
	a.invalidateBar()
}

func (a *App) pickPrefill() string {
	if cb := strings.TrimSpace(clipboardText()); isPlayableInput(cb) {
		if len(cb) > 8000 {
			cb = cb[:8000]
		}
		return cb
	}
	if p := a.player; p != nil {
		p.mu.Lock()
		u := p.url
		p.mu.Unlock()
		if u != "" {
			return u
		}
	}
	return a.settings.LastURL
}

func (a *App) submit() {
	if a.editHwnd == 0 {
		return
	}
	raw := getWindowText(a.editHwnd)
	u, start, errMsg := normalizeURL(raw)
	if errMsg != "" {
		a.setStatus(errMsg, true)
		return
	}
	if a.player != nil && a.player.isRunning() {
		if a.loading && !a.player.ready() {
			a.setStatus("Still starting the player…", false)
			return
		}
	}
	a.loading = true
	a.loadRestarted = false
	a.loadingURL = u
	a.retries = 0
	a.settings.LastURL = u
	saveSettings(a.settings)
	a.setStatus("Loading…", false)
	pSetTimer.Call(helperHwnd, TIMER_LOADWAIT, 30000, 0)

	if a.player == nil || !a.player.isRunning() {
		if err := a.startPlayer(u, start); err != nil {
			a.loading = false
			logf("start failed: %v", err)
			a.setStatus("Could not start mpv: "+err.Error(), true)
			messageBox(a.barHwnd, "Could not start the bundled mpv.exe.\n\n"+err.Error(),
				"YTFrame", MB_OK|MB_ICONERROR)
			return
		}
	} else {
		a.player.load(u, start)
	}
}

func (a *App) startPlayback(raw string, showBarIfError bool) {
	u, start, errMsg := normalizeURL(raw)
	if errMsg != "" {
		a.showBarWith(raw, errMsg, true)
		return
	}
	a.loading = true
	a.loadRestarted = false
	a.loadingURL = u
	a.retries = 0
	a.settings.LastURL = u
	saveSettings(a.settings)
	pSetTimer.Call(helperHwnd, TIMER_LOADWAIT, 30000, 0)
	if a.player == nil || !a.player.isRunning() {
		if err := a.startPlayer(u, start); err != nil {
			a.loading = false
			logf("start failed: %v", err)
			a.showBarWith(u, "Could not start mpv: "+err.Error(), true)
		}
	} else {
		a.player.load(u, start)
	}
}

func (a *App) startPlayer(u string, startSec int) error {
	p := newPlayer(a)
	if err := p.start(u, startSec); err != nil {
		return err
	}
	a.player = p
	return nil
}

func (a *App) handleLoaded(id int) {
	if a.player == nil || a.player.id != id {
		logf("ignoring stale load event id=%d", id)
		return
	}
	wasLoading := a.loading
	a.loading = false
	a.eofShown = false
	a.loadRestarted = false
	pKillTimer.Call(helperHwnd, TIMER_LOADWAIT)
	a.retries = 0
	if wasLoading && a.loadingURL != "" {
		a.settings.LastURL = a.loadingURL
	}
	if a.barVisible {
		a.hideBar(false)
	}
	pSetTimer.Call(helperHwnd, TIMER_SNAP, 250, 0)
}

func (a *App) handleEndFile(id int) {
	p := a.player
	if p == nil || p.id != id {
		return
	}
	p.mu.Lock()
	reason := p.endReason
	p.mu.Unlock()
	logf("end-file handling reason=%s", reason)
	switch reason {
	case "eof":
		if a.eofShown {
			return
		}
		a.eofShown = true
		a.loading = false
		pKillTimer.Call(helperHwnd, TIMER_LOADWAIT)
		hasDur := p.snapshot().HasDuration
		msg := "Finished. Paste another link or press Esc."
		if !hasDur {
			msg = "Stream ended. Paste another link or press Esc."
		}
		a.captureRect()
		a.showBarWith(a.settings.LastURL, msg, false)
	case "error":
		a.loading = false
		pKillTimer.Call(helperHwnd, TIMER_LOADWAIT)
		raw := p.lastErrorText()
		logf("playback error: %s", raw)
		msg, permanent := friendlyError(raw)
		if !permanent && a.retries < 2 {
			a.retries++
			a.setStatus(fmt.Sprintf("Reconnecting… (%d/2)", a.retries), false)
			pSetTimer.Call(helperHwnd, TIMER_RETRY, 3000, 0)
			return
		}
		a.showBarWith(a.settings.LastURL, msg, true)
	}
}

func (a *App) handleMPVExit(id int) {
	if a.quitting {
		return
	}
	if a.player == nil || a.player.id != id {
		logf("ignoring stale player exit id=%d", id)
		return
	}
	wasLoading := a.loading
	a.loading = false
	pKillTimer.Call(helperHwnd, TIMER_LOADWAIT)
	pKillTimer.Call(helperHwnd, TIMER_RETRY)
	pKillTimer.Call(helperHwnd, TIMER_SNAP)
	a.captureRect()
	a.player = nil
	if wasLoading || a.barVisible {
		a.showBarWith(a.settings.LastURL, "The player closed. Press Enter to try again.", true)
		return
	}
	logf("player exited; quitting")
	a.requestExit()
}

func (a *App) onTimer(id uintptr) {
	switch id {
	case TIMER_POLL:
		a.pollCount++
		a.captureRect()
		if a.pollCount%5 == 0 {
			saveSettings(a.settings)
		}
	case TIMER_SNAP:
		pKillTimer.Call(helperHwnd, TIMER_SNAP)
		a.snapAspect()
	case TIMER_RETRY:
		pKillTimer.Call(helperHwnd, TIMER_RETRY)
		if a.player != nil && a.loadingURL != "" {
			a.loading = true
			a.setStatus(fmt.Sprintf("Reconnecting… (%d/2)", a.retries), false)
			a.player.load(a.loadingURL, 0)
			pSetTimer.Call(helperHwnd, TIMER_LOADWAIT, 30000, 0)
		}
	case TIMER_LOADWAIT:
		pKillTimer.Call(helperHwnd, TIMER_LOADWAIT)
		if a.loading {
			if !a.loadRestarted && a.loadingURL != "" && a.player != nil {
				logf("load watchdog: restarting player")
				a.loadRestarted = true
				a.setStatus("Player is stuck — restarting…", true)
				old := a.player
				a.player = nil
				old.killAsync()
				a.loading = true
				if err := a.startPlayer(a.loadingURL, 0); err == nil {
					pSetTimer.Call(helperHwnd, TIMER_LOADWAIT, 30000, 0)
				} else {
					a.loading = false
					logf("restart failed: %v", err)
					a.setStatus("Could not restart mpv: "+err.Error(), true)
				}
				return
			}
			a.loading = false
			a.setStatus("Taking a while… check your connection, or try Update yt-dlp.", true)
		}
	}
}

func (a *App) captureRect() {
	p := a.player
	if p == nil {
		return
	}
	hwnd := p.hwndNow()
	if hwnd == 0 {
		return
	}
	st := p.snapshot()
	if st.Fullscreen {
		return
	}
	a.settings.Volume = st.Volume
	a.settings.Ontop = st.Ontop
	var rc RECT
	if ok, _, _ := pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc))); ok == 0 {
		return
	}
	w, h := rectW(rc), rectH(rc)
	if w < 80 || h < 60 {
		return
	}
	a.settings.X = rc.Left
	a.settings.Y = rc.Top
	a.settings.Width = w
	a.settings.Height = h
	a.settings.HasRect = true
}

func (a *App) placePlayerWindow(id int) {
	p := a.player
	if p == nil || p.id != id {
		return
	}
	hwnd := p.hwndNow()
	if hwnd == 0 {
		return
	}
	w, h := a.settings.Width, a.settings.Height
	if w < 100 {
		w = 960
	}
	if h < 60 {
		h = 540
	}
	var x, y int32
	if a.settings.HasRect {
		x, y = a.settings.X, a.settings.Y
	} else {
		var pt POINT
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		work := workAreaForPoint(pt)
		x = work.Left + (rectW(work)-w)/2
		y = work.Top + (rectH(work)-h)/2
	}
	work := workAreaAt(x+w/2, y+h/2)
	if w > rectW(work) {
		w = rectW(work)
	}
	if h > rectH(work) {
		h = rectH(work)
	}
	x = clamp(x, work.Left, work.Right-w)
	y = clamp(y, work.Top, work.Bottom-h)
	pSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		SWP_NOZORDER|SWP_NOACTIVATE)
	a.settings.X, a.settings.Y, a.settings.Width, a.settings.Height = x, y, w, h
	a.settings.HasRect = true
}

func (a *App) snapAspect() {
	p := a.player
	if p == nil {
		return
	}
	st := p.snapshot()
	hwnd := p.hwndNow()
	if hwnd == 0 || st.Fullscreen || st.Dwidth <= 0 || st.Dheight <= 0 {
		return
	}
	var rc RECT
	if ok, _, _ := pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc))); ok == 0 {
		return
	}
	cw, ch := rectW(rc), rectH(rc)
	if cw < 100 || ch < 60 {
		return
	}
	vw, vh := float64(st.Dwidth), float64(st.Dheight)
	targetH := int32(math.Round(float64(cw) * vh / vw))
	if targetH < 60 {
		return
	}
	work := monitorWork(hwnd)
	maxW, maxH := rectW(work), rectH(work)
	if targetH > maxH {
		targetH = maxH
		cw = int32(math.Round(float64(targetH) * vw / vh))
	}
	if cw > maxW {
		cw = maxW
		targetH = int32(math.Round(float64(cw) * vh / vw))
	}
	x, y := rc.Left, rc.Top
	if x+cw > work.Right {
		x = work.Right - cw
	}
	if y+targetH > work.Bottom {
		y = work.Bottom - targetH
	}
	if x < work.Left {
		x = work.Left
	}
	if y < work.Top {
		y = work.Top
	}
	if targetH == ch && x == rc.Left && y == rc.Top {
		return
	}
	pSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(cw), uintptr(targetH),
		SWP_NOZORDER|SWP_NOACTIVATE)
	a.settings.Width, a.settings.Height = cw, targetH
}

func (a *App) onCopyData(lparam uintptr) {
	cds := (*COPYDATASTRUCT)(unsafe.Pointer(lparam))
	if cds == nil || cds.CbData == 0 || cds.LpData == 0 {
		return
	}
	b := unsafe.Slice((*byte)(unsafe.Pointer(cds.LpData)), cds.CbData)
	s := strings.TrimRight(string(b), "\x00")
	if s != "" {
		a.startPlayback(s, true)
	}
}

// ---- misc ----------------------------------------------------------------

func (a *App) updateYtdlp() {
	path := filepath.Join(dataDirPath, "yt-dlp.exe")
	if _, err := os.Stat(path); err != nil {
		messageBox(0, "yt-dlp.exe is missing from the YTFrame folder.", "Update yt-dlp", MB_OK|MB_ICONERROR)
		return
	}
	go func() {
		cmd := exec.Command(path, "-U")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: CREATE_NO_WINDOW}
		out, err := cmd.CombinedOutput()
		msg := strings.TrimSpace(string(out))
		if err != nil {
			msg = "Update failed: " + err.Error() + "\n\n" + msg
		}
		if msg == "" {
			msg = "yt-dlp is already up to date."
		}
		app.updateResult = msg
		postMsg(WM_APP_UPDATEDONE, 0, 0)
	}()
}

func (a *App) openLog() {
	p := filepath.Join(dataDirPath, "ytframe.log")
	if _, err := os.Stat(p); err != nil {
		messageBox(0, "No log file yet.", "YTFrame", MB_OK)
		return
	}
	r, _, _ := pShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(u16("open"))),
		uintptr(unsafe.Pointer(u16(p))),
		0, 0, 1)
	if r <= 32 {
		messageBox(0, "Log file:\n"+p, "YTFrame", MB_OK)
	}
}

func (a *App) showAbout() {
	msg := "YTFrame " + appVersion + "\n\n" +
		"A borderless, video-only window for YouTube links.\n\n" +
		"Enter — play the link\n" +
		"Ctrl+Alt+V — paste another link\n" +
		"Drag edges/corners — resize (keeps the video's aspect)\n" +
		"Double-click — fullscreen\n" +
		"Space — pause\n" +
		"Right-click the video — player menu\n\n" +
		"mpv and yt-dlp are bundled. If a video suddenly stops working,\n" +
		"use “Update yt-dlp” in the tray menu."
	messageBox(0, msg, "About YTFrame", MB_OK|MB_ICONINFORMATION)
}

func (a *App) requestExit() {
	if a.quitting {
		return
	}
	a.quitting = true
	a.captureRect()
	saveSettings(a.settings)
	if a.player != nil {
		a.player.quitAndWait(2 * time.Second)
		a.player = nil
	}
	a.removeTray()
	pUnregisterHotKey.Call(helperHwnd, HK_PASTE)
	if a.barHwnd != 0 {
		pDestroyWindow.Call(a.barHwnd)
		a.barHwnd = 0
	}
	pPostQuitMessage.Call(0)
}

func (a *App) cleanup() {
	if a.cleanupDone {
		return
	}
	a.cleanupDone = true
	a.captureRect()
	saveSettings(a.settings)
	if a.player != nil {
		a.player.quitAndWait(2 * time.Second)
		a.player = nil
	}
	a.removeTray()
	a.freeGDI()
}

// ---- URL helpers ---------------------------------------------------------

func isPlayableInput(s string) bool {
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") || strings.HasPrefix(low, "file://") {
		return true
	}
	return isDrivePath(s)
}

func isDrivePath(s string) bool {
	if len(s) < 3 {
		return false
	}
	c := s[0]
	if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
		return false
	}
	return s[1] == ':' && (s[2] == '\\' || s[2] == '/')
}

func normalizeURL(raw string) (string, int, string) {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "\"'")
	if s == "" {
		return "", 0, "Paste a link first."
	}
	low := strings.ToLower(s)
	if !strings.Contains(low, "://") {
		for _, prefix := range []string{"youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com", "youtu.be"} {
			if strings.HasPrefix(low, prefix) {
				s = "https://" + s
				low = strings.ToLower(s)
				break
			}
		}
	}
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return s, startParam(s), ""
	}
	if strings.HasPrefix(low, "file://") {
		return s, 0, ""
	}
	if isDrivePath(s) {
		return s, 0, ""
	}
	return "", 0, "That doesn't look like a link."
}

func startParam(raw string) int {
	u, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	v := u.Query().Get("t")
	if v == "" {
		v = u.Query().Get("start")
	}
	return parseTime(v)
}

func parseTime(s string) int {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 {
			return 0
		}
		if n > 86400 {
			return 86400
		}
		return n
	}
	total := 0
	num := 0
	seen := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			num = num*10 + int(r-'0')
			seen = true
		case r == 'h':
			total += num * 3600
			num = 0
		case r == 'm':
			total += num * 60
			num = 0
		case r == 's':
			total += num
			num = 0
		default:
			return 0
		}
	}
	if seen {
		total += num
	}
	if total > 86400 {
		total = 86400
	}
	return total
}

func friendlyError(raw string) (string, bool) {
	low := strings.ToLower(raw)
	switch {
	case strings.Contains(low, "will begin"), strings.Contains(low, "premieres"),
		strings.Contains(low, "not started"), strings.Contains(low, "hasn't started"),
		strings.Contains(low, "has not started"):
		return "This stream hasn't started yet. Try again when it's live.", true
	case strings.Contains(low, "private video"):
		return "That video is private.", true
	case strings.Contains(low, "members-only"), strings.Contains(low, "join this channel"):
		return "That video is members-only.", true
	case strings.Contains(low, "sign in"), strings.Contains(low, "confirm your age"),
		strings.Contains(low, "age-restricted"), strings.Contains(low, "inappropriate for some users"):
		return "YouTube wants a sign-in for that video. Age-restricted videos are not supported.", true
	case strings.Contains(low, "unsupported url"):
		return "That link isn't supported.", true
	case strings.Contains(low, "video unavailable"), strings.Contains(low, "removed by the uploader"),
		strings.Contains(low, "no longer available"), strings.Contains(low, "does not exist"):
		return "That video isn't available.", true
	case strings.Contains(low, "not available in your country"), strings.Contains(low, "blocked it in your country"),
		strings.Contains(low, "geo restrict"):
		return "That video is blocked in your region.", true
	case strings.Contains(low, "http error 403"), strings.Contains(low, "failed to extract"),
		strings.Contains(low, "unable to extract"), strings.Contains(low, "nsig"):
		return "YouTube refused the stream. Try “Update yt-dlp” from the tray menu.", true
	case strings.Contains(low, "timed out"), strings.Contains(low, "connection"),
		strings.Contains(low, "network"), strings.Contains(low, "temporary failure"),
		strings.Contains(low, "http error 5"), strings.Contains(low, "getaddrinfo"):
		return "Network problem — check your connection.", false
	}
	return "Couldn't play that link. Try “Update yt-dlp” from the tray menu.", false
}

func loadAppIcon() uintptr {
	h, _, _ := pLoadImageW.Call(getModuleHandle(), 1, IMAGE_ICON, 0, 0, LR_DEFAULTSIZE|LR_SHARED)
	if h != 0 {
		return h
	}
	h, _, _ = pLoadIconW.Call(0, IDI_APPLICATION)
	return h
}

func loadCursor(id uintptr) uintptr {
	h, _, _ := pLoadCursorW.Call(0, id)
	return h
}

func workAreaAt(px, py int32) RECT {
	pt := POINT{X: px, Y: py}
	mon, _, _ := pMonitorFromPoint.Call(uintptr(unsafe.Pointer(&pt)), MONITOR_DEFAULTTONEAREST)
	var mi MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	if mon != 0 {
		pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
		return mi.RcWork
	}
	return RECT{Right: 1280, Bottom: 720}
}
