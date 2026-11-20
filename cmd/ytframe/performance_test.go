//go:build windows

package main

import (
	"archive/zip"
	"bufio"

	"bytes"
	"encoding/binary"
	"encoding/json"
	"github.com/Microsoft/go-winio"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIdleRemoteStaysAvailableWithoutRepainting(t *testing.T) {
	a := &App{settings: defaultSettings(), cbVisible: true, cbHwnd: 0}
	if !a.cbReadState() || !a.cbIdle {
		t.Fatal("idle controls must have a valid idle state")
	}
	if a.cbReadState() {
		t.Fatal("unchanged idle state triggers needless repaint")
	}
	if !a.cbEnabled(cbBtnOpen) || !a.cbEnabled(cbBtnPlay) || a.cbEnabled(cbBtnBack) {
		t.Fatal("incorrect idle actions")
	}
	a.loading = true
	if !a.cbReadState() || !a.cbEnabled(cbBtnPlay) {
		t.Fatal("loading must expose Cancel immediately")
	}
	a.loading = false
	if !a.cbReadState() {
		t.Fatal("canceled request must return to idle")
	}
}

func TestRecentVideosAreNewestFirstAndBounded(t *testing.T) {
	h := &History{Entries: map[string]HistoryEntry{"a": {URL: "first", Updated: 1}, "b": {URL: "newest", Updated: 3}, "c": {URL: "second", Updated: 2}, "d": {Updated: 4}}}
	got := recentEntries(h, 2)
	if len(got) != 2 || got[0].URL != "newest" || got[1].URL != "second" {
		t.Fatalf("unexpected recents: %+v", got)
	}
	if recentEntries(nil, 2) != nil || recentEntries(h, 0) != nil {
		t.Fatal("empty recent history is invalid")
	}
}

func TestResolverArchiveRejectsTraversalBeforeWriting(t *testing.T) {
	for _, name := range []string{"../escape.exe", `..\escape.exe`, "/absolute.exe", `C:\escape.exe`} {
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		f, _ := w.Create("yt-dlp.exe")
		f.Write([]byte("placeholder"))
		f, _ = w.Create(name)
		f.Write([]byte("malicious"))
		w.Close()
		dir := t.TempDir()
		archive := filepath.Join(dir, "resolver.zip")
		os.WriteFile(archive, buf.Bytes(), 0o600)
		dest := filepath.Join(dir, "output")
		if err := unpackResolver(archive, dest); err == nil {
			t.Errorf("accepted archive path %q", name)
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatal("unsafe archive was partially extracted")
		}
	}
}

func TestResolverArchiveInstallsRuntime(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range []string{"yt-dlp.exe", "_internal/python.dll"} {
		f, _ := w.Create(name)
		f.Write([]byte("fixture"))
	}
	w.Close()
	dir := t.TempDir()
	archive := filepath.Join(dir, "resolver.zip")
	os.WriteFile(archive, buf.Bytes(), 0o600)
	dest := filepath.Join(dir, "resolver")
	if err := unpackResolver(archive, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "_internal", "python.dll")); err != nil {
		t.Fatal(err)
	}
}

// Run against the actual packaged mpv, without network, GPU or audio device variability.
// Results compare the same binary's cold launch with an already running idle player.
func TestPlaybackStartupBenchmark(t *testing.T) {
	if os.Getenv("YTFRAME_BENCHMARK") != "1" {
		t.Skip("run scripts/benchmark.ps1 against the packaged dependencies")
	}
	previous := dataDirPath
	dataDirPath = t.TempDir()
	defer func() { dataDirPath = previous }()
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	binary.Write(&wav, binary.LittleEndian, uint32(320000+36))
	wav.WriteString("WAVEfmt ")
	for _, v := range []interface{}{uint32(16), uint16(1), uint16(1), uint32(16000), uint32(32000), uint16(2), uint16(16)} {
		binary.Write(&wav, binary.LittleEndian, v)
	}
	wav.WriteString("data")
	binary.Write(&wav, binary.LittleEndian, uint32(320000))
	wav.Write(make([]byte, 320000))
	file := filepath.Join(dataDirPath, "fixture.wav")
	os.WriteFile(file, wav.Bytes(), 0o600)
	result := map[string][]int64{"cold_ms": {}, "warm_ms": {}}
	wait := func(p *Player, ready bool) {
		t.Helper()
		until := time.Now().Add(12 * time.Second)
		for time.Now().Before(until) {
			p.mu.Lock()
			ok := p.loadNotified
			if ready {
				ok = p.cmdCh != nil
			}
			p.mu.Unlock()
			if ok {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		t.Fatalf("player did not become ready (ready=%v)", ready)
	}
	a := &App{settings: defaultSettings()}
	a.settings.ExtraArgs = "--vo=null --ao=null"
	for _, warm := range []bool{false, true} {
		for i := 0; i < 4; i++ {
			p := newPlayer(a)
			started := time.Now()
			url := file
			if warm {
				url = ""
			}
			if err := p.start(url, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { p.quitAndWait(2 * time.Second) })
			if warm {
				wait(p, true)
				started = time.Now()
				p.load(file, 0)
			}
			wait(p, false)
			elapsed := time.Since(started).Milliseconds()
			p.quitAndWait(2 * time.Second)
			label := "cold_ms"
			if warm {
				label = "warm_ms"
			}
			result[label] = append(result[label], elapsed)
		}
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	t.Logf("Playback startup samples: %s", data)
	if path := os.Getenv("YTFRAME_BENCHMARK_OUTPUT"); path != "" {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIPCWriteWhileObserverIsWaiting(t *testing.T) {
	p := &Player{id: 9876, running: true, done: make(chan struct{}), processStarted: time.Now()}
	listener, err := winio.ListenPipe(p.pipeName(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	observed := make(chan struct{})
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			var request struct {
				Command []json.RawMessage `json:"command"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
				serverErr <- err
				return
			}
			if len(request.Command) < 2 {
				continue
			}
			var operation, property string
			json.Unmarshal(request.Command[0], &operation)
			json.Unmarshal(request.Command[1], &property)
			if operation == "observe_property" && len(request.Command) > 2 {
				json.Unmarshal(request.Command[2], &property)
				if property == "cache-buffering-state" {
					close(observed)
				}
			}
			if operation == "set_property" && property == "volume" {
				_, err = conn.Write([]byte("{\"event\":\"property-change\",\"name\":\"volume\",\"data\":42}\n"))
				serverErr <- err
				return
			}
		}
		serverErr <- scanner.Err()
	}()
	go p.connectLoop()
	defer func() {
		close(p.done)
		p.mu.Lock()
		conn := p.conn
		p.mu.Unlock()
		if conn != nil {
			conn.Close()
		}
	}()
	select {
	case <-observed:
	case err := <-serverErr:
		t.Fatalf("pipe server exited: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("observer setup timed out")
	}
	// Let the observer enter its next read, with no property events queued.
	time.Sleep(25 * time.Millisecond)
	if !p.command("set_property", "volume", 42) {
		t.Fatal("could not queue command")
	}
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		if p.snapshot().Volume == 42 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("a blocked observer prevented the control command from reaching mpv")
}
