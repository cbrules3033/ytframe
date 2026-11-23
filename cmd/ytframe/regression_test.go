//go:build windows

package main

import (
	"encoding/json"
	"math"
	"runtime"
	"strconv"
	"testing"
	"time"
	"unsafe"
)

func TestLoadAtTimestampUsesCurrentMPVProtocol(t *testing.T) {
	for _, start := range []int{0, 42} {
		p := &Player{cmdCh: make(chan []byte, 8), duration: 120, hasDuration: true, eofPosted: true}
		p.load("https://youtu.be/abcdefghijk", start)
		var message struct {
			Command []json.RawMessage `json:"command"`
		}
		if err := json.Unmarshal(<-p.cmdCh, &message); err != nil {
			t.Fatal(err)
		}
		if len(message.Command) != 5 {
			t.Fatalf("loadfile needs URL, flags, index, and options: %s", message.Command)
		}
		if string(message.Command[3]) != "-1" {
			t.Fatalf("options are in the insertion-index position: %s", message.Command)
		}
		var options map[string]string
		if err := json.Unmarshal(message.Command[4], &options); err != nil {
			t.Fatal(err)
		}
		if options["start"] != strconv.Itoa(start) {
			t.Fatalf("wrong start: %v", options)
		}
		if p.snapshot().HasDuration || p.snapshot().Duration != 0 || p.eofPosted {
			t.Fatal("previous video's duration/EOF leaked into new load")
		}
	}
}

func TestDuplicateBrowserNavigationDoesNotRestartLoad(t *testing.T) {
	previous := dataDirPath
	dataDirPath = t.TempDir()
	defer func() { dataDirPath = previous }()
	u := "https://www.youtube.com/watch?v=abcdefghijk"
	s := defaultSettings()
	s.BarVisible = false
	a := &App{
		settings:     s,
		loading:      true,
		loadingURL:   u,
		lastStartKey: videoKey(u),
		lastStartAt:  time.Now(),
	}
	a.startPlayback(u, true)
	if !a.loading || a.loadingURL != u {
		t.Fatal("a duplicate navigation event restarted or canceled the active load")
	}
}

func TestControlBarDownsampleAveragesPixels(t *testing.T) {
	src := []byte{
		0, 0, 0, 255, 10, 20, 30, 255,
		40, 50, 60, 255, 70, 80, 90, 255,
	}
	dst := make([]byte, 4)
	cbDownsample(unsafe.Pointer(&dst[0]), unsafe.Pointer(&src[0]), 2, 2, 1, 1, 2)
	if dst[0] != 30 || dst[1] != 37 || dst[2] != 45 || dst[3] != 255 {
		t.Fatalf("box filter produced %v", dst)
	}
}

func TestSwitchingVideoKeepsPlayerWindowRect(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hwnd, _, _ := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(u16("STATIC"))),
		uintptr(unsafe.Pointer(u16("YTFrame rect test"))),
		WS_POPUP|WS_VISIBLE,
		10, 20, 300, 200,
		0, 0, getModuleHandle(), 0,
	)
	if hwnd == 0 {
		t.Fatal("could not create a test window")
	}
	defer pDestroyWindow.Call(hwnd)
	p := &Player{id: 7, hwnd: hwnd, running: true, dwidth: 1280, dheight: 720}
	a := &App{
		settings:        defaultSettings(),
		player:          p,
		switchRect:      RECT{Left: 120, Top: 90, Right: 760, Bottom: 450},
		switchRectValid: true,
	}
	a.placePlayerWindow(7)
	var rc RECT
	if ok, _, _ := pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc))); ok == 0 {
		t.Fatal("could not read the test window rect")
	}
	if rc.Left != 120 || rc.Top != 90 || rectW(rc) != 640 || rectH(rc) != 360 {
		t.Fatalf("new video moved the window to %d,%d %dx%d", rc.Left, rc.Top, rectW(rc), rectH(rc))
	}
	if a.switchRectValid {
		t.Fatal("the saved switch rect was not consumed")
	}
}

func TestPlayerRestartsUseDifferentPipes(t *testing.T) {
	if (&Player{id: 1}).pipeName() == (&Player{id: 2}).pipeName() {
		t.Fatal("restarted player can connect to the old player's pipe")
	}
}

func TestTimestampOverridesResumeIncludingZero(t *testing.T) {
	u := "https://www.youtube.com/watch?v=abcdefghijk"
	h := &History{Entries: map[string]HistoryEntry{videoKey(u): {Pos: 80, Dur: 300}}}
	a := &App{settings: defaultSettings(), history: h}
	for _, tc := range []struct {
		suffix string
		want   int
	}{{"", 80}, {"&t=0", 0}, {"&start=0", 0}, {"&t=1m2s", 62}, {"#t=25", 25}} {
		raw := u + tc.suffix
		if got := a.resumeFor(raw, startParam(raw)); got != tc.want {
			t.Errorf("%s: got %d, want %d", raw, got, tc.want)
		}
	}
}

