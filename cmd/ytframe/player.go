//go:build windows

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type Player struct {
	app *App
	id  int

	mu       sync.Mutex
	cmd      *exec.Cmd
	hwnd     uintptr
	conn     *os.File
	cmdCh    chan []byte
	running  bool
	done     chan struct{}
	quitting bool

	dwidth, dheight int
	duration        float64
	hasDuration     bool
	pause           bool
	fullscreen      bool
	ontop           bool
	mute            bool
	volume          int
	url             string
	endReason       string
	endError        string
	eof             bool
	eofPosted       bool
	loadNotified    bool
}

func newPlayer(a *App) *Player {
	a.playerSeq++
	p := &Player{app: a, id: a.playerSeq, volume: a.settings.Volume}
	return p
}

func (p *Player) pipeName() string {
	return fmt.Sprintf(`\\.\pipe\ytframe-%d`, os.Getpid())
}

func (p *Player) start(url string, startSec int) error {
	mpvPath := filepath.Join(dataDirPath, "mpv.exe")
	if _, err := os.Stat(mpvPath); err != nil {
		return fmt.Errorf("mpv.exe is missing from the YTFrame folder")
	}
	ytdlpPath := filepath.Join(dataDirPath, "yt-dlp.exe")

	writeInputConf(filepath.Join(dataDirPath, "input.conf"))

	w := p.app.settings.Width
	h := p.app.settings.Height
	args := []string{
		"--no-config",
		"--no-border",
		"--no-osc",
		"--no-osd-bar",
		"--osd-level=0",
		"--no-terminal",
		"--force-window=immediate",
		"--keep-open=yes",
		"--idle=yes",
		"--auto-window-resize=no",
		"--keepaspect-window=yes",
		"--title=YTFrame",
		"--input-ipc-server=" + p.pipeName(),
		fmt.Sprintf("--geometry=%dx%d", w, h),
		"--autofit-larger=100%x100%",
		fmt.Sprintf("--volume=%d", p.volume),
		"--log-file=" + filepath.Join(dataDirPath, "mpv.log"),
		"--input-conf=" + filepath.Join(dataDirPath, "input.conf"),
		"--script-opts=ytdl_hook-ytdl_path=" + ytdlpPath,
		"--msg-level=all=warn",
	}
	if p.app.settings.Ontop {
		args = append(args, "--ontop=yes")
	}
	if startSec > 0 {
		args = append(args, fmt.Sprintf("--start=%d", startSec))
	}
	if extra := strings.TrimSpace(p.app.settings.ExtraArgs); extra != "" {
		args = append(args, strings.Fields(extra)...)
	}
	args = append(args, "--", url)

	cmd := exec.Command(mpvPath, args...)
	cmd.Dir = dataDirPath
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		return err
	}

	p.mu.Lock()
	p.cmd = cmd
	p.running = true
	p.url = url
	p.loadNotified = false
	p.endReason = ""
	p.endError = ""
	p.eof = false
	p.done = make(chan struct{})
	done := p.done
	p.mu.Unlock()

	logf("mpv started pid=%d url=%s", cmd.Process.Pid, url)

	go func() {
		cmd.Wait()
		close(done)
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
		logf("mpv exited id=%d", p.id)
		postMsg(WM_APP_MPVEXIT, uintptr(p.id), 0)
	}()

	go p.connectLoop()
	go p.findWindowLoop(cmd.Process.Pid)
	return nil
}

