# Performance measurements

The 1.3 startup work removes resolver extraction, keeps a hidden idle mpv ready while the launcher is shown, reuses the browser's existing WebView2 session, reuses mpv for quality changes and removes synchronous disk flushes from UI diagnostics. Unchanged idle/paused remote state no longer triggers timer-driven redraws. The timeline updates at 10 Hz while playing.

A Windows regression also exposed a synchronous named-pipe deadlock: an idle observer could block subsequent control writes, including a new load or quit command. Playback now uses Microsoft's go-winio with overlapped I/O. A pipe regression test keeps the observer waiting while checking that a control write and response still complete.

## Native Windows results

Measured in [Windows CI run 44813421716](https://github.com/cbrules3033/ytframe/actions/runs/44813421716) on Windows NT 10.0.26100.0, using the same yt-dlp 2026.10.26 release for both resolver builds:

| Operation | Before / cold process | After / reused process |
| --- | ---: | ---: |
| Resolver process startup, median of five trials after the initial launch | 1,614.67 ms, single-file executable | 292.32 ms, unpacked executable |
| Local WAV load, subsequent cold-player launches | 38, 40, 38 ms | 9, 9, 9, 9 ms, prestarted idle player |

The first cold-player trial took 935 ms. It is retained in the raw report, alongside all subsequent trials. Resolver first-launch samples were 2,348.92 ms for the single-file executable and 329.53 ms for the unpacked executable.

The resolver comparison is **about 82% less process-start overhead** in this run. These figures do not represent total YouTube click-to-picture time. Network requests, YouTube's responses, JavaScript challenges, format selection, the selected stream, the GPU and the user's antivirus/storage can affect that time. No video quality limit, TLS bypass, extractor-client override or network-cache disabling is used to obtain these results.

## Reproduce

On Windows, from the repository:

```powershell
./scripts/package.ps1
./scripts/benchmark.ps1 -PackagePath dist/staging/YTFrame-win64
```

The benchmark compares the official single-file and unpacked resolvers with `--ignore-config --version`; it measures process startup without network access. The playback benchmark uses a generated local WAV and the actual packaged mpv, with null audio/video outputs to remove GPU and audio-device variance. It runs four cold-process and four prestarted-process trials. This tests engine startup and IPC responsiveness, rather than the first rendered video frame on a desktop.

CI runs the benchmarks during packaging. `RESOLVER_BENCHMARK.json` and `PLAYBACK_BENCHMARK.json` inside each Windows ZIP are the raw results for that exact build. The original single-file resolver is downloaded for comparison and is not shipped in the package.

For desktop investigation, `ytframe.log` records request-to-loaded, process-to-IPC and first playback-restart timings with millisecond precision. The log labels these stages separately. Browser reopen and unchanged-state painting also have behavioral regression checks; no unmeasured browser speed percentage is claimed.

## Tradeoffs

Keeping the idle player and an already opened browser session uses additional memory. The player performs no network request until a link is submitted. Browser media is paused and the WebView2 controller is hidden when the browser is hidden. The remote menu's **Close YouTube browser** releases that session, and quitting YTFrame ends both processes.

Updates install the verified resolver into a versioned directory, then atomically switch a small manifest. Loaded executables and DLLs are not overwritten. Older resolver versions remain in the app's data directory; deleting an unused old version is optional. Legacy portable folders with a root-level `yt-dlp.exe` remain supported as a fallback.
