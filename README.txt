YTFrame 1.4.1
=============

A borderless, video-only YouTube window with a floating control bar and a
built-in YouTube browser.

QUICK START
-----------
1. Extract this folder anywhere (Desktop, USB stick, ...).
2. Run YTFrame.exe.
3. Paste a link and press Enter, or use Browse YouTube.

No installer, no registry, no admin rights. Settings, history, logs and the
browser profile are written next to YTFrame.exe. If the folder is read-only
they go to %LOCALAPPDATA%\YTFrame.

Windows SmartScreen will warn because the exe is unsigned:
click "More info" -> "Run anyway".

CONTROL BAR
-----------
A separate window, usable even when the video is tiny. It starts as a normal
window; use the pin or Window behavior -> Control bar always on top to keep
it above other apps. In normal-window mode it has a taskbar button and Alt+Tab
entry. Pinning turns it back into a no-taskbar tool window. The 1.3.4 remote
has a larger card, a dedicated timeline, a large play button and consistent
rounded icons. The card and icons are supersampled and filtered down, with
text drawn at native resolution so both stay crisp.

  seek line          click or drag to seek
  play / pause
  -10 / +10          seek 10 seconds
  speaker + slider   volume / mute
  fullscreen
  eye                video mode: show always / hide on pause / always hidden
  grid               layouts
  pin                control bar always on top (removes its taskbar button)
  Browse             open YouTube
  + Open             paste a new link
  -                  hide the bar
  stop square        cancel the current load

The bar is available before playback and remains visible during loading.
Restore it from the launcher's Show controls button, Ctrl+Alt+B or the tray.
The bars overflow menu has everything, including video always-on-top and the
optional in-video mpv controls.

VIDEO MODES
-----------
Show always          the paused frame stays visible
Hide on pause        pause hides the picture; the bar stays as the remote
Always hidden        audio-only; the picture stays hidden while playing

Seeking, volume, SponsorBlock and resume all work while the video is hidden.
Fullscreen from audio-only brings the picture back.

LAYOUTS
-------
Five slots storing the video window and control bar positions.

Ctrl+Alt+1...5          apply a layout
Ctrl+Alt+Shift+1...5    save the current arrangement into a slot
Tray -> Layouts         apply / save / set default / clear
Bar grid button         same menu

One slot can be the default; it is applied at startup / when the first video
opens. Applying a layout while playing honors the saved width and top-left
and recomputes the height for the current video aspect.

Switching to another video reuses the same player window and keeps it on the
same monitor, instead of reopening on the monitor where the browser or mouse
is.

WINDOW BEHAVIOR / SIZE / OPACITY
--------------------------------
Tray -> Window behavior, or the remote's overflow menu, has:
  Video always on top            independent from the bar
  Control bar always on top      same as the pin button
  Control bar size               80 / 100 / 120 / 140%
  Control bar opacity            100 / 90 / 80 / 70 / 60 / 50%
  Video opacity                  100 / 90 / 80 / 70 / 60 / 50%

The tray menu also has Bring control bar forward if it is buried behind other
windows.

Resize the bar by dragging its bottom-right grip, or hold Ctrl and use the
mouse wheel. Both windows start as normal windows. With topmost off, click a
visible part of the bar to raise it above the video without taking focus.

Video opacity uses the layered-window path with mpv's Blt swapchain. If your
GPU driver ignores it, leave video opacity at 100% and use Hide on pause or
the audio-only video mode instead.

YOUTUBE BROWSER
---------------
Tray -> Browse YouTube, or the bars Browse button. A borderless window with
a dark toolbar and the real YouTube site. Clicking a video sends it to the
player and hides the browser. The loaded page is kept for fast reopening.
The X, Alt+F4 and the taskbar close button all hide it; the remote menu's
Close YouTube browser frees the session. Quitting YTFrame closes it completely.

Needs the Microsoft Edge WebView2 Runtime, which normally comes with Edge on
Windows 10/11. If it is missing, the browser window has a Get WebView2 button;
the player keeps working without it.

The page is created in a visible container and explicitly re-shown and resized
when the retained browser window is reopened, so it should not remain blank
white.

The profile is stored in browse-data\ next to the exe, so logins/cookies
survive restarts. Tray -> Clear YouTube browsing data removes it.
YouTube ads/tracking live in the browse window; the player stays ad-free.
Google may block sign-in inside embedded browsers; browsing still works
logged out.

OTHER CONTROLS
--------------
Ctrl+Alt+V          paste bar
Enter               play the pasted link
Ctrl+V              play the clipboard link in the video window
Drag                move the video window
Drag edges/corners  resize, keeping the video aspect
Double-click / F    fullscreen (Esc leaves it)
Space / Ctrl+Alt+Space   play or pause
Left / Right        seek 5 seconds
Ctrl+Alt+J / L      seek 10 seconds from anywhere
Up / Down / wheel   volume
M / Ctrl+Alt+M      mute
[ / ]               speed
PGUP / PGDN         chapters
Home                start from the beginning
Right-click video   mpv player menu
Right-click tray    app menu
Q                   quit while the video is focused

RESUME / QUALITY / SPONSORBLOCK
-------------------------------
Resume remembers VOD positions; never live, never within ~20s of the end, and
a &t= timestamp wins. Tray toggle; Home starts over.
Quality: Best / 1080p / 720p / 480p; reloads at the same spot.
SponsorBlock skips sponsor, self-promo and interaction segments on YouTube
VODs. Only the video ID is sent to sponsor.ajay.app.

LIVE STREAMS
------------
Live links work the same way. Dropped streams are retried; scheduled streams
that have not started show a friendly message. Live latency is whatever
YouTube serves (usually 10-20 seconds behind the live edge).

IF A VIDEO STOPS WORKING
------------------------
Right-click the tray icon and choose "Update yt-dlp". It downloads the official
unpacked resolver, verifies its release SHA-256 checksum and installs it
side by side. The next video uses the new version. No admin rights needed.

FILES
-----
YTFrame.exe    the app
mpv.exe        playback engine
yt-dlp.exe     link resolver
settings.json  created on first run
history.json   resume positions and titles
browse-data\   WebView2 profile (logins/cookies)
ytframe.log    app log
mpv.log        player log

Delete the folder to uninstall.

NOT SUPPORTED
-------------
DRM'd YouTube Movies, age-restricted videos, members-only videos, and videos
that require a YouTube sign-in in the player. Google sign-in inside the
browse window may be blocked.

LICENCES
--------
YTFrame is MIT. mpv is GPLv2+; yt-dlp is Unlicense; go-webview2 is MIT.
See LICENSES.txt and LICENSE.GPL-2.0. Versions and source links are in
SOURCES.txt.

WHAT IS NEW IN 1.3
------------------
A redesigned launcher with a Browse YouTube button and Tab/Shift+Tab navigation.
A clearer remote with the current title, larger controls and hover hints.
Fixes for timestamp reloads, slider drags, monitor restoration, DPI changes
and read-only program folders. Up/Down now adjust volume as documented.

CODE SIGNING POLICY
-------------------
This build is unsigned. See CODE_SIGNING.txt for the release policy and
free open-source signing setup. Signing provider approval is still required.

STARTUP AND PERFORMANCE
-----------------------
The player warms while the launcher is visible. The bundled resolver lives
in resolver/ so Python is unpacked once, rather than on every video load.
Recent videos can be resumed from the launcher. Loading can be canceled.
Idle and paused controls redraw only when their displayed state changes.
Windows builds include RESOLVER_BENCHMARK.json and PLAYBACK_BENCHMARK.json.
These measure resolver startup and local playback; network times vary.
