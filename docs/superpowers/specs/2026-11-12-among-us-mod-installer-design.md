# Among Us Mod Installer & Launcher — Design

Date: 2026-11-12
Status: Draft for review
Name: **CleanInstaller** [Owner]. Repo: new public repo `Kiirb/CleanInstaller`, branch `main` [Designer's call — owner to confirm]. The name, repo and branch are fixed from the first release, because the manifest URL, folder names and shortcut appid all depend on them.

This is a ground-up remake of [Kiirb/CInstaller](https://github.com/Kiirb/CInstaller), a C#/WPF tool that set up a modded Among Us install for a friend group. The remake can use any language or structure. It only has to do the same job, done better.

Marking used below: **[Owner]** = decided by the project owner. **[Designer's call]** = chosen during design, open to change. Anything else is left to the builder.

---

## 1. Intent

A small friend group plays modded Among Us together. Modded Among Us only works when everyone has exactly the same mod setup (mod pack + plugins, same versions). Most of the friends aren't technical. They should:

1. download one exe and run it once, and
2. from then on press Play on an "Among Us Modded" entry in Steam and always end up with the right setup, without thinking about it.

The owner wants to keep the mod setup current **without ever editing version numbers by hand**.

The previous version proved the idea works but had real problems:
- It always pulled the *latest* release of each mod at launch time, so friends could end up on different versions.
- A GitHub token was embedded in the exe.
- Windows SmartScreen and Defender flagged it.
- It had several bugs: it crashed on a first install because it deleted a folder that didn't exist; removing a Steam shortcut could clobber another shortcut; it continued after "game not installed".
- It swallowed errors silently and kept no log.
- It produced a ~60–70 MB exe.

## 2. Goals

- **One exe, two modes [Owner]:** installer (no arguments) and launcher (`--launch`, used by the Steam shortcut).
- **Identical setups for everyone:** every client installs exactly what one manifest says, verified by SHA-256.
- **Zero-maintenance updates [Owner]:** a scheduled GitHub Actions workflow resolves new mod releases into the manifest automatically.
- **Not flagged as suspicious by Windows [Owner]:** behaviour, build and signing are all chosen to avoid antivirus heuristics.
- **Painless for non-technical users [Owner]:** few clicks, clear German messages, no admin rights.
- **Windows first. Linux later [Owner]:** used by the owner and one friend, so it's a second phase. The architecture must allow it.
- **Single exe, no runtime dependency [Owner].**
- **Joke mode in the installer, switched remotely through the manifest [Owner]** (see §9).

## 3. Non-goals

- **Detecting whether a mod supports the current Among Us version [Owner].** The owner handles this manually, using pins if needed.
- **Per-user mod selection, profiles, or multiple mod packs.** One shared setup for the group.
- **Distributing mod settings/config [Owner]:** settings stay personal.
- **Holding back updates during a game night [Designer's call]:** a new mod release reaches each friend at their next launch. This is accepted as the price of zero maintenance. A broken release is handled by pinning it in `sources.json`.
- **Epic Games / Microsoft Store versions of Among Us.** Steam only.
- **A GUI for editing sources or the manifest.** Owner edits JSON on GitHub.
- **Linux in the first release** (phase 2, §12).

## 4. Anti-goals (failure even if it "works")

- Anything that makes Defender or other antivirus flag or quarantine the exe:
  - hidden PowerShell or `cmd` processes
  - packers (UPX etc.)
  - downloading and executing arbitrary code
  - suspicious strings in the binary
- Friends ending up on different mod versions.
- Any secret or token shipped in the exe.
- Clients calling the GitHub API (rate limits).
- Silent failure: when something breaks, the user sees a real German error message and there is a log the owner can ask for.
- Touching the vanilla Among Us install.
- Requiring admin rights.

## 5. Architecture overview

```
GitHub repo (public, open source)
├── Go source code
├── sources.json        ← hand-edited by owner: which mods, asset patterns, pins, removals
├── manifest.json       ← mods section written by workflow; "fun" block hand-edited
├── assets/steam/       ← Steam artwork (also embedded in exe)
└── .github/workflows/
    ├── manifest.yml    ← scheduled: sources.json → manifest.json
    └── release.yml     ← build (+ later: SignPath signing) → GitHub Release

Client PC
├── %LOCALAPPDATA%\CleanInstaller\        ← exe, state.json, logs\, download cache
├── <SteamLibrary>\steamapps\common\Among Us\          ← vanilla, never touched
└── <SteamLibrary>\steamapps\common\Among Us Modded\   ← game copy + mod pack + plugins
```

Clients make **no GitHub API calls**. They fetch only:
- `manifest.json` from `raw.githubusercontent.com`
- release asset files through `browser_download_url` links listed in the manifest

**Language: Go [Owner].**

## 6. Repo files

### 6.1 `sources.json` (hand-edited, rarely)

```json
{
  "modpack": { "repo": "AU-Avengers/TOU-Mira", "assetPattern": "-steam-itch.zip" },
  "plugins": [
    { "id": "SomePlugin", "repo": "owner/SomePlugin", "assetPattern": ".dll", "pin": null }
  ],
  "launcher": { "repo": "Kiirb/CleanInstaller", "assetPattern": ".exe" },
  "remove": ["BepInEx/plugins/AUnlocker.dll"]
}
```

- `assetPattern` is a substring match on the release asset name. It's the same rule as the old version. Exactly one asset must match, or the workflow fails for that entry and keeps the previous manifest value.
- `pin` (optional, any entry): a release tag to hold at instead of following the latest release.
- `remove`: paths, relative to the modded folder, that must not exist after sync. This replaces the old hardcoded `AUnlocker.dll` deletion.
- **[Designer's call]** Only non-draft, non-prerelease releases count as "latest". The old tool used `releases[0]`, which can be a prerelease.
- The old tool's plugin list lived in a gist. It moves into `sources.json`. The builder should copy the current plugin entries from the gist referenced in the old `RemoteManager.cs` (`https://gist.github.com/Kiirb/b96f6b5f2268f239fc387222aa3795be/raw`) as the starting list.

### 6.2 `manifest.json`

```json
{
  "schema": 1,
  "generatedAt": "2026-11-12T10:00:00Z",
  "launcher": { "version": "1.0.0", "url": "...", "sha256": "...", "size": 0 },
  "modpack":  { "tag": "v5.1.0", "url": "...", "sha256": "...", "size": 0 },
  "plugins": [
    { "id": "SomePlugin", "tag": "v1.2.3", "fileName": "SomePlugin.dll", "url": "...", "sha256": "...", "size": 0 }
  ],
  "remove": ["BepInEx/plugins/AUnlocker.dll"],
  "fun": {
    "jokeText": false,
    "chaosBar": false,
    "jokeLines": ["Kreditkarteninformationen abziehen", "IP-Adresse leaken", "Discord-Account-Token klauen", "67 Passwörter an Server senden"]
  }
}
```

- **[Owner]** The `fun` block lives in `manifest.json` itself and is edited by hand. **The workflow must preserve it byte-for-byte**: it only rewrites `generatedAt`, `launcher`, `modpack`, `plugins`, `remove`.
- Field names in `fun` are provisional **[Owner: "name pending"]**.
- Clients must ignore unknown fields. If `schema` is newer than they understand, they treat the manifest as unreachable and keep the last good one (§7.4).

## 7. Manifest workflow (`manifest.yml`)

- **Triggers [Designer's call]:**
  - schedule every 3 hours
  - push to `sources.json`
  - manual `workflow_dispatch`
- **Code:** runs the same Go program in a `build-manifest` mode, so GitHub and download logic exists once.
- **Auth:** the workflow's built-in `GITHUB_TOKEN`. No personal access token needed.
- **For each source:**
  1. Find the target release (pinned tag, or latest stable).
  2. If tag and asset are unchanged from the current manifest, keep the entry as is.
  3. Otherwise download the asset once, compute SHA-256 and size, and write the entry.
- **Commits:** only when the content changed (ignoring `generatedAt`). The commit message lists what changed.
- **Failure handling:** a source that fails (no matching asset, API error) keeps its previous entry. The run is marked failed so GitHub emails the owner. Other sources still update.
- Commits to `manifest.json` must not race with the owner's hand edits of `fun`. The workflow reads, modifies and pushes in one go, and retries on a non-fast-forward.

### 7.4 Client behaviour when the manifest is unreachable

- The last successfully fetched manifest is cached in `%LOCALAPPDATA%\CleanInstaller\`.
- **Launcher:** offline or GitHub down means skip sync and self-update, and launch with what's on disk.
- **Installer:** needs a manifest. If there's no cached one, show a clear German error ("Keine Internetverbindung…").
- **Joke mode is off whenever the live manifest couldn't be fetched [Designer's call].**

## 8. Client behaviour

### 8.1 Mode selection [Owner + Designer's call]

- No arguments: **installer**.
- `--launch`: **launcher**.
- The Steam shortcut always passes `--launch`.
- Mode is never inferred from files on disk.

### 8.2 Locations [Owner]

- **Exe, state, cache, logs:** `%LOCALAPPDATA%\CleanInstaller\`.
- **Modded game:** `<library containing Among Us>\steamapps\common\Among Us Modded\`. It holds only the game copy, the mod pack and plugins, so it can be wiped and rebuilt at any time.

### 8.3 Shared "sync" step (used by both modes)

Goal: make `Among Us Modded` match the manifest.

1. **Locate Steam and the game:**
   - `HKCU\Software\Valve\Steam\SteamPath`
   - parse `steamapps/libraryfolders.vdf` with a real text-VDF parser, not regex
   - find the library holding app 945360
2. **Game version:** read `buildid` from that library's `appmanifest_945360.acf`.
   - If `Among Us Modded` is missing, or its recorded buildid (in `state.json`) differs, **delete and rebuild the whole modded folder**:
     1. move `BepInEx/config/` aside, if present
     2. copy vanilla minus BepInEx/doorstop leftovers (same exclusion list as the old tool)
     3. extract the mod pack
     4. restore `BepInEx/config/`, keeping the user's existing files over any the mod pack ships
     5. add plugins

     **Settings stay personal [Owner]:** the manifest never distributes config files, and rebuilds never reset a user's `BepInEx/config/`.
3. **Mod pack:** if the installed mod pack's sha256 (recorded in state) differs from the manifest, rebuild as in step 2. The mod pack overwrites game files, so there's no partial update.
4. **Plugins:** in `BepInEx/plugins/`, every manifest plugin must be present with matching SHA-256 (hash on every sync; files are small). Download whatever is missing or wrong. **Delete any other file** in that folder, as the old tool did.
5. **Remove list:** delete listed paths if present.
6. **Write `state.json`:** buildid, modpack sha, plugin list.

Details:
- **Download cache [Designer's call]:** downloads go into `%LOCALAPPDATA%\CleanInstaller\cache\` keyed by SHA-256, so rebuilds after a game update don't re-download. Keep only files referenced by the current manifest.
- **Integrity:** every download is verified against its SHA-256 before use. On mismatch, retry once, then error.
- **Zip extraction:** strip a single top-level folder if present (old behaviour). Guard against zip-slip paths.
- **Game launch prerequisite:** `steam_appid.txt` (945360) must exist in the modded folder so the game doesn't relaunch through Steam as vanilla. The builder should check whether the mod pack already provides it and create it if not.

### 8.4 Installer mode

1. Fetch the manifest. Show a first screen that says, in German, what will happen:
   - create `Among Us Modded`
   - add a Steam shortcut
   - copy the program to `%LOCALAPPDATA%`
   
   One "Installieren" button. Listing these changes up front is required by SignPath (§11).
2. **If already installed** (state exists), offer **Reparieren / Neu installieren** and **Deinstallieren** instead.
   **Old installs (§8.8):** if an old CInstaller install is detected, the first screen also says it will be replaced, and the cleanup runs before sync.
3. **Checks:**
   - Steam not found: clear error, stop.
   - Among Us not installed: offer to open `steam://install/945360`, then **stop** with "install the game, then run this again". Do not continue (this was an old bug).
4. Run sync, with a progress window.
5. Copy itself to `%LOCALAPPDATA%\CleanInstaller\CleanInstaller.exe`.
6. **Steam shortcut (§10).**
7. "Fertig" screen with a button that starts Steam if it's closed.

### 8.5 Launcher mode

1. **Self-update:**
   - If `manifest.launcher.version` is newer than the running version: download, verify, rename the running exe to `.old`, move the new one in place, and restart with the same arguments.
   - On every start, delete a leftover `.old` if present.
   - **No PowerShell or helper processes.**
2. **Run sync.** A window appears only if there is work to do (download or rebuild). Otherwise nothing is shown.
3. **Start the game:** run `Among Us Modded\Among Us.exe` with its folder as the working directory.
4. **[Designer's call]** The launcher waits for the game process to exit before exiting itself, so Steam shows the user as playing for the whole session. The builder should confirm this behaves well with the Steam overlay.

Joke mode never applies to the launcher **[Owner]**.

### 8.6 Uninstall

Removes, in this order:
1. the Steam shortcut and its artwork (Steam handling as in §10)
2. `Among Us Modded`
3. `%LOCALAPPDATA%\CleanInstaller\`

The vanilla game is never touched. If the uninstaller is running from inside `%LOCALAPPDATA%\CleanInstaller\`, it can't delete its own exe. The builder picks a clean approach, without hidden scripts. Telling the user "you can delete this file" is acceptable.

### 8.8 Migrating old CInstaller installs [Owner: must be handled]

Some friends still have the old C# CInstaller. From its source ([Kiirb/CInstaller](https://github.com/Kiirb/CInstaller)), it left behind:

- **Folder:** `<steamCommon>\Among Us Modded\`, containing the game copy plus mods, `CInstaller.exe` and `CInstallerConfig.json`.
- **Steam shortcut:** in `userdata/<id>/config/shortcuts.vdf`, with hardcoded `appid` `-305070`, `AppName` "Among Us Modded", `Exe` pointing at `...\Among Us Modded\CInstaller.exe`, and `StartDir` = that folder. It was written only for the most-recently-logged-in user at the time.
- **Artwork:** in `userdata/<id>/config/grid/`, the files `4294662226p.png`, `4294662226_hero.png`, `4294662226_logo.png`, `4294662226_icon.png` and `4294662226.json`.

**Detection and cleanup** (installer, every run):
- **Every Steam user**, not just the selected one: remove shortcut entries whose `Exe` points at a `CInstaller.exe`, or whose `appid` is `-305070` and `AppName` is "Among Us Modded". Then renumber the entries. Delete the `4294662226*` artwork files. All Steam-file writes follow the rules in §10.
- **The folder:** an `Among Us Modded` folder containing `CInstallerConfig.json`, or with no `state.json` of ours, is treated as foreign. It gets deleted and rebuilt, keeping its `BepInEx/config/` as in §8.3. The new folder has the same name and place, so nothing else changes for the user.
- **Logging:** each removal is logged.

**Getting old users onto the new tool:**
- **Default:** friends download and run the new installer once, and the cleanup above does the rest.
- **Optional [Designer's call; builder decides whether it works]:** the old tool self-updates from `Kiirb/CInstaller` releases. It takes the first `.exe` asset of the newest release whose version is greater than 0.1.4.3, replaces itself, and starts the new exe without arguments. Publishing the new CleanInstaller exe as a final `Kiirb/CInstaller` release (for example `0.2.0.0`) would turn old launchers into the new installer automatically.
  - The new installer must cope with being started from inside the old `Among Us Modded` folder that it is about to delete. For example, it copies itself to `%LOCALAPPDATA%` first and continues from there.
  - Whether the old tool's embedded GitHub token still works decides if its update check succeeds at all. If it doesn't, this path silently does nothing, and the default path still applies.

### 8.7 Errors and logging

- **Every failure** shows a German message saying what failed in plain words, plus a button that opens the log folder.
- **Logs:**
  - location: `%LOCALAPPDATA%\CleanInstaller\logs\`
  - one file per run, keep the last ~10
  - contents: timestamps, paths, URLs, hashes, errors
- **No empty `catch`.**

## 9. Joke mode (installer only) [Owner]

Both switches are controlled remotely through the `fun` block in `manifest.json`, so the owner can turn them off when a friend gets stuck, then ask the friend to rerun the installer and see real messages.

- **`jokeText: true`:**
  - The status text above the progress bar shows random lines from `jokeLines`, changing every ~1.5–3 s.
  - The **progress bar stays accurate**, so the real duration can still be judged.
- **`chaosBar: true`:**
  - The bar shows nonsense: random speed changes, stalls, occasional backwards jumps.
  - The real work continues unchanged underneath.
  - **[Designer's call]** When the real work finishes, the bar jumps to 100% at once. "Fertig" is never faked, and the chaos never delays completion.
- **The two switches are independent.**
- **Errors always show the real message**, even in joke mode **[Designer's call]**.
- **Joke strings are never compiled into the exe.** They come only from the manifest. Text like "Discord token" in a binary next to download code is an antivirus heuristic trigger, and a SignPath complaint risk.

## 10. Steam integration

- **Which account:** user folders under `Steam/userdata/` are matched against `config/loginusers.vdf`.
  - One user: use it.
  - Several users: the installer asks which account (show persona names).
- **Steam must not be running while `shortcuts.vdf` is written.** Steam rewrites the file when it exits, which would drop the new shortcut.
  - If Steam is running, tell the user and offer to close it (`steam.exe -shutdown`, wait for exit, with a timeout).
  - Write the file, then offer to start Steam again.
- **`shortcuts.vdf`** (binary VDF): parse fully, remove any existing entry whose `Exe` or `StartDir` points at our exe or our folder, **renumber entries contiguously**, then append ours. This fixes the old index-collision bug.
- **Shortcut fields:**
  - `AppName` = "Among Us Modded"
  - `Exe` = `"%LOCALAPPDATA%\CleanInstaller\CleanInstaller.exe"` (quoted)
  - `StartDir` = the `CleanInstaller` folder (quoted)
  - `LaunchOptions` = `--launch`
  - `icon` = the icon file path
  - `appid` = computed with Steam's scheme: CRC32 of exe + name, high bit set. Do not hardcode it like the old tool did.
- **Artwork** (grid, hero, logo, icon, plus logo position JSON):
  - written to `userdata/<id>/config/grid/` using the computed appid
  - **[Designer's call]** image files are embedded in the exe with `go:embed`, so they don't depend on SteamGridDB links staying alive
  - the owner supplies the images, and can start from the four SteamGridDB images the old tool used

## 11. Build, release, signing, antivirus hygiene

- **License and repo:** public GitHub repo, OSI license. **[Designer's call] MIT.**
- **`release.yml`:**
  - runs on a version tag
  - builds `windows/amd64` with `-trimpath`
  - embeds a version-info resource (product name, version, publisher, description), the icon and an application manifest (`asInvoker`, DPI-aware), e.g. via `go-winres`
  - **no UPX, no stripping tricks**
  - publishes the exe to a GitHub Release
- **The manifest workflow picks up new launcher releases** through the `launcher` source, so clients self-update automatically.
- **Signing [Owner]:** apply to **SignPath Foundation** (free OSS signing) once a working release exists, which is one of their requirements. Until approved, releases are unsigned and SmartScreen shows "More info → Run anyway" once, at first install.
  - The README's download section shows this step with a screenshot.
  - Launcher self-updates are fetched by the program, not a browser, so they don't carry Mark-of-the-Web and don't trigger SmartScreen.
- **SignPath conditions to meet:**
  - README "Code signing policy" section with their required wording ("Free code signing provided by SignPath.io, certificate by SignPath Foundation")
  - team roles listed
  - privacy statement (e.g. "only downloads from GitHub, sends no data")
  - project description on the download page
  - install-time disclosure of system changes (§8.4) and an uninstall option (§8.6)
  - MFA on GitHub and SignPath
  - every release manually approved for signing
  - builds reproducible from the repo
- **Behaviour rules (antivirus):**
  - no hidden processes
  - no PowerShell
  - no writing outside our folder, the modded folder and Steam's `userdata/<id>/config/`
  - always a visible window during installs
  - no admin rights
- **If a release is flagged anyway:** submit it to Microsoft's false-positive form. Document this in a maintainer note in the repo.
- BepInEx's `winhttp.dll` may itself be flagged by some antivirus tools. If a downloaded or extracted file disappears or can't be written, the error message should say antivirus may have removed it.

## 12. Linux (phase 2) [Owner]

Not in the first release, but the code must keep platform specifics behind a small interface:
- find Steam
- find libraries
- create the shortcut
- launch the game
- locate the app data folder

Known phase-2 work:
- **Steam locations:** `~/.steam/steam`, `~/.local/share/Steam`, Flatpak `~/.var/app/com.valvesoftware.Steam/.local/share/Steam`.
- **The game runs through Proton.** BepInEx needs `WINEDLLOVERRIDES="winhttp=n,b"`.
- **Launching the modded copy through Proton** from a native launcher is the riskiest part and needs testing on real Linux and Steam.
- **No SmartScreen, Defender or signing concerns.**

The UI library must work on Linux too.

## 13. UI

- **Language:** all user-facing text in German **[Designer's call, matching the old tool]**. Code and logs in English.
- **Look:**
  - one small window
  - title, status line, progress bar, buttons as needed by each screen
  - dark theme like the old tool
- **UI library (builder's choice), with these constraints:**
  - single static exe
  - **no cgo for the Windows build** (keeps cross-compiling and CI simple)
  - no runtime dependency such as WebView2 being required
  - must also run on Linux later
  - Gio is a likely fit; the builder verifies

## 14. Testing

- **Unit tests:**
  - text VDF and binary VDF round-trip (use real-world sample files)
  - shortcut appid calculation
  - manifest merge that preserves `fun`
  - sync planning (given disk state + manifest, which actions?)
  - zip extraction including zip-slip
  - asset pattern matching
- **Manual test checklist on a clean Windows 11 VM:**
  - fresh install
  - second run (repair)
  - launch with no changes
  - launch after a plugin update
  - launch after a simulated game update (change buildid)
  - offline launch
  - multiple Steam users
  - Steam running during install
  - uninstall
  - joke text and chaos bar
  - a PC with an old CInstaller install: one Steam entry afterwards, old artwork gone, `BepInEx/config` kept
  - optional old-tool auto-update path (§8.8)
- **Unit tests also cover:**
  - detecting old shortcut entries and artwork
  - preserving `BepInEx/config` across a rebuild

## 15. Left to the builder

- Go package layout, UI library (within §13 constraints), VDF libraries vs own code.
- Exact window layouts and wording of German messages.
- Retry and timeout values, log format.
- How the uninstaller deals with its own exe (§8.6), within the no-hidden-scripts rule.
- Exact joke-line timing and chaos-bar motion.