func (p *Player) connectLoop() {
	pipe := p.pipeName()
	access := uint32(syscall.GENERIC_READ | syscall.GENERIC_WRITE)
	for i := 0; i < 120; i++ {
		h, err := syscall.CreateFile(u16(pipe), access, FILE_SHARE_READ|FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
		if err == nil {
			f := os.NewFile(uintptr(h), "mpv-ipc")
			ch := make(chan []byte, 64)
			p.mu.Lock()
			p.conn = f
			p.cmdCh = ch
			p.mu.Unlock()
			logf("mpv ipc connected")
			go p.writeLoop(f, ch)
			p.observeAll()
			go p.readLoop(f)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	logf("mpv ipc connection timed out")
}

func (p *Player) observeAll() {
	props := []string{
		"dwidth", "dheight", "duration", "pause", "fullscreen", "ontop",
		"volume", "mute", "eof-reached", "path", "media-title", "idle-active",
	}
	for i, name := range props {
		p.command("observe_property", i+1, name)
	}
}

func (p *Player) readLoop(f *os.File) {
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		var m struct {
			Event  string          `json:"event"`
			Name   string          `json:"name"`
			Data   json.RawMessage `json:"data"`
			Reason string          `json:"reason"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal(line, &m); err != nil {
			continue
		}
		switch m.Event {
		case "property-change":
			p.applyProperty(m.Name, m.Data)
		case "file-loaded":
			p.mu.Lock()
			already := p.loadNotified
			p.loadNotified = true
			p.eof = false
			p.mu.Unlock()
			if !already {
				logf("mpv file-loaded id=%d", p.id)
				postMsg(WM_APP_LOADED, uintptr(p.id), 0)
			}
		case "end-file":
			p.mu.Lock()
			p.endReason = m.Reason
			p.endError = m.Error
			p.mu.Unlock()
			logf("mpv end-file reason=%q error=%q", m.Reason, m.Error)
			postMsg(WM_APP_ENDFILE, uintptr(p.id), 0)
		}
	}
	logf("mpv ipc closed")
}

func (p *Player) applyProperty(name string, raw json.RawMessage) {
	var v interface{}
	json.Unmarshal(raw, &v)
	num := func() (float64, bool) {
		if f, ok := v.(float64); ok {
			return f, true
		}
		return 0, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	switch name {
	case "dwidth":
		if f, ok := num(); ok {
			p.dwidth = int(f)
			postMsg(WM_APP_SNAP, 0, 0)
		}
	case "dheight":
		if f, ok := num(); ok {
			p.dheight = int(f)
			postMsg(WM_APP_SNAP, 0, 0)
		}
	case "duration":
		if v == nil {
			p.hasDuration = false
			p.duration = 0
		} else if f, ok := num(); ok {
			p.hasDuration = true
			p.duration = f
		}
	case "pause":
		if b, ok := v.(bool); ok {
			p.pause = b
		}
	case "fullscreen":
		if b, ok := v.(bool); ok {
			p.fullscreen = b
		}
	case "ontop":
		if b, ok := v.(bool); ok {
			p.ontop = b
		}
	case "mute":
		if b, ok := v.(bool); ok {
			p.mute = b
		}
	case "volume":
		if f, ok := num(); ok {
			p.volume = int(f)
		}
	case "eof-reached":
		if b, ok := v.(bool); ok {
			p.eof = b
			if b && !p.eofPosted {
				p.eofPosted = true
				p.endReason = "eof"
				postMsg(WM_APP_ENDFILE, uintptr(p.id), 0)
			}
		}
	case "path":
		if s, ok := v.(string); ok && s != "" {
			// Keep the user-facing URL we loaded; path can be a resolved
			// stream URL. Only use it if we do not have one.
			if p.url == "" {
				p.url = s
			}
		}
	}
}

// writeLoop owns the pipe. Commands are queued, so a stalled mpv can never
// freeze the UI thread; a wedged player is restarted by the load watchdog.
func (p *Player) writeLoop(f *os.File, ch chan []byte) {
	w := bufio.NewWriter(f)
	for b := range ch {
		if _, err := w.Write(b); err != nil {
			logf("ipc write failed: %v", err)
			return
		}
		if err := w.Flush(); err != nil {
			logf("ipc flush failed: %v", err)
			return
		}
	}
}

func (p *Player) command(args ...interface{}) bool {
	msg := map[string]interface{}{"command": args}
	b, err := json.Marshal(msg)
	if err != nil {
		return false
	}
	b = append(b, byte(10))
	p.mu.Lock()
	ch := p.cmdCh
	p.mu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- b:
		return true
	default:
		logf("ipc queue full, dropping command")
		return false
	}
}

func (p *Player) load(url string, startSec int) {
	p.mu.Lock()
	p.loadNotified = false
	p.eof = false
	p.eofPosted = false
	p.endReason = ""
	p.endError = ""
	p.url = url
	p.mu.Unlock()

	if startSec > 0 {
		p.command("loadfile", url, "replace", fmt.Sprintf("start=%d", startSec))
	} else {
		p.command("loadfile", url, "replace")
	}
	p.command("set_property", "pause", false)
	logf("loading %s", url)
}

func (p *Player) pauseSet(on bool) { p.command("set_property", "pause", on) }
func (p *Player) togglePause()     { p.command("cycle", "pause") }
func (p *Player) toggleMute()      { p.command("cycle", "mute") }
func (p *Player) toggleFullscreen() {
	p.command("cycle", "fullscreen")
}

func (p *Player) setOntop(on bool) {
	if on {
		p.command("set_property", "ontop", true)
	} else {
		p.command("set_property", "ontop", false)
	}
}

type PlayerState struct {
	Pause       bool
	Fullscreen  bool
	Ontop       bool
	Mute        bool
	Eof         bool
	Running     bool
	Dwidth      int
	Dheight     int
	HasDuration bool
	Volume      int
}

func (p *Player) snapshot() PlayerState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PlayerState{
		Pause: p.pause, Fullscreen: p.fullscreen, Ontop: p.ontop,
		Mute: p.mute, Eof: p.eof, Running: p.running,
		Dwidth: p.dwidth, Dheight: p.dheight,
		HasDuration: p.hasDuration, Volume: p.volume,
	}
}

func (p *Player) isRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

func (p *Player) ready() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmdCh != nil
}

func (p *Player) hwndNow() uintptr {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hwnd
}

func (p *Player) killAsync() {
	p.mu.Lock()
	cmd := p.cmd
	p.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		go cmd.Process.Kill()
	}
}

func (p *Player) quitAndWait(timeout time.Duration) {
	p.mu.Lock()
	p.quitting = true
	cmd := p.cmd
	done := p.done
	p.mu.Unlock()
	p.command("quit")
	if done != nil {
		select {
		case <-done:
		case <-time.After(timeout):
			if cmd != nil && cmd.Process != nil {
				cmd.Process.Kill()
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}
	}
	p.mu.Lock()
	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}
	p.cmdCh = nil
	p.mu.Unlock()
}

func (p *Player) findWindowLoop(pid int) {
	for i := 0; i < 100; i++ {
		if hwnd := findWindowForPID(uint32(pid)); hwnd != 0 {
			var rc RECT
			if ok, _, _ := pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc))); ok != 0 && rectW(rc) > 0 {
				p.mu.Lock()
				p.hwnd = hwnd
				p.mu.Unlock()
				logf("mpv window found %dx%d", rectW(rc), rectH(rc))
				postMsg(WM_APP_WINDOWFOUND, uintptr(p.id), 0)
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	logf("mpv window not found")
}

var (
	enumMu     sync.Mutex
	enumPidSet uint32
	enumBest   uintptr
	enumArea   int32
)

var enumWindowsCB = syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
	var pid uint32
	pGetWindowThreadProcID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != enumPidSet {
		return 1
	}
	vis, _, _ := pIsWindowVisible.Call(hwnd)
	if vis == 0 {
		return 1
	}
	var rc RECT
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	area := rectW(rc) * rectH(rc)
	if area > enumArea {
		enumArea = area
		enumBest = hwnd
	}
	return 1
})

func findWindowForPID(pid uint32) uintptr {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumPidSet = pid
	enumBest = 0
	enumArea = 0
	pEnumWindows.Call(enumWindowsCB, 0)
	return enumBest
}

func writeInputConf(path string) {
	content := "# YTFrame: Ctrl+V replaces the current video with the clipboard link\n" +
		"Ctrl+v loadfile ${clipboard/text}\n"
	if b, err := os.ReadFile(path); err == nil && string(b) == content {
		return
	}
	os.WriteFile(path, []byte(content), 0o644)
}

func postMsg(msg uint32, w, l uintptr) {
	if helperHwnd != 0 {
		pPostMessageW.Call(helperHwnd, uintptr(msg), w, l)
	}
}

// ---- error text ----------------------------------------------------------

func (p *Player) lastErrorText() string {
	p.mu.Lock()
	evErr := p.endError
	p.mu.Unlock()
	if evErr != "" && evErr != "error" && evErr != "cancelled" {
		return evErr
	}
	tail := readLogTail(filepath.Join(dataDirPath, "mpv.log"), 96*1024)
	lines := strings.Split(tail, "\n")
	var cands []string
	for i := len(lines) - 1; i >= 0 && len(cands) < 4; i-- {
		ln := strings.TrimSpace(stripANSI(lines[i]))
		if ln == "" {
			continue
		}
		low := strings.ToLower(ln)
		if strings.Contains(ln, "ERROR") || strings.Contains(low, "error:") ||
			strings.Contains(low, "failed to") || strings.Contains(low, "unable to") {
			cands = append(cands, ln)
		}
	}
	if len(cands) == 0 {
		return ""
	}
	// Oldest to newest of the candidates we collected.
	for i, j := 0, len(cands)-1; i < j; i, j = i+1, j-1 {
		cands[i], cands[j] = cands[j], cands[i]
	}
	out := strings.Join(cands, " | ")
	if len(out) > 600 {
		out = out[len(out)-600:]
	}
	return out
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func readLogTail(path string, n int64) string {
	h, err := syscall.CreateFile(u16(path), syscall.GENERIC_READ,
		FILE_SHARE_READ|FILE_SHARE_WRITE|FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		return ""
	}
	f := os.NewFile(uintptr(h), path)
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	off := st.Size() - n
	if off < 0 {
		off = 0
	}
	if _, err := f.Seek(off, 0); err != nil {
		return ""
	}
	buf := make([]byte, n)
	m, _ := f.Read(buf)
	return string(buf[:m])
}
