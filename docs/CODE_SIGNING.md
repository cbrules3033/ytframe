# Code signing policy

YTFrame builds are currently **unsigned**. No certificate has been issued to this project, no signing service is configured, and no application has been submitted on the maintainer's behalf. A checksum verifies a download's integrity; it is not an Authenticode signature and does not remove Windows warnings.

## Release policy

- Maintainer: [cbrules3033](https://github.com/cbrules3033), responsible for source review, releases and approving any future signing requests.
- Build source: the public [YTFrame repository](https://github.com/cbrules3033/ytframe). The Windows workflow tests the app, builds from its checked-out commit and packages dependencies verified against pinned SHA-256 hashes.
- Signing scope: YTFrame.exe built from this repository. Bundled mpv.exe and yt-dlp.exe retain their upstream identities; this project does not sign upstream executables as its own work.
- Signing credentials must be held by the signing provider or secure hardware, and never committed to the repository. Production signing requires the maintainer's approval of the exact build artifact.
- A release may be called signed only after its Authenticode signature and certificate chain have been verified. Self-signed certificates are not distributed as trusted signatures.

## Free open-source signing

[SignPath Foundation](https://signpath.org/) offers free code signing for eligible open-source projects. Its [conditions](https://signpath.org/terms.html) require project review, MFA, designated signing roles and a documented signing policy. Eligibility and acceptance are determined by the Foundation.

The maintainer can apply at [signpath.org/apply](https://signpath.org/apply). Submission requires the maintainer's account, contact details and acceptance of the provider's terms. This document prepares the project information without claiming approval.

Application information:

- Project: YTFrame
- Repository: https://github.com/cbrules3033/ytframe
- Downloads: https://github.com/cbrules3033/ytframe/releases
- License: MIT for YTFrame; GPLv2+ for bundled mpv; Unlicense for yt-dlp. Dependency notices and exact source links are in LICENSES.txt and SOURCES.txt.
- Purpose: a portable Windows video-only player for YouTube and local media, with a separate floating remote and optional YouTube browser.
- Target artifact: Windows amd64 YTFrame.exe, compiled by Go with CGO disabled. The workflow also packages upstream binaries, which are outside the requested signing scope.
- Dependency disclosure: a patched MIT go-webview2 wrapper and its Microsoft WebView2Loader DLL are embedded in YTFrame.exe. The Microsoft Edge WebView2 Runtime is a separately installed system component. Ask the Foundation to confirm eligibility for the embedded loader before integration.
- Suggested role assignment for the maintainer to confirm: cbrules3033 as author, reviewer and signing approver.

After approval, configure the SignPath GitHub integration and a project artifact definition matching only YTFrame.exe. Submit the workflow's exact artifact for signing, require production approval, verify the returned signature on Windows with `Get-AuthenticodeSignature`, then package that returned executable and regenerate the ZIP checksum. Do not rebuild or modify the executable after signing.

## Network and privacy

Playback contacts the supplied media provider. Browse loads YouTube in WebView2; YouTube and Google process browser traffic under their own policies. The browser profile, including cookies and sign-ins, is stored locally in browse-data/. SponsorBlock sends the YouTube video ID to sponsor.ajay.app when enabled. Updating yt-dlp contacts GitHub. YTFrame does not operate a telemetry service; local history, settings and logs remain on the user's machine.
