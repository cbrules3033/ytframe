//go:build windows

package main

import (
	"bufio"
	"context"
	"io"

	"encoding/json"
	"fmt"
	"github.com/Microsoft/go-winio"
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

	mu            sync.Mutex
	cmd           *exec.Cmd
	hwnd          uintptr
	conn          io.ReadWriteCloser
	cmdCh         chan []byte
	running       bool
	done          chan struct{}
	quitting      bool
	findingWindow bool

	dwidth, dheight int
	duration        float64
	hasDuration     bool
	pos             float64
	seeking         bool
	title           string
	playlistCount   int
	playlistPos     int
	pause           bool
	buffering       bool
	bufferPercent   float64
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
	processStarted  time.Time
	loadStarted     time.Time
	firstFrame      bool
	resolver        string
}

func newPlayer(a *App) *Player {
	a.playerSeq++
	p := &Player{app: a, id: a.playerSeq, volume: a.settings.Volume}
	return p
}

func (p *Player) pipeName() string {
	return fmt.Sprintf(`\\.\pipe\ytframe-%d-%d`, os.Getpid(), p.id)
}

func (p *Player) start(url string, startSec int) error {
	mpvPath := bundledPath("mpv.exe")
	if _, err := os.Stat(mpvPath); err != nil {
		return fmt.Errorf("mpv.exe is missing from the YTFrame folder")
	}
	ytdlpPath := resolverPath()
	p.resolver = ytdlpPath
	p.processStarted = time.Now()
	if url != "" {
		p.loadStarted = p.processStarted
	}

	writeInputConf(filepath.Join(dataDirPath, "input.conf"))

	w := p.app.settings.Width
	h := p.app.settings.Height
	args := []string{
		"--no-config",
		"--no-border",
		"--no-osd-bar",
		"--osd-level=1",
		"--osd-duration=1000",
		"--no-terminal",
		"--force-window=no",
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
		"--msg-level=all=warn",
	}

	oscVisibility := "auto"
	if !p.app.settings.Controls {
		oscVisibility = "never"
	}
	scriptOpts := []string{
		"ytdl_hook-ytdl_path=" + ytdlpPath,
		"osc-layout=bottombar",
		"osc-visibility=" + oscVisibility,
		"osc-hidetimeout=1200",
		"osc-fadeduration=150",
		"osc-fadein=yes",
		"osc-barmargin=8",
		"osc-deadzonesize=0.6",
		"osc-windowcontrols=no",
		"osc-idlescreen=no",
		"osc-title=",
		"osc-icon_style=fluent",
		"osc-timetotal=yes",
		"osc-seekbarstyle=bar",
		"osc-greenandgrumpy=yes",
		"osc-background_color=#101114",
		"osc-buttons_color=#F2F3F5",
		"osc-timecode_color=#9AA0A6",
		"osc-time_pos_color=#F2F3F5",
		"osc-time_pos_outline_color=#101114",
		"osc-held_element_color=#3EA6FF",
	}
	args = append(args, "--script-opts="+strings.Join(scriptOpts, ","))
	if q := qualityFormat(p.app.settings.Quality); q != "" {
		args = append(args, "--ytdl-format="+q)
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
	if url != "" {
		args = append(args, "--force-window=immediate", "--", url)
	}

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
	p.pos = 0
	p.seeking = false
	p.title = ""
	p.playlistCount = 0
	p.playlistPos = 0
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
		p.mu.Lock()
		p.running = false
		conn := p.conn
		p.conn = nil
		p.cmdCh = nil
		p.mu.Unlock()
		close(done)
		if conn != nil {
			conn.Close()
		}
		logf("mpv exited id=%d", p.id)
		postMsg(WM_APP_MPVEXIT, uintptr(p.id), 0)
	}()

	go p.connectLoop()
	if url != "" {
		p.discoverWindow()
	}
	return nil
}

func (p *Player) connectLoop() {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	go func() {
		select {
		case <-p.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	// IOCP-backed overlapped handles allow writes while the observer is blocked
	// reading an idle pipe. A synchronous os.File deadlocks here on Windows.
	var conn io.ReadWriteCloser
	var err error
	for {
		conn, err = winio.DialPipeContext(ctx, p.pipeName())
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			logf("mpv IPC connection failed: %v", err)
			return
		case <-time.After(15 * time.Millisecond):
		}
	}
	ch := make(chan []byte, 64)
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		conn.Close()
		return
	}
	p.conn, p.cmdCh = conn, ch
	p.mu.Unlock()
	logf("timing player=%d stage=ipc elapsed_ms=%d", p.id, time.Since(p.processStarted).Milliseconds())
	go p.writeLoop(conn, ch)
	p.observeAll()
	go p.readLoop(conn)
	postMsg(WM_APP_MPVREADY, uintptr(p.id), 0)
}

func (p *Player) observeAll() {
	props := []string{
		"dwidth", "dheight", "duration", "pause", "fullscreen", "ontop",
		"volume", "mute", "eof-reached", "path", "media-title", "idle-active",
		"time-pos", "seeking", "playlist-count", "playlist-pos",
		"paused-for-cache", "cache-buffering-state",
	}
	for i, name := range props {
		p.command("observe_property", i+1, name)
	}
}