func TestRejectMalformedLinks(t *testing.T) {
	for _, raw := range []string{"https://", "https:///watch", "https://bad host/watch", "youtube.com.evil/watch"} {
		if _, _, err := normalizeURL(raw); err == "" {
			t.Errorf("accepted %q", raw)
		}
	}
	for _, raw := range []string{"youtu.be/abcdefghijk", "https://youtube.com/watch?v=abcdefghijk", `C:\Videos\clip.mp4`} {
		if _, _, err := normalizeURL(raw); err != "" {
			t.Errorf("rejected %q: %s", raw, err)
		}
	}
}

func TestRestoreRectPreservesSizeAndFitsWorkArea(t *testing.T) {
	work := RECT{Left: -1920, Top: 0, Right: 0, Bottom: 1080}
	for _, tc := range []struct{ in, want RECT }{
		{RECT{-2500, -100, -1300, 700}, RECT{-1920, 0, -720, 800}},
		{RECT{-100, 900, 1100, 1700}, RECT{-1200, 280, 0, 1080}},
		{RECT{-2500, -100, 500, 1900}, work},
	} {
		if got := fitWindowRect(tc.in, work); got != tc.want {
			t.Errorf("%v => %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSignedMouseCoordinates(t *testing.T) {
	v := makeLong(-12, -5)
	if mouseX(v) != -12 || mouseY(v) != -5 {
		t.Fatal("captured drag wraps negative client coordinates")
	}
	a := &App{cbLastDur: 120, cbSeekRect: RECT{Left: 22, Right: 538}}
	if got := a.cbSeekAt(-12); got != 0 {
		t.Fatalf("seek left of window = %f", got)
	}
	if got := a.cbSeekAt(900); got != 120 {
		t.Fatalf("seek right of window = %f", got)
	}
	if got := a.cbSeekAt(280); math.Abs(got-60) > 0.01 {
		t.Fatalf("midpoint seek = %f", got)
	}
}

func TestControlTargetsDoNotOverlapAcrossDPIScales(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	previous := app
	defer func() { app = previous }()
	a := &App{scale: 1, settings: defaultSettings()}
	app = a
	a.createControlBar()
	if a.cbHwnd == 0 {
		t.Fatal("could not create native controls window")
	}
	defer func() { pDestroyWindow.Call(a.cbHwnd); a.cbFreeGDI() }()
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		a.cbScale = scale
		pSetWindowPos.Call(a.cbHwnd, 0, 0, 0, uintptr(int32(cbW*scale)), uintptr(int32(cbH*scale)), SWP_NOZORDER|SWP_NOACTIVATE)
		a.cbLayout()
		for i, r := range a.cbBtn {
			if rectW(r) < int32(28*scale) || rectH(r) < int32(28*scale) {
				t.Errorf("scale %.2f: button %d too small: %v", scale, i, r)
			}
			for j := i + 1; j < len(a.cbBtn); j++ {
				q := a.cbBtn[j]
				if r.Left < q.Right && q.Left < r.Right && r.Top < q.Bottom && q.Top < r.Bottom {
					t.Errorf("scale %.2f: buttons %d and %d overlap", scale, i, j)
				}
			}
			if a.cbHitButton((r.Left+r.Right)/2, (r.Top+r.Bottom)/2) != i {
				t.Errorf("button %d cannot be clicked", i)
			}
		}
	}
}

func TestMonitorPointIsPassedByValue(t *testing.T) {
	p := POINT{X: -1920, Y: -400}
	v := uint64(packedPoint(p))
	if int32(v&0xffffffff) != p.X || int32(v>>32) != p.Y {
		t.Fatal("MonitorFromPoint must receive the packed POINT value")
	}
}

func TestLauncherSelectAll(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	previous := app
	defer func() { app = previous }()
	a := &App{scale: 1}
	app = a
	a.createBarWindow()
	if a.editHwnd == 0 {
		t.Fatal("could not create launcher input")
	}
	defer func() { pDestroyWindow.Call(a.barHwnd); a.freeGDI() }()
	setWindowText(a.editHwnd, "https://youtu.be/abcdefghijk")
	var original, keys [256]byte
	get := user32.NewProc("GetKeyboardState")
	set := user32.NewProc("SetKeyboardState")
	get.Call(uintptr(unsafe.Pointer(&original[0])))
	keys = original
	keys[VK_CONTROL] = 0x80
	set.Call(uintptr(unsafe.Pointer(&keys[0])))
	defer set.Call(uintptr(unsafe.Pointer(&original[0])))
	pSendMessageW.Call(a.editHwnd, WM_KEYDOWN, VK_A, 0)
	var start, end uint32
	pSendMessageW.Call(a.editHwnd, 0x00B0 /* EM_GETSEL */, uintptr(unsafe.Pointer(&start)), uintptr(unsafe.Pointer(&end)))
	if start != 0 || end != 28 {
		t.Fatalf("Ctrl+A selected %d…%d, want whole URL", start, end)
	}
}
