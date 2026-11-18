YTFrame 1.1.0
=============

A borderless, video-only YouTube window.

QUICK START
-----------
1. Extract this folder anywhere (Desktop, USB stick, ...).
2. Run YTFrame.exe.
3. Paste a link and press Enter.

No installer, no registry, no admin rights. Settings, history and logs are
written next to YTFrame.exe. If the folder is read-only they go to
%LOCALAPPDATA%\YTFrame.

Windows SmartScreen will warn because the exe is unsigned:
click "More info" -> "Run anyway".

CONTROLS
--------
Hover the bottom of the video   control bar (play, seek, volume, fullscreen)
Ctrl+Alt+V                      open the paste bar from anywhere
Enter                           play the pasted link
Ctrl+V                          in the video window: play the clipboard link
Drag                            move the window (drag the video itself)
Drag edges/corners              resize, keeping the video's aspect ratio
Double-click / F                fullscreen (Esc leaves it)
Space / Ctrl+Alt+Space          play or pause
Left / Right                    seek 5 seconds
Ctrl+Alt+J / Ctrl+Alt+L         seek 10 seconds from anywhere
Up / Down / wheel               volume
M / Ctrl+Alt+M                  mute
[ / ]                           speed
PGUP / PGDN                     chapters
Home                            start from the beginning
Right-click video               mpv's player menu
Right-click tray                app menu
Q                               quit while the video is focused

RESUME
------
Normal videos longer than a minute resume where you left off. Live streams
never resume. A timestamp in the link (&t=) wins. Tray -> Resume playback
turns it off; Home starts from the beginning.

QUALITY
-------
Tray -> Quality: Best, 1080p, 720p, 480p. Switching while a video plays
reloads at the same timestamp (brief black flash). The choice is remembered.

SPONSORBLOCK
------------
Tray -> SponsorBlock skips sponsor, self-promo and interaction segments on
YouTube videos. Only the video ID is sent to sponsor.ajay.app. Not every video
has segments, and live streams/playlists are skipped.

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
history.json   resume positions and titles
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