func (p *Player) readLoop(f io.Reader) {
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		var m struct {
			Args   []string        `json:"args"`
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
		case "client-message":
			if len(m.Args) == 1 && m.Args[0] == "ytframe-paste" {
				postMsg(WM_APP_PASTE, uintptr(p.id), 0)
			}
		case "property-change":
			p.applyProperty(m.Name, m.Data)
		case "playback-restart":
			p.mu.Lock()
			first := !p.firstFrame && !p.loadStarted.IsZero()
			p.firstFrame = true
			started := p.loadStarted
			p.mu.Unlock()
			if first {
				logf("timing player=%d stage=first-frame elapsed_ms=%d", p.id, time.Since(started).Milliseconds())
			}
		case "file-loaded":
			p.mu.Lock()
			already := p.loadNotified
			p.loadNotified = true
			p.eof = false
			p.eofPosted = false
			started := p.loadStarted
			p.mu.Unlock()
			if !already {
				logf("timing player=%d stage=file-loaded elapsed_ms=%d", p.id, time.Since(started).Milliseconds())
				p.discoverWindow()
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
	case "time-pos":
		if v == nil {
			p.pos = 0
		} else if f, ok := num(); ok {
			p.pos = f
		}
	case "seeking":
		if b, ok := v.(bool); ok {
			p.seeking = b
		}
	case "media-title":
		if s, ok := v.(string); ok {
			p.title = s
		}
	case "playlist-count":
		if f, ok := num(); ok {
			p.playlistCount = int(f)
		}
	case "playlist-pos":
		if f, ok := num(); ok {
			p.playlistPos = int(f)
		}
	case "paused-for-cache":
		if b, ok := v.(bool); ok {
			p.buffering = b
		}
	case "cache-buffering-state":
		if f, ok := num(); ok {
			p.bufferPercent = f
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
func (p *Player) writeLoop(f io.Writer, ch chan []byte) {
	w := bufio.NewWriter(f)
	for {
		var b []byte
		select {
		case <-p.done:
			return
		case b = <-ch:
		}
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
	p.loadStarted = time.Now()
	p.firstFrame = false
	p.eof = false
	p.eofPosted = false
	p.endReason = ""
	p.endError = ""
	p.url = url
	p.pos = 0
	p.seeking = false
	p.title = ""
	p.duration = 0
	p.hasDuration = false
	p.mu.Unlock()

	// Always override --start, including zero, so the previous video's
	// command-line resume offset cannot leak into the next load.
	options := map[string]string{"start": fmt.Sprint(startSec)}
	if p.app != nil {
		q := qualityFormat(p.app.settings.Quality)
		if q == "" {
			q = "bestvideo+bestaudio/best"
		}
		options["ytdl-format"] = q
	}
	p.command("loadfile", url, "replace", -1, options)
	p.command("set_property", "force-window", "immediate")
	pause := p.app != nil && p.app.pendingPause
	p.command("set_property", "pause", pause)
	p.discoverWindow()
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
	Pause         bool
	Buffering     bool
	BufferPercent float64
	Fullscreen    bool
	Ontop         bool
	Mute          bool
	Eof           bool
	Seeking       bool
	Running       bool
	Dwidth        int
	Dheight       int
	Duration      float64
	Pos           float64
	HasDuration   bool
	Volume        int
	Title         string
	PlaylistCount int
}

func (p *Player) snapshot() PlayerState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PlayerState{
		Pause: p.pause, Buffering: p.buffering, BufferPercent: p.bufferPercent, Fullscreen: p.fullscreen, Ontop: p.ontop,
		Mute: p.mute, Eof: p.eof, Seeking: p.seeking, Running: p.running,
		Dwidth: p.dwidth, Dheight: p.dheight,
		Duration: p.duration, Pos: p.pos,
		HasDuration: p.hasDuration, Volume: p.volume,
		Title: p.title, PlaylistCount: p.playlistCount,
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

func (p *Player) resetPosition() {
	p.mu.Lock()
	p.pos = 0
	p.seeking = false
	p.mu.Unlock()
}

func (p *Player) urlNow() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.url
}

func (p *Player) seekRelative(sec float64) {
	p.command("seek", sec, "relative")
}

func (p *Player) seekAbsolute(sec float64) {
	p.command("seek", sec, "absolute+exact")
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
	for i := 0; i < 333; i++ {
		select {
		case <-p.done:
			return
		default:
		}
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
		select {
		case <-p.done:
			return
		case <-time.After(15 * time.Millisecond):
		}
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
		"Ctrl+v script-message ytframe-paste\n" +
		"UP add volume 5\nDOWN add volume -5\nHOME seek 0 absolute+exact\n"
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

func qualityFormat(q string) string {
	switch q {
	case "1080":
		return "bestvideo[height<=1080]+bestaudio/best[height<=1080]/best"
	case "720":
		return "bestvideo[height<=720]+bestaudio/best[height<=720]/best"
	case "480":
		return "bestvideo[height<=480]+bestaudio/best[height<=480]/best"
	}
	return ""
}

func (p *Player) stop() {
	p.mu.Lock()
	p.url, p.title = "", ""
	p.duration, p.pos = 0, 0
	p.hasDuration, p.loadNotified, p.firstFrame = false, false, false
	p.mu.Unlock()
	p.command("stop")
	p.command("set_property", "force-window", "no")
}

func (p *Player) discoverWindow() {
	p.mu.Lock()
	if p.findingWindow || p.hwnd != 0 || !p.running || p.cmd == nil {
		p.mu.Unlock()
		return
	}
	p.findingWindow = true
	pid := p.cmd.Process.Pid
	p.mu.Unlock()
	go func() { defer func() { p.mu.Lock(); p.findingWindow = false; p.mu.Unlock() }(); p.findWindowLoop(pid) }()
}
