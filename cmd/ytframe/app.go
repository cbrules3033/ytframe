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
	"sync"
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

	history *History
	sponsor *sponsorClient

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

	historyDirty      bool
	segments          []sponsorSegment
	segmentsKey       string
	lastSkipEnd       float64
	pendingPause      bool
	pendingFullscreen bool
	loadingStart      int
	loadedAt          time.Time
	loadRequested     time.Time
	lastStartKey      string
	lastStartAt       time.Time
	playRequestedAt   time.Time
	switchRect        RECT
	switchRectValid   bool
	lastTrayTitle     string

	status    string
	statusErr bool

	scale float64

	gdiReady      bool
	fontTitle     uintptr
	fontBrand     uintptr
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
	penFocus      uintptr
	penError      uintptr

	btnRect       RECT
	closeRect     RECT
	editRect      RECT
	browseRect    RECT
	hoverBrowse   bool
	pressBrowse   bool
	launcherFocus int
	launcherHover int
	launcherPress int
	controlsRect  RECT
	recentRect    [2]RECT
	recent        [2]HistoryEntry
	recentCount   int
	hoverBtn      bool
	hoverClose    bool
	pressBtn      bool

	backDC  uintptr
	backBmp uintptr
	backW   int32
	backH   int32

	updateResult string
	updateMu     sync.Mutex
	updating     bool
	pollCount    int

	cbHwnd       uintptr
	cbVisible    bool
	cbReady      bool
	cbBaseScale  float64
	cbScale      float64
	cbSuper      int
	cbDrawScale  float64
	cbGripRect   RECT
	cbResizeDrag bool
	cbResizeFrom POINT
	cbResizeBase float64

	cbFontBody   uintptr
	cbFontLabel  uintptr
	cbFontSmall  uintptr
	cbFontTime   uintptr
	cbBrBg       uintptr
	cbBrPanel    uintptr
	cbBrHover    uintptr
	cbBrPress    uintptr
	cbBrAccent   uintptr
	cbBrAccentHi uintptr
	cbBrAccentLo uintptr
	cbBrIcon     uintptr
	cbBrMuted    uintptr
	cbBrTrack    uintptr
	cbPenBorder  uintptr
	cbPenSurface uintptr
	cbPenDivider uintptr
	cbPenIcon    uintptr
	cbPenMuted   uintptr
	cbPenAccent  uintptr

	cbBackDC    uintptr
	cbBackBmp   uintptr
	cbBackBits  unsafe.Pointer
	cbFinalDC   uintptr
	cbFinalBmp  uintptr
	cbFinalBits unsafe.Pointer
	cbBackW     int32
	cbBackH     int32

	cbSeekRect      RECT
	cbTimeRect      RECT
	cbVolRect       RECT
	cbBtn           [cbBtnCount]RECT
	cbHover         int
	cbPress         int
	cbSeekHover     bool
	cbVolHover      bool
	cbDrag          bool
	cbDragStart     POINT
	cbDragRect      RECT
	cbSeekDrag      bool
	cbSeekTarget    float64
	cbVolDrag       bool
	cbStatus        string
	cbStatusUntil   time.Time
	cbLastPause     bool
	cbTitle         string
	cbBuffering     bool
	cbBufferPercent int
	cbIdle          bool
	cbLastLoading   bool
	cbLastTick      int
	cbPaintCount    uint64
	cbInvalidations uint64
	cbLastPos       float64
	cbLastDur       float64
	cbLastVol       int
	cbLastMute      bool
	videoHidden     bool
	pendingLayout   int

	browseOpen bool
	browseCmd  *exec.Cmd
	browsePid  int
}

