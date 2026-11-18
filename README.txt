YTFrame 1.0.0
=============

A borderless, video-only YouTube window.

QUICK START
-----------
1. Extract this folder anywhere (Desktop, USB stick, ...).
2. Run YTFrame.exe.
3. Paste a link and press Enter.

No installer, no registry, no admin rights. Settings and logs are written next
to YTFrame.exe. If the folder is read-only they go to %LOCALAPPDATA%\YTFrame.

Windows SmartScreen will warn because the exe is unsigned:
click "More info" -> "Run anyway".

CONTROLS
--------
Ctrl+Alt+V     open the paste bar from anywhere
Enter          play the pasted link
Ctrl+V         in the video window: replace with the clipboard link
Drag           move the window (drag the video itself)
Drag edges     resize, keeping the video's aspect ratio
Double-click   fullscreen (Esc leaves it)
Space          pause
Left / Right   seek
Up / Down      volume
M              mute
Right-click    mpv's player menu
Tray icon      paste, pause, mute, fullscreen, always on top,
               update yt-dlp, open log, about, exit
Q              quit while the video is focused

LIVE STREAMS
------------
Live links work the same way. Dropped streams are retried; scheduled streams
that have not started show a friendly message. Live latency is whatever
YouTube serves (usually 10-20 seconds behind the live edge).

IF A VIDEO STOPS WORKING
------------------------
YouTube changes things regularly. Right-click the tray icon and choose
"Update yt-dlp". It updates the bundled yt-dlp.exe in place (no admin needed).

FILES
-----
YTFrame.exe    the app
mpv.exe        playback engine
yt-dlp.exe     link resolver
settings.json  created on first run
ytframe.log    app log
mpv.log        player log

Delete the folder to uninstall.

NOT SUPPORTED
-------------
DRM'd YouTube Movies, age-restricted videos, members-only videos, and
videos that require a YouTube sign-in.

LICENCES
--------
YTFrame is MIT. mpv is GPLv2+; yt-dlp is Unlicense. See LICENSES.txt and
LICENSE.GPL-2.0. Exact versions and source links are in SOURCES.txt.