func main() {
	runtime.LockOSThread()
	initDPI()

	if hasArg("--webview-version") {
		fmt.Println(webviewVersion())
		return
	}

	dataDirPath = dataDir()
	initLog()
	defer closeLog()

	if hasArg("--browse") {
		runBrowse()
		return
	}

	app = &App{dir: dataDirPath, pendingLayout: -1}
	app.settings = loadSettings()
	app.history = loadHistory()
	app.sponsor = newSponsorClient()
	app.scale = float64(systemDPI()) / 96.0
	if d := app.settings.DefaultLayout; d >= 0 && d < layoutSlots && app.settings.Layouts[d].Used {
		l := app.settings.Layouts[d]
		app.settings.X, app.settings.Y = l.X, l.Y
		app.settings.Width, app.settings.Height = l.Width, l.Height
		app.settings.HasRect = true
		if l.BarSet {
			app.settings.BarX, app.settings.BarY = l.BarX, l.BarY
			app.settings.BarHasPos = true
		}
		logf("default layout %d applied", d+1)
	}

	// Single instance: an existing helper window is the most reliable test.
	if target, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(u16("YTFrameHelper"))), 0); target != 0 {
		if u, ok := urlArg(); ok {
			sendCopyData(target, u)
		} else {
			pPostMessageW.Call(target, WM_APP_SHOWBAR, 0, 0)
		}
		logf("second instance handed off to existing window")
		return
	}

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
		pSetTimer.Call(helperHwnd, TIMER_PREWARM, 1, 0)
	}
	if app.settings.BarVisible {
		app.showControlBar()
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
		switch {
		case wparam == HK_PASTE:
			a.showBar()
		case wparam == HK_PLAY:
			if p := a.player; p != nil {
				p.togglePause()
			}
		case wparam == HK_BACK:
			if p := a.player; p != nil {
				p.seekRelative(-10)
			}
		case wparam == HK_FWD:
			if p := a.player; p != nil {
				p.seekRelative(10)
			}
		case wparam == HK_MUTE:
			if p := a.player; p != nil {
				p.toggleMute()
			}
		case wparam == HK_BAR:
			a.toggleControlBar()
		case wparam >= HK_LAYOUT && wparam < HK_LAYOUT+layoutSlots:
			a.applyLayout(int(wparam - HK_LAYOUT))
		case wparam >= HK_LAYOUT_SAVE && wparam < HK_LAYOUT_SAVE+layoutSlots:
			a.saveLayout(int(wparam - HK_LAYOUT_SAVE))
		}
		return 0
	case WM_APP_PASTE:
		if a.player != nil && a.player.id == int(wparam) {
			a.startPlayback(clipboardText(), true)
		}
		return 0
	case WM_APP_MPVREADY:
		if p := a.player; p != nil && p.id == int(wparam) {
			if a.loading && p.urlNow() != a.loadingURL {
				p.load(a.loadingURL, a.loadingStart)
			} else if !a.loading && p.urlNow() == "" {
				p.stop()
			}
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
		a.applyVideoVisibility()
		return 0
	case WM_APP_SNAP:
		pSetTimer.Call(helperHwnd, TIMER_SNAP, 150, 0)
		return 0
	case WM_APP_BROWSEEXIT:
		a.onBrowseExit()
		return 0
	case WM_APP_UPDATEDONE:
		a.updateMu.Lock()
		msg := a.updateResult
		a.updateMu.Unlock()
		a.updating = false
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
	pSetTimer.Call(hwnd, TIMER_SPONSOR, 500, 0)
	pSetTimer.Call(hwnd, TIMER_BAR, 100, 0)
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
	type hk struct {
		id  uintptr
		vk  uintptr
		mod uintptr
		key string
	}
	keys := []hk{
		{HK_PASTE, VK_V, MOD_CONTROL | MOD_ALT, "V"},
		{HK_PLAY, VK_SPACE, MOD_CONTROL | MOD_ALT, "Space"},
		{HK_BACK, VK_J, MOD_CONTROL | MOD_ALT, "J"},
		{HK_FWD, VK_L, MOD_CONTROL | MOD_ALT, "L"},
		{HK_MUTE, VK_M, MOD_CONTROL | MOD_ALT, "M"},
		{HK_BAR, VK_B, MOD_CONTROL | MOD_ALT, "B"},
	}
	for i := 0; i < layoutSlots; i++ {
		keys = append(keys,
			hk{HK_LAYOUT + uintptr(i), VK_1 + uintptr(i), MOD_CONTROL | MOD_ALT, fmt.Sprintf("%d", i+1)},
			hk{HK_LAYOUT_SAVE + uintptr(i), VK_1 + uintptr(i), MOD_CONTROL | MOD_ALT | MOD_SHIFT, "Shift+" + fmt.Sprintf("%d", i+1)},
		)
	}
	for _, k := range keys {
		r, _, _ := pRegisterHotKey.Call(helperHwnd, k.id, k.mod, k.vk)
		if r == 0 {
			logf("global hotkey Ctrl+Alt+%s could not be registered", k.key)
		}
	}
	a.hotkeyOK = true
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

	hasPlayer := a.player != nil && a.player.isRunning() && a.player.urlNow() != ""
	st := PlayerState{}
	if hasPlayer {
		st = a.player.snapshot()
	}
	add := func(flags uintptr, id int, text string) {
		pAppendMenuW.Call(menu, flags, uintptr(id), uintptr(unsafe.Pointer(u16(text))))
	}
	sep := func() { pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0) }
	base := uintptr(MF_STRING)
	if !hasPlayer {
		base |= MF_GRAYED
	}
	check := func(on bool) uintptr {
		f := uintptr(MF_STRING)
		if on {
			f |= MF_CHECKED
		}
		return f
	}

	add(MF_STRING, 100, "Paste link…\tCtrl+Alt+V")
	add(MF_STRING, 140, "Browse YouTube…")
	add(MF_STRING, 141, "Clear YouTube browsing data")
	sep()
	pauseLabel := "Play/Pause"
	if hasPlayer && st.Pause {
		pauseLabel = "Play"
	} else if hasPlayer {
		pauseLabel = "Pause"
	}
	add(base, 101, pauseLabel+"\tCtrl+Alt+Space")
	add(base, 102, "Back 10s\tCtrl+Alt+J")
	add(base, 103, "Forward 10s\tCtrl+Alt+L")
	muteLabel := "Mute"
	if hasPlayer && st.Mute {
		muteLabel = "Unmute"
	}
	add(base, 113, muteLabel+"\tCtrl+Alt+M")
	sep()

	if qmenu, _, _ := pCreatePopupMenu.Call(); qmenu != 0 {
		qadd := func(id int, text string) {
			pAppendMenuW.Call(qmenu, MF_STRING, uintptr(id), uintptr(unsafe.Pointer(u16(text))))
		}
		qadd(200, "Best")
		qadd(201, "1080p")
		qadd(202, "720p")
		qadd(203, "480p")
		cur := 200
		switch a.settings.Quality {
		case "1080":
			cur = 201
		case "720":
			cur = 202
		case "480":
			cur = 203
		}
		pCheckMenuRadioItem.Call(qmenu, 200, 203, uintptr(cur), MF_BYCOMMAND)
		pAppendMenuW.Call(menu, MF_POPUP|MF_STRING, qmenu, uintptr(unsafe.Pointer(u16("Quality"))))
	}

	if vmenu, _, _ := pCreatePopupMenu.Call(); vmenu != 0 {
		vadd := func(id int, text, mode string) {
			flags := uintptr(MF_STRING)
			if a.settings.VideoMode == mode {
				flags |= MF_CHECKED
			}
			pAppendMenuW.Call(vmenu, flags, uintptr(id), uintptr(unsafe.Pointer(u16(text))))
		}
		vadd(120, "Show always", "visible")
		vadd(121, "Hide on pause", "auto")
		vadd(122, "Always hidden (audio only)", "hidden")
		pAppendMenuW.Call(menu, MF_POPUP|MF_STRING, vmenu, uintptr(unsafe.Pointer(u16("Video"))))
	}

	add(check(a.settings.BarVisible), 112, "Control bar")
	add(MF_STRING, 143, "Bring control bar forward")
	add(check(a.settings.Controls), 115, "In-video controls")

	if lmenu, _, _ := pCreatePopupMenu.Call(); lmenu != 0 {
		a.buildLayoutsMenu(lmenu)
		pAppendMenuW.Call(menu, MF_POPUP|MF_STRING, lmenu, uintptr(unsafe.Pointer(u16("Layouts"))))
	}
	sep()

	if wmenu, _, _ := pCreatePopupMenu.Call(); wmenu != 0 {
		a.buildWindowBehaviorMenu(wmenu)
		pAppendMenuW.Call(menu, MF_POPUP|MF_STRING, wmenu, uintptr(unsafe.Pointer(u16("Window behavior"))))
	}
	fsLabel := "Fullscreen"
	if hasPlayer && st.Fullscreen {
		fsLabel = "Exit fullscreen"
	}
	add(base, 105, fsLabel)
	sep()

	add(check(a.settings.Resume), 110, "Resume playback")
	add(check(a.settings.SponsorBlock), 111, "SponsorBlock")
	sep()
	add(MF_STRING, 106, "Update yt-dlp")
	add(MF_STRING, 107, "Open log")
	add(MF_STRING, 114, "Controls…")
	add(MF_STRING, 108, "About YTFrame")
	sep()
	add(MF_STRING, 109, "Exit")

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
	p := a.player
	switch {
	case id == 100:
		a.showBar()
	case id == 101:
		if p != nil {
			p.togglePause()
		}
	case id == 102:
		if p != nil {
			p.seekRelative(-10)
		}
	case id == 103:
		if p != nil {
			p.seekRelative(10)
		}
	case id == 113:
		if p != nil {
			p.toggleMute()
		}
	case id == 104:
		if p != nil {
			cur := p.snapshot().Ontop
			p.setOntop(!cur)
			a.settings.Ontop = !cur
		} else {
			a.settings.Ontop = !a.settings.Ontop
		}
		saveSettings(a.settings)
	case id == 105:
		if p != nil {
			p.toggleFullscreen()
		}
	case id == 106:
		a.updateYtdlp()
	case id == 107:
		a.openLog()
	case id == 108:
		a.showAbout()
	case id == 109:
		a.requestExit()
	case id == 110:
		a.settings.Resume = !a.settings.Resume
		saveSettings(a.settings)
	case id == 111:
		a.settings.SponsorBlock = !a.settings.SponsorBlock
		a.segmentsKey = ""
		a.segments = nil
		a.lastSkipEnd = 0
		saveSettings(a.settings)
	case id == 112:
		a.setControlBarVisible(!a.settings.BarVisible)
	case id == 114:
		a.showControlsHelp()
	case id == 115:
		a.setControls(!a.settings.Controls)
	case id == 116:
		a.setBarOntop(!a.settings.BarOntop)
	case id == 120:
		a.setVideoMode("visible")
	case id == 121:
		a.setVideoMode("auto")
	case id == 122:
		a.setVideoMode("hidden")
	case id == 140:
		a.openBrowse()
	case id == 141:
		a.clearBrowseData()
	case id == 142:
		a.closeBrowse()
	case id == 143:
		a.bringControlBarForward()
	case id >= 150 && id <= 153:
		a.setBarScale([]float64{0.8, 1, 1.2, 1.4}[id-150], true)
	case id >= 160 && id <= 165:
		a.setBarOpacity([]int{100, 90, 80, 70, 60, 50}[id-160])
	case id >= 170 && id <= 175:
		a.setVideoOpacity([]int{100, 90, 80, 70, 60, 50}[id-170])
	case id >= 200 && id <= 203:
		q := "best"
		switch id {
		case 201:
			q = "1080"
		case 202:
			q = "720"
		case 203:
			q = "480"
		}
		a.switchQuality(q)
	case id >= 300 && id <= 304:
		a.applyLayout(id - 300)
	case id >= 310 && id <= 314:
		a.saveLayout(id - 310)
	case id >= 320 && id <= 324:
		a.setDefaultLayout(id - 320)
	case id == 330:
		a.clearLayouts()
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

	w := int32(launcherW * a.scale)
	h := int32(a.launcherHeight() * a.scale)
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
	a.refreshRecent()
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

	w := int32(launcherW * a.scale)
	h := int32(a.launcherHeight() * a.scale)
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
	a.applyVideoVisibility()
}

func (a *App) hideBar(resume bool) {
	if a.barHwnd == 0 {
		return
	}
	pShowWindow.Call(a.barHwnd, SW_HIDE)
	a.barVisible = false
	wasPausedByBar := a.pausedByBar
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
	if !(resume && wasPausedByBar) {
		a.applyVideoVisibility()
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
	if a.loading {
		a.cancelLoading()
		return
	}
	a.startPlayback(getWindowText(a.editHwnd), true)
}

func (a *App) startPlayback(raw string, showBarIfError bool) {
	u, ts, errMsg := normalizeURL(raw)
	if errMsg != "" {
		a.showBarWith(raw, errMsg, true)
		return
	}
	// A browser click can also trigger YouTube's own navigation event. The
	// same video then arrives twice, and the second load used to restart a
	// resolver request that was already seconds into its work.
	key := videoKey(u)
	if key != "" && key == a.lastStartKey {
		if a.loading && time.Since(a.lastStartAt) < 20*time.Second {
			logf("ignoring duplicate load while %s is opening", key)
			if a.settings.BarVisible {
				a.showControlBar()
			}
			return
		}
		if !a.eofShown && time.Since(a.loadedAt) < 4*time.Second {
			logf("ignoring duplicate load just after %s opened", key)
			if a.settings.BarVisible {
				a.showControlBar()
			}
			return
		}
	}
	a.lastStartKey, a.lastStartAt = key, time.Now()
	// Remember where the current picture is before replacing it. mpv can
	// recreate or resize its window for a new stream, and on a multi-monitor
	// desktop that used to leave the new video on whichever monitor Windows
	// chose. This rect is restored once the new window exists.
	a.captureRect()
	a.switchRectValid = a.settings.HasRect && a.settings.Width >= 100 && a.settings.Height >= 60
	if p := a.player; p != nil {
		if st := p.snapshot(); st.Fullscreen {
			a.switchRectValid = false
		}
	}
	if a.switchRectValid {
		a.switchRect = RECT{
			Left: a.settings.X, Top: a.settings.Y,
			Right:  a.settings.X + a.settings.Width,
			Bottom: a.settings.Y + a.settings.Height,
		}
		logf("keeping video window at %d,%d %dx%d", a.settings.X, a.settings.Y, a.settings.Width, a.settings.Height)
	}
	a.saveCurrentPos()
	start := a.resumeFor(u, ts)
	a.loading, a.loadRestarted = true, false
	a.loadingURL, a.loadingStart = u, start
	a.loadRequested = time.Now()
	a.retries = 0
	a.pendingPause, a.pendingFullscreen = false, false
	a.settings.LastURL = u
	saveSettings(a.settings)
	a.setStatus("Opening video…", false)
	pKillTimer.Call(helperHwnd, TIMER_PREWARM)
	pKillTimer.Call(helperHwnd, TIMER_RETRY)
	pSetTimer.Call(helperHwnd, TIMER_LOADWAIT, 30000, 0)
	if a.player != nil && a.player.isRunning() && a.player.resolver != resolverPath() {
		logf("resolver changed; restarting player")
		old := a.player
		a.player = nil
		old.killAsync()
	}
	if a.player != nil && a.player.isRunning() {
		logf("reusing player %d for the next video", a.player.id)
	}
	if a.player == nil || !a.player.isRunning() {
		if err := a.startPlayer(u, start); err != nil {
			a.loading = false
			pKillTimer.Call(helperHwnd, TIMER_LOADWAIT)
			a.showBarWith(u, "Could not start the player: "+err.Error(), true)
		}
	} else if a.player.ready() {
		a.player.load(u, start)
	}
	if a.settings.BarVisible {
		a.showControlBar()
	}
	a.cbInvalidate()
}

func (a *App) cancelLoading() {
	a.loading = false
	a.loadingURL = ""
	a.pendingPause, a.pendingFullscreen = false, false
	pKillTimer.Call(helperHwnd, TIMER_LOADWAIT)
	pKillTimer.Call(helperHwnd, TIMER_RETRY)
	if p := a.player; p != nil {
		p.stop()
	}
	a.setStatus("Canceled. Ready for another link.", false)
	a.cbInvalidate()
}

func (a *App) warmPlayer() {
	if a.quitting || a.loading || (a.player != nil && a.player.isRunning()) {
		return
	}
	if err := a.startPlayer("", 0); err != nil {
		logf("player warmup skipped: %v", err)
	}
}

func (a *App) startPlayer(u string, startSec int) error {
	a.videoHidden = false
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
	if a.player.urlNow() == "" {
		a.player.stop()
		return
	}
	wasLoading := a.loading
	if !a.loadRequested.IsZero() {
		logf("timing stage=request-to-loaded elapsed_ms=%d", time.Since(a.loadRequested).Milliseconds())
	}
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
	if p := a.player; p != nil {
		a.loadedAt = time.Now()
		p.resetPosition()
		if a.pendingPause {
			p.pauseSet(true)
			a.pendingPause = false
		}
		if a.pendingFullscreen {
			p.command("set_property", "fullscreen", true)
			a.pendingFullscreen = false
		}
		a.segmentsKey = ""
		a.lastSkipEnd = 0
		a.refreshTrayTitle()
		a.trackPosition()
		if st := p.snapshot(); !st.Fullscreen && !a.pendingFullscreen {
			a.placePlayerWindow(p.id)
		}
		if a.settings.BarVisible && !a.browseOpen {
			a.showControlBar()
		}
		a.applyVideoVisibility()
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
		a.clearCurrentPos()
		a.updateTrayTip("")
		a.segmentsKey = ""
		a.segments = nil
		a.lastSkipEnd = 0
		a.pendingPause = false
		a.pendingFullscreen = false
		a.loadingStart = 0
		hasDur := p.snapshot().HasDuration
		msg := "Finished. Paste another link or press Esc."
		if !hasDur {
			msg = "Stream ended. Paste another link or press Esc."
		}
		a.captureRect()
		a.showBarWith(a.settings.LastURL, msg, false)
	case "error":
		a.loading = false
		a.pendingPause = false
		a.pendingFullscreen = false
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
	if a.player.urlNow() == "" {
		a.player = nil
		a.loading = false
		if a.settings.BarVisible {
			a.showControlBar()
		}
		return
	}
	wasLoading := a.loading
	a.loading = false
	pKillTimer.Call(helperHwnd, TIMER_LOADWAIT)
	pKillTimer.Call(helperHwnd, TIMER_RETRY)
	pKillTimer.Call(helperHwnd, TIMER_SNAP)
	a.saveCurrentPos()
	a.captureRect()
	a.updateTrayTip("")
	a.segmentsKey = ""
	a.segments = nil
	a.pendingPause = false
	a.pendingFullscreen = false
	a.pendingLayout = -1
	a.videoHidden = false
	a.hideControlBar()
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
	case TIMER_PREWARM:
		pKillTimer.Call(helperHwnd, TIMER_PREWARM)
		a.warmPlayer()
	case TIMER_POLL:
		a.pollCount++
		a.captureRect()
		a.trackPosition()
		a.refreshTrayTitle()
		if a.pollCount%5 == 0 {
			saveSettings(a.settings)
			if a.historyDirty && a.history != nil {
				a.history.save()
				a.historyDirty = false
			}
		}
	case TIMER_SPONSOR:
		a.sponsorTick()
	case TIMER_BAR:
		a.cbTick()
	case TIMER_LAYOUT:
		pKillTimer.Call(helperHwnd, TIMER_LAYOUT)
		if a.pendingLayout >= 0 {
			i := a.pendingLayout
			a.pendingLayout = -1
			a.applyLayoutNow(i)
		}
	case TIMER_SNAP:
		pKillTimer.Call(helperHwnd, TIMER_SNAP)
		a.snapAspect()
	case TIMER_RETRY:
		pKillTimer.Call(helperHwnd, TIMER_RETRY)
		if a.player != nil && a.loadingURL != "" {
			a.loading = true
			a.setStatus(fmt.Sprintf("Reconnecting… (%d/2)", a.retries), false)
			a.player.load(a.loadingURL, a.loadingStart)
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
				if err := a.startPlayer(a.loadingURL, a.loadingStart); err == nil {
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
	if !windowAlive(hwnd) {
		p.forgetWindow()
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
	if !windowAlive(hwnd) {
		// mpv may have recreated its window for the new stream. Find the new
		// HWND; WM_APP_WINDOWFOUND will run this placement again.
		p.forgetWindow()
		p.discoverWindow()
		return
	}
	w, h := a.settings.Width, a.settings.Height
	var x, y int32
	if a.switchRectValid {
		x, y = a.switchRect.Left, a.switchRect.Top
		w, h = rectW(a.switchRect), rectH(a.switchRect)
		logf("restoring video window to %d,%d %dx%d", x, y, w, h)
	} else if a.settings.HasRect {
		x, y = a.settings.X, a.settings.Y
	} else {
		if w < 100 {
			w = 960
		}
		if h < 60 {
			h = 540
		}
		var pt POINT
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		work := workAreaForPoint(pt)
		x = work.Left + (rectW(work)-w)/2
		y = work.Top + (rectH(work)-h)/2
	}
	if w < 100 {
		w = 960
	}
	if h < 60 {
		h = 540
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
	a.switchRectValid = false
	a.applyVideoOpacity()
}

func (a *App) snapAspect() {
	p := a.player
	if p == nil {
		return
	}
	st := p.snapshot()
	hwnd := p.hwndNow()
	if !windowAlive(hwnd) {
		p.forgetWindow()
		return
	}
	if st.Fullscreen || st.Dwidth <= 0 || st.Dheight <= 0 {
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
	if s == "" {
		return
	}
	if cds.DwData == 3 {
		a.browseOpen = false
		return
	}
	if cds.DwData == 2 {
		var x, y, w, h int32
		if _, err := fmt.Sscanf(s, "%d,%d,%d,%d", &x, &y, &w, &h); err == nil {
			a.settings.BrowseX, a.settings.BrowseY = x, y
			a.settings.BrowseWidth, a.settings.BrowseHeight = w, h
			a.settings.BrowseHasRect = true
			saveSettings(a.settings)
		}
		return
	}
	a.startPlayback(s, true)
}

// ---- misc ----------------------------------------------------------------

func (a *App) updateYtdlp() {
	if a.loading {
		a.setBarStatus("Finish or cancel loading before updating")
		return
	}
	if a.updating {
		a.setBarStatus("Resolver update already in progress")
		return
	}
	a.updating = true
	a.setBarStatus("Updating video resolver…")
	go func() {
		err := updateResolver()
		a.updateMu.Lock()
		a.updateResult = "Video resolver updated. The next video will use it."
		if err != nil {
			a.updateResult = "Update failed: " + err.Error()
		}
		a.updateMu.Unlock()
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
		"A borderless, video-only window with a floating control bar.\n\n" +
		"The bar has transport, seek, volume, three video modes and layouts.\n" +
		"Ctrl+Alt+B shows or hides it. Pin it to keep it above other apps.\n\n" +
		"Tray → Browse YouTube opens the built-in browser.\n" +
		"The tray menu also has quality, layouts, resume and SponsorBlock.\n\n" +
		"Drag edges/corners to resize (aspect stays locked).\n" +
		"Double-click for fullscreen.\n\n" +
		"mpv and yt-dlp are bundled. If a video suddenly stops working,\n" +
		"use “Update yt-dlp” in the tray menu."
	messageBox(0, msg, "About YTFrame", MB_OK|MB_ICONINFORMATION)
}

func (a *App) requestExit() {
	if a.quitting {
		return
	}
	a.quitting = true
	a.saveCurrentPos()
	a.captureRect()
	saveSettings(a.settings)
	if a.history != nil {
		a.history.save()
	}
	if a.player != nil {
		a.player.quitAndWait(2 * time.Second)
		a.player = nil
	}
	a.removeTray()
	a.closeBrowse()
	for hk := uintptr(HK_PASTE); hk <= HK_LAST; hk++ {
		pUnregisterHotKey.Call(helperHwnd, hk)
	}
	a.cbFreeGDI()
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
	a.saveCurrentPos()
	a.captureRect()
	saveSettings(a.settings)
	if a.history != nil {
		a.history.save()
	}
	if a.player != nil {
		a.player.quitAndWait(2 * time.Second)
		a.player = nil
	}
	a.closeBrowse()
	a.removeTray()
	a.freeGDI()
}

// ---- resume, quality and SponsorBlock ------------------------------------

func (a *App) resumeFor(u string, ts int) int {
	if ts > 0 || hasStartParam(u) {
		return ts
	}
	if !a.settings.Resume {
		return 0
	}
	return resumeStart(u, a.history)
}

func (a *App) saveCurrentPos() {
	p := a.player
	if p == nil || a.history == nil || !p.isRunning() {
		return
	}
	st := p.snapshot()
	url := p.urlNow()
	if url == "" || hasPlaylistParam(url) {
		return
	}
	if !st.HasDuration || st.Duration <= 0 || st.Pos <= 5 || st.Eof {
		return
	}
	key := videoKey(url)
	if key == "" {
		return
	}
	a.history.set(key, HistoryEntry{
		URL: url, Title: st.Title, Pos: st.Pos, Dur: st.Duration, Updated: nowUnix(),
	})
	a.history.save()
	a.historyDirty = false
}

func (a *App) clearCurrentPos() {
	p := a.player
	if p == nil || a.history == nil {
		return
	}
	url := p.urlNow()
	if url == "" || hasPlaylistParam(url) {
		return
	}
	st := p.snapshot()
	// Keep finished videos in Recents, while clearing only the resume offset.
	a.history.set(videoKey(url), HistoryEntry{URL: url, Title: st.Title, Pos: 0, Dur: st.Duration, Updated: nowUnix()})
	a.history.save()
	a.historyDirty = false
}

func (a *App) trackPosition() {
	p := a.player
	if p == nil || a.history == nil || !p.isRunning() || a.loading {
		return
	}
	if time.Since(a.loadedAt) < 2*time.Second {
		return
	}
	st := p.snapshot()
	url := p.urlNow()
	if url == "" || hasPlaylistParam(url) {
		return
	}
	if !st.HasDuration || st.Duration <= 0 || st.Pos <= 5 || st.Eof {
		return
	}
	key := videoKey(url)
	if key == "" {
		return
	}
	if e, ok := a.history.get(key); ok &&
		math.Abs(e.Pos-st.Pos) < 2 && e.Dur == st.Duration && e.Title == st.Title {
		return
	}
	a.history.set(key, HistoryEntry{
		URL: url, Title: st.Title, Pos: st.Pos, Dur: st.Duration, Updated: nowUnix(),
	})
	a.historyDirty = true
}

func (a *App) refreshTrayTitle() {
	title := ""
	if p := a.player; p != nil && p.isRunning() {
		title = p.snapshot().Title
	}
	if title != a.lastTrayTitle {
		a.lastTrayTitle = title
		a.updateTrayTip(title)
	}
}

func (a *App) updateTrayTip(title string) {
	if !a.trayAdded {
		return
	}
	tip := "YTFrame — video only"
	if title != "" {
		r := []rune(title)
		if len(r) > 90 {
			r = append(r[:90], '…')
		}
		tip = "YTFrame — " + string(r)
	}
	var nid NOTIFYICONDATAW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = helperHwnd
	nid.UID = 1
	nid.UFlags = NIF_TIP
	u := syscall.StringToUTF16(tip)
	n := len(u)
	if n > len(nid.SzTip)-1 {
		n = len(nid.SzTip) - 1
	}
	copy(nid.SzTip[:], u[:n])
	nid.SzTip[len(nid.SzTip)-1] = 0
	pShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
	a.lastTrayTitle = title
}

func (a *App) switchQuality(q string) {
	switch q {
	case "best", "1080", "720", "480":
	default:
		return
	}
	if q == a.settings.Quality {
		return
	}
	a.settings.Quality = q
	saveSettings(a.settings)

	p := a.player
	if p == nil || !p.isRunning() || a.loading {
		return
	}
	st := p.snapshot()
	if st.PlaylistCount > 1 {
		logf("quality change deferred: playlist")
		return
	}
	url := p.urlNow()
	low := strings.ToLower(url)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		return
	}

	a.saveCurrentPos()
	a.captureRect()
	a.pendingPause = st.Pause
	a.pendingFullscreen = st.Fullscreen
	pos := 0
	if st.HasDuration && !st.Eof && st.Pos > 5 {
		pos = int(st.Pos)
	}

	a.loading = true
	a.loadRestarted = false
	a.loadingURL = url
	a.loadingStart = pos
	a.retries = 0
	a.loadRequested = time.Now()
	a.player.load(url, pos)
	a.setStatus("Changing quality…", false)
	a.cbInvalidate()
	pSetTimer.Call(helperHwnd, TIMER_LOADWAIT, 30000, 0)
	logf("quality -> %s at %ds", q, pos)
}

func (a *App) setControls(on bool) {
	a.settings.Controls = on
	saveSettings(a.settings)
	if p := a.player; p != nil {
		mode := "never"
		if on {
			mode = "auto"
		}
		p.command("script-message", "osc-visibility", mode, "silent")
	}
}

func (a *App) sponsorTick() {
	if !a.settings.SponsorBlock || a.sponsor == nil {
		return
	}
	p := a.player
	if p == nil || !p.isRunning() || a.loading {
		return
	}
	if time.Since(a.loadedAt) < 2*time.Second {
		return
	}
	st := p.snapshot()
	if !st.HasDuration || st.Duration <= 0 || st.Pause || st.Seeking || st.Eof {
		return
	}
	url := p.urlNow()
	if url == "" || hasPlaylistParam(url) {
		return
	}
	id := youtubeID(url)
	if id == "" {
		return
	}
	if id != a.segmentsKey {
		if segs, ok := a.sponsor.cached(id); ok {
			a.segments = segs
			a.segmentsKey = id
			a.lastSkipEnd = 0
		} else {
			a.sponsor.fetchAsync(id)
			return
		}
	}
	if len(a.segments) == 0 {
		return
	}
	pos := st.Pos
	if a.lastSkipEnd > 0 && pos < a.lastSkipEnd-2 {
		a.lastSkipEnd = 0
	}
	for _, seg := range a.segments {
		if pos < seg.Start || pos >= seg.End-0.15 {
			continue
		}
		if seg.End <= pos+0.25 || seg.End == a.lastSkipEnd {
			continue
		}
		a.lastSkipEnd = seg.End
		p.seekAbsolute(seg.End)
		logf("sponsor: skipped %s %.1f-%.1f at %.1f", seg.Category, seg.Start, seg.End, pos)
		return
	}
}

func (a *App) showControlsHelp() {
	msg := "YTFrame controls\n\n" +
		"The floating bar has play/pause, seek, volume, video modes\n" +
		"and layouts. Drag it by the grip.\n\n" +
		"Ctrl+Alt+B               show or hide the control bar\n" +
		"Space / Ctrl+Alt+Space   play or pause\n" +
		"Left / Right             seek 5 seconds\n" +
		"Ctrl+Alt+J / Ctrl+Alt+L  seek 10 seconds from anywhere\n" +
		"Ctrl+Alt+1...5           apply a layout\n" +
		"Ctrl+Alt+Shift+1...5     save the current layout\n" +
		"Up / Down / wheel        volume\n" +
		"M / Ctrl+Alt+M           mute\n" +
		"F or double-click        fullscreen\n" +
		"Esc                      leave fullscreen\n" +
		"[ / ]                    playback speed\n" +
		"PGUP / PGDN              chapters\n" +
		"Home                     start from the beginning\n" +
		"Ctrl+V                   play the clipboard link\n" +
		"Ctrl+Alt+V               open the paste bar\n" +
		"Right-click the video    player menu\n" +
		"Right-click the tray     app menu\n\n" +
		"Tray → Browse YouTube opens the browser. The tray also has\n" +
		"quality, layouts, resume and SponsorBlock."
	messageBox(0, msg, "YTFrame controls", MB_OK|MB_ICONINFORMATION)
}

// ---- URL helpers ---------------------------------------------------------

func isPlayableInput(s string) bool {
	_, _, err := normalizeURL(s)
	return err == ""
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
			if low == prefix || strings.HasPrefix(low, prefix+"/") || strings.HasPrefix(low, prefix+"?") {
				s = "https://" + s
				low = strings.ToLower(s)
				break
			}
		}
	}
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		u, err := url.Parse(s)
		if err != nil || u.Hostname() == "" || strings.ContainsAny(u.Host, " \t\r\n") {
			return "", 0, "That link is incomplete. Paste the full video URL."
		}
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
	if v == "" {
		v = strings.TrimPrefix(u.Fragment, "t=")
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

func hasStartParam(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	q := u.Query()
	_, t := q["t"]
	_, start := q["start"]
	return t || start || strings.HasPrefix(u.Fragment, "t=")
}
