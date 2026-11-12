# CleanInstaller Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build CleanInstaller, a single Windows exe that installs a modded copy of Among Us with a Steam shortcut and, launched from that shortcut, keeps the mods in sync with a manifest that a GitHub workflow keeps up to date.

**Architecture:**
- **One Go module, two binaries.**
  - `cmd/cleaninstaller` is the shipped exe: installer mode with no arguments, launcher mode with `--launch`.
  - `cmd/manifestgen` runs only in GitHub Actions to turn `sources.json` into `manifest.json`.
- **Logic lives in small `internal/` packages** (vdf, manifest, download, fsutil, steam, syncer, migrate, …). They are shared by both binaries and tested without a UI or a real Steam install.
- **Platform specifics** (registry, processes, paths) sit behind one `platform.Platform` interface, so Linux can be added later.

**Tech Stack:**
- Go: current stable, with the `go` directive pinned to the installed version.
- `gioui.org` for the UI (no cgo on Windows).
- `golang.org/x/sys/windows` and `golang.org/x/sys/windows/registry`.
- `golang.org/x/mod/semver`.
- `github.com/tc-hib/go-winres` for exe resources.
- GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-11-12-among-us-mod-installer-design.md`. Read it first. Section references (§) below point into it.

## Global Constraints

**Names and locations**
- Module path `github.com/Kiirb/CleanInstaller`. Repo `Kiirb/CleanInstaller`, branch `main`.
- Manifest URL, compiled in: `https://raw.githubusercontent.com/Kiirb/CleanInstaller/main/manifest.json`.
- Exe name `CleanInstaller.exe`. Data dir `%LOCALAPPDATA%\CleanInstaller\`, containing the exe, `state.json`, `manifest.cache.json`, `cache\` and `logs\`.

**Among Us and Steam constants**
- Steam app id `945360`, game folder `Among Us`, game exe `Among Us.exe`.
- Modded folder `Among Us Modded`, next to the vanilla folder. The Steam shortcut `AppName` is `Among Us Modded`.
- SteamID64 base `76561197960265728`.
- Vanilla copy excludes:
  - root files `winhttp.dll`, `doorstop_config.ini`, `changelog.txt`, `.doorstop_version`
  - root directory `BepInEx`

**Old CInstaller install**
- Exe `CInstaller.exe`, config `CInstallerConfig.json`.
- Shortcut `appid` `-305070`.
- Artwork prefix `4294662226` (files `4294662226p.png`, `4294662226_hero.png`, `4294662226_logo.png`, `4294662226_icon.png`, `4294662226.json`).

**Behaviour rules**
- No PowerShell, no `cmd`, no hidden processes, no UPX or packers, no admin rights.
- Write only to: the data dir, the modded folder, Steam `userdata/<id>/config/`, and `%TEMP%` (installer relaunch copy).
- The vanilla `Among Us` folder is never written.
- Clients never call `api.github.com`.
- All user-facing text in German. Code, comments and logs in English.
- Every download is verified by SHA-256 before use.
- No empty error handling: every error is logged and either shown to the user or deliberately ignored with a log line saying why.
- Joke strings never appear in the Go source or binary. They come only from `manifest.fun.jokeLines`.
- Joke mode only ever applies in installer mode.

**Workflow**
- Cron `0 */3 * * *`, plus a trigger on push to `sources.json`, plus `workflow_dispatch`.
- Only non-draft, non-prerelease releases count as "latest".

## Review Focus

1. **Library paths with escaped backslashes, spaces or umlauts** in `libraryfolders.vdf` (e.g. `"D:\\Spiele\\SteamLibrary"`): the path must come out as `D:\Spiele\SteamLibrary`. Test added in Task 1.
2. **Among Us Modded is running or a file is locked** during a rebuild (friend left the game open, antivirus holding a DLL): the old folder must stay intact and usable, and the user gets a clear message. The rebuild builds into `Among Us Modded.new` and swaps only at the end. A launcher started while the modded game is already running starts nothing new and exits. Tests added in Tasks 10 and 15.
3. **Interrupted download or full disk:** a half-written file is never used or kept as if valid. Downloads go to `*.part`, are verified, then renamed. Test added in Task 6.
4. **A Steam account with no `shortcuts.vdf` or no `config/grid` yet** (fresh account): the shortcut is created and nothing crashes. Test added in Task 9.
5. **A manifest entry whose `fileName` or `remove` path contains `..`, `/`, `\` or is absolute** (a typo or bad release asset name): it is rejected before anything is written. Test added in Task 3.

---

## File Structure

```
go.mod / go.sum / LICENSE (MIT) / .gitignore / README.md
sources.json                      hand-edited mod list (Task 5)
manifest.json                     generated + hand-edited "fun" block (Task 5)
assets/steam/                     grid.png hero.png logo.png icon.png logo.json (Task 9)
cmd/cleaninstaller/main.go        mode parsing, wiring (Tasks 0, 14, 15)
cmd/cleaninstaller/winres/        winres.json + icon (Task 16)
cmd/manifestgen/main.go           workflow tool (Task 4)
internal/vdf/text.go              text VDF parser (Task 1)
internal/vdf/binary.go            binary VDF read/write, ordered (Task 2)
internal/manifest/manifest.go     Manifest/Sources types, parse, validate, merge (Task 3)
internal/ghrel/ghrel.go           GitHub releases client, workflow-only (Task 4)
internal/manifestgen/build.go     sources + old manifest -> new manifest (Task 4)
internal/download/download.go     verified download + sha-keyed cache (Task 6)
internal/fsutil/copy.go           tree copy with excludes (Task 7)
internal/fsutil/zip.go            zip extract, strip root, zip-slip guard (Task 7)
internal/platform/platform.go     Platform interface (Task 8)
internal/platform/windows.go      Windows impl (Task 8)
internal/steam/locate.go          libraries, game dir, buildid (Task 8)
internal/steam/users.go           loginusers.vdf, userdata ids (Task 8)
internal/steam/shortcuts.go       shortcuts.vdf edit, appid, artwork (Task 9)
internal/syncer/state.go          state.json (Task 10)
internal/syncer/sync.go           plan + apply (Task 10)
internal/migrate/migrate.go       old CInstaller detection/cleanup (Task 11)
internal/progress/tracker.go      weighted progress tracker (Task 12)
internal/fun/fun.go               joke-line picker + chaos bar model (Task 12)
internal/applog/applog.go         file logging, keep 10 (Task 12)
internal/apperr/apperr.go         UserError with German message (Task 12)
internal/ui/ui.go                 UI interface (Task 12)
internal/ui/gioui.go              Gio implementation (Task 13)
internal/install/install.go       installer + repair + uninstall flow (Task 14)
internal/selfupdate/selfupdate.go (Task 15)
internal/launch/launch.go         launcher flow (Task 15)
.github/workflows/ci.yml          go test on windows-latest (Task 0)
.github/workflows/manifest.yml    (Task 5)
.github/workflows/release.yml     (Task 16)
docs/manual-test-checklist.md     (Task 16)
```

**[Planner's call]** The spec says the workflow runs "the same Go program in a build-manifest mode". This plan uses a separate `cmd/manifestgen` binary in the same module instead. All logic is still shared through `internal/`, and the shipped exe doesn't carry GitHub API code.

---

### Task 0: Repo scaffold, mode parsing, CI

**Files:**
- Create: `go.mod`, `LICENSE`, `.gitignore`, `cmd/cleaninstaller/main.go`, `cmd/cleaninstaller/mode.go`, `cmd/cleaninstaller/mode_test.go`, `.github/workflows/ci.yml`

**Interfaces:**
- Produces:
  - `type Mode int` with constants `ModeInstall`, `ModeLaunch`
  - `func parseMode(args []string) (Mode, error)`
  - `var version = "0.0.0-dev"`, overridden at build time via `-ldflags "-X main.version=1.2.3"`

- [ ] **Step 1: Prerequisite.** If `go version` fails, install Go with `winget install GoLang.Go` and open a new shell. Run `go mod init github.com/Kiirb/CleanInstaller`. Add an MIT `LICENSE` (copyright Kiirb). Add a `.gitignore` covering `*.exe`, `dist/` and `*.syso` (generated by winres; regenerated in CI).
- [ ] **Step 2: Write the failing test**

```go
func TestParseMode(t *testing.T) {
    cases := []struct{ args []string; want Mode; wantErr bool }{
        {nil, ModeInstall, false},
        {[]string{"--launch"}, ModeLaunch, false},
        {[]string{"--bogus"}, 0, true},
        {[]string{"--launch", "extra"}, 0, true},
    }
    // for each: got, err := parseMode(c.args); compare
}
```

- [ ] **Step 3: Run** `go test ./cmd/cleaninstaller/`. Expected: FAIL, `parseMode` undefined.
- [ ] **Step 4: Implement `parseMode`** in `mode.go`. `main.go` calls it with `os.Args[1:]`. For now it prints the mode and exits.
- [ ] **Step 5: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 6: Add `ci.yml`.** On push and pull request: `runs-on: windows-latest`, `actions/setup-go` with `go-version-file: go.mod`, then `go vet ./...` and `go test ./...`.
- [ ] **Step 7: Commit** with message `chore: scaffold module, mode parsing, CI`.

---

### Task 1: Text VDF parser

**Files:**
- Create: `internal/vdf/text.go`, `internal/vdf/text_test.go`, `internal/vdf/testdata/libraryfolders.vdf`, `internal/vdf/testdata/loginusers.vdf`, `internal/vdf/testdata/appmanifest_945360.acf`

**Interfaces:**
- Produces:
  - `type KV struct{ Key string; Value string; Children []*KV }`. A node is either a leaf (`Value`) or a block (`Children`).
  - `func ParseText(r io.Reader) (*KV, error)`, which returns a root with `Key == ""` holding the top-level nodes.
  - `func (k *KV) Child(key string) *KV`: case-insensitive first match, nil if absent.
  - `func (k *KV) Get(path ...string) (string, bool)`: walks children by key and returns the leaf value.

- [ ] **Step 1: Write testdata files** in real Steam format:
  - **`libraryfolders.vdf`** with two libraries:
    - `"0"`: path `C:\\Program Files (x86)\\Steam`, apps without 945360
    - `"1"`: path `D:\\Spiele\\Steam Bibliothek ä`, apps containing `"945360" "123"`
  - **`loginusers.vdf`** with two users, one `"MostRecent" "1"`.
  - **`appmanifest_945360.acf`** with `"buildid" "16000001"`.
- [ ] **Step 2: Write failing tests**

```go
func TestParseLibraryFolders(t *testing.T) {
    root := mustParse(t, "testdata/libraryfolders.vdf")
    lib1 := root.Child("libraryfolders").Child("1")
    p, _ := lib1.Get("path")
    if p != `D:\Spiele\Steam Bibliothek ä` { t.Fatalf("path %q", p) }  // Review Focus 1
    if lib1.Child("apps").Child("945360") == nil { t.Fatal("app missing") }
}
func TestParseEscapes(t *testing.T)   // `"k" "a\"b\\c"` -> a"b\c
func TestParseComments(t *testing.T)  // lines starting with // ignored
func TestParseUnterminated(t *testing.T) // `"a" {` -> error, no panic
```

- [ ] **Step 3: Run** `go test ./internal/vdf/`. Expected: FAIL.
- [ ] **Step 4: Implement `ParseText`.** It is a hand-written tokenizer:
  - Tokens are quoted strings (escapes `\\`, `\"`, `\n`, `\t`), `{`, and `}`.
  - Skip `//` comments.
  - Ignore `[$WIN32]`-style conditionals after a value.
- [ ] **Step 5: Run.** Expected: PASS.
- [ ] **Step 6: Commit** with message `feat(vdf): text VDF parser`.

---

### Task 2: Binary VDF (shortcuts.vdf) read/write

**Files:**
- Create: `internal/vdf/binary.go`, `internal/vdf/binary_test.go`, `internal/vdf/testdata/shortcuts.vdf` (hand-built in the test helper if no real sample is available)

**Interfaces:**
- Produces:
  - `type BNode struct{ Key string; Type byte; Str string; Int int32; U64 uint64; F32 uint32; Children []*BNode }`. Children are **ordered**.
  - Type constants: `TypeMap = 0x00`, `TypeString = 0x01`, `TypeInt32 = 0x02`, `TypeFloat32 = 0x03` (raw bits kept in `F32`), `TypeUint64 = 0x07`. The end marker is `0x08`.
  - `func ReadBinary(r io.Reader) (*BNode, error)` reads one top-level map (root `Key == ""`).
  - `func WriteBinary(w io.Writer, root *BNode) error`
  - `func (n *BNode) Child(key string) *BNode`: case-insensitive.

- [ ] **Step 1: Write failing tests**

```go
func TestBinaryRoundTrip(t *testing.T) {
    in := readFile(t, "testdata/shortcuts.vdf")
    root, err := ReadBinary(bytes.NewReader(in))
    var out bytes.Buffer; WriteBinary(&out, root)
    if !bytes.Equal(in, out.Bytes()) { t.Fatal("round trip changed bytes") }
}
func TestBinaryPreservesOrder(t *testing.T)   // keys "b","a","c" stay in that order
func TestBinaryUnknownType(t *testing.T)      // type 0x05 -> error, not panic
func TestBinaryTruncated(t *testing.T)        // cut file mid-string -> error
```

- [ ] **Step 2: Run** `go test ./internal/vdf/`. Expected: FAIL.
- [ ] **Step 3: Implement.** Keys and strings are NUL-terminated UTF-8, and integers are little-endian. The file is the root map's children followed by a final `0x08`. That structure matches the old `BinVdfReader`, except ordered and with float32 support.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** with message `feat(vdf): ordered binary VDF read/write`.

---

### Task 3: Manifest and sources types, validation, merge

**Files:**
- Create: `internal/manifest/manifest.go`, `internal/manifest/manifest_test.go`

**Interfaces:**
- Produces:

```go
const SchemaVersion = 1
type Asset struct {
    Version  string `json:"version,omitempty"`   // launcher only, semver without "v"
    Tag      string `json:"tag,omitempty"`
    FileName string `json:"fileName,omitempty"`  // plugins only
    ID       string `json:"id,omitempty"`        // plugins only
    URL      string `json:"url"`
    SHA256   string `json:"sha256"`              // lowercase hex
    Size     int64  `json:"size"`
}
type Fun struct {
    JokeText  bool     `json:"jokeText"`
    ChaosBar  bool     `json:"chaosBar"`
    JokeLines []string `json:"jokeLines"`
}
type Manifest struct {
    Schema      int             `json:"schema"`
    GeneratedAt time.Time       `json:"generatedAt"`
    Launcher    Asset           `json:"launcher"`
    Modpack     Asset           `json:"modpack"`
    Plugins     []Asset         `json:"plugins"`
    Remove      []string        `json:"remove"`
    FunRaw      json.RawMessage `json:"fun,omitempty"`
}
func (m *Manifest) Fun() Fun                 // decode FunRaw; zero Fun on absent or invalid
func Parse(b []byte) (*Manifest, error)       // errors: invalid JSON, Schema > SchemaVersion (ErrSchemaTooNew), Validate failure
func (m *Manifest) Validate() error
func Marshal(m *Manifest) ([]byte, error)     // 2-space indent, trailing newline, field order as struct

type Source struct {
    ID           string `json:"id"`
    Repo         string `json:"repo"`          // "owner/name"
    AssetPattern string `json:"assetPattern"`  // default ".dll" for plugins if empty
    Pin          string `json:"pin,omitempty"` // release tag
}
type Sources struct {
    Modpack  Source   `json:"modpack"`
    Plugins  []Source `json:"plugins"`
    Launcher Source   `json:"launcher"`
    Remove   []string `json:"remove"`
}
func ParseSources(b []byte) (*Sources, error) // errors: duplicate plugin id, empty repo, bad repo format
var ErrSchemaTooNew = errors.New("manifest schema too new")
```

- [ ] **Step 1: Write failing tests**

```go
func TestValidateRejectsBadPaths(t *testing.T) {  // Review Focus 5
    for _, bad := range []string{"../x.dll", "a/b.dll", `a\b.dll`, `C:\x.dll`, "", ".."} {
        m := validManifest(); m.Plugins[0].FileName = bad
        if m.Validate() == nil { t.Errorf("accepted fileName %q", bad) }
    }
    for _, bad := range []string{"../x", `C:\x`, "/abs", "BepInEx/../../x"} {
        m := validManifest(); m.Remove = []string{bad}
        if m.Validate() == nil { t.Errorf("accepted remove %q", bad) }
    }
}
func TestValidateRequiresSHA(t *testing.T)      // sha256 not 64 lowercase hex -> error
func TestValidateDuplicateFileName(t *testing.T) // two plugins same fileName (case-insensitive) -> error
func TestParseSchemaTooNew(t *testing.T)        // schema 2 -> errors.Is(err, ErrSchemaTooNew)
func TestParseIgnoresUnknownFields(t *testing.T)
func TestFunDecode(t *testing.T)                // jokeText true + lines -> Fun(); invalid fun JSON -> zero Fun
func TestMarshalKeepsFunContent(t *testing.T) { // Parse -> Marshal -> Parse: Fun() equal, FunRaw semantically equal
}
func TestParseSourcesDuplicateID(t *testing.T)
```

  `Remove` entries may contain forward slashes (relative sub-paths like `BepInEx/plugins/X.dll`), but not `..`, a leading `/` or `\`, a drive letter, or an empty element.
- [ ] **Step 2: Run** `go test ./internal/manifest/`. Expected: FAIL.
- [ ] **Step 3: Implement.** For `Marshal`, use `json.MarshalIndent`. Whitespace inside `fun` may be normalised, but its content must not change. This is the plan's reading of "preserve" in §6.2.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** with message `feat(manifest): types, validation, sources`.

---

### Task 4: Manifest generator (workflow-side)

**Files:**
- Create: `internal/ghrel/ghrel.go`, `internal/ghrel/ghrel_test.go`, `internal/manifestgen/build.go`, `internal/manifestgen/build_test.go`, `cmd/manifestgen/main.go`

**Interfaces:**
- Consumes: Task 3 types.
- Produces:

```go
// ghrel
type ReleaseAsset struct{ Name, BrowserDownloadURL string; Size int64 }
type Release struct{ TagName string; Draft, Prerelease bool; Assets []ReleaseAsset }
type Client struct{ HTTP *http.Client; Token string; BaseURL string } // BaseURL default "https://api.github.com"
func (c *Client) Latest(ctx context.Context, repo string) (*Release, error) // newest non-draft non-prerelease from GET /repos/{repo}/releases?per_page=30
func (c *Client) ByTag(ctx context.Context, repo, tag string) (*Release, error)
func MatchAsset(r *Release, pattern string) (ReleaseAsset, error)            // substring; exactly one match else error naming the matches

// manifestgen
type Resolver interface {
    Resolve(ctx context.Context, src manifest.Source) (*ghrel.Release, error) // Pin != "" -> ByTag else Latest
}
type Hasher interface {
    Hash(ctx context.Context, url string) (sha256 string, size int64, err error) // downloads and hashes
}
type Result struct {
    Manifest *manifest.Manifest
    Changes  []string // human lines e.g. "Reactor: v2.3.0 -> v2.4.0"
    Failures []string // "<id>: <error>"; previous entry kept
}
func Build(ctx context.Context, src *manifest.Sources, old *manifest.Manifest, r Resolver, h Hasher, now time.Time) (*Result, error)
```

- [ ] **Step 1: Write failing tests** for `ghrel`, using `httptest.Server`:
  - `TestLatestSkipsPrerelease`: the releases list is `[pre, draft, stable]` and `Latest` returns `stable`.
  - `TestMatchAssetExactlyOne`: no match is an error, two matches is an error, one match returns it.
- [ ] **Step 2: Write failing tests** for `manifestgen`, using fake Resolver and Hasher:

```go
func TestBuildUnchangedSkipsHash(t *testing.T)   // same tag+asset name as old -> Hasher not called, entry identical, Changes empty
func TestBuildNewVersion(t *testing.T)           // new tag -> hashed, Changes has "Reactor: v1 -> v2"
func TestBuildFailureKeepsOld(t *testing.T)      // resolver error for one plugin -> old entry kept, Failures has it, others updated
func TestBuildPin(t *testing.T)                  // Pin "v1.0.0" -> Resolver called with pin (fake records)
func TestBuildPreservesFun(t *testing.T)         // old.FunRaw carried over unchanged
func TestBuildLauncherVersion(t *testing.T)      // launcher tag "v1.2.3" -> Version "1.2.3"
func TestBuildPluginFileName(t *testing.T)       // FileName = asset name
func TestBuildRemoveConflict(t *testing.T)       // remove path equal to "BepInEx/plugins/<plugin fileName>" -> Failures entry, remove path dropped
```

- [ ] **Step 3: Run** `go test ./internal/ghrel/ ./internal/manifestgen/`. Expected: FAIL.
- [ ] **Step 4: Implement.** "Unchanged" means same tag and same matched asset name as the old entry. `GeneratedAt` is set to `now` only when `Changes` is non-empty; otherwise the old value is kept.
- [ ] **Step 5: Implement `cmd/manifestgen`.**
  - Flags: `-sources sources.json -manifest manifest.json`.
  - Reads `GITHUB_TOKEN` from the environment.
  - Writes the manifest only if `Changes` is non-empty.
  - Prints `Changes` to stdout (used as the commit message body).
  - Exits 1 if `Failures` is non-empty, after writing.
- [ ] **Step 6: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 7: Commit** with message `feat(manifestgen): resolve sources into manifest`.

---

### Task 5: sources.json, initial manifest, manifest workflow

**Files:**
- Create: `sources.json`, `manifest.json`, `.github/workflows/manifest.yml`

- [ ] **Step 1: Write `sources.json`** from the old gist:
  - `modpack`: `{ "id": "TOU-Mira-Pack", "repo": "AU-Avengers/TOU-Mira", "assetPattern": "-steam-itch.zip" }`
  - `launcher`: `{ "id": "CleanInstaller", "repo": "Kiirb/CleanInstaller", "assetPattern": ".exe" }`
  - `remove`: `[]`
  - `plugins` (ids must be unique):

| id | repo | assetPattern |
|----|------|--------------|
| TownOfUsMira | AU-Avengers/TOU-Mira | TownOfUsMira.dll |
| MiraAPI | AU-Avengers/TOU-Mira | MiraAPI.dll |
| Reactor | NuclearPowered/Reactor | .dll |
| Mini.RegionInstall | miniduikboot/Mini.RegionInstall | .dll |
| TownOfUsMiraRolesExtension | rewalo/TownOfUsMiraRolesExtension | .dll |
| ModExplorer | xChipseq/ModExplorer | .dll |
| AUnlocker | astra1dev/AUnlocker | .dll |
| TownOfExtra | Mehzxzz/TownOfExtra | .dll |
| TownOfUsMiraDivaniModsAddOn | DivaniNL/TownOfUsMiraDivaniModsAddOn | .dll |
| Perfect-Comms | artriy/Perfect-Comms | .dll |

  **[Planner's call]** The old tool deleted the mod pack's `AUnlocker.dll` and then installed the AUnlocker plugin from astra1dev. Plugin sync already replaces a same-named file with the manifest version, so `remove` starts empty. Keeping the old entry would delete the plugin.
- [ ] **Step 2: Write `manifest.json`** with `schema: 1`, empty entries, and the `fun` block from spec §6.2 (`jokeText:false`, `chaosBar:false`, the four example lines).
- [ ] **Step 3: Write `manifest.yml`.**
  - Triggers: `schedule: cron "0 */3 * * *"`, `push` with `paths: [sources.json]`, `workflow_dispatch`.
  - `permissions: contents: write`.
  - Steps:
    1. Checkout.
    2. setup-go.
    3. `go run ./cmd/manifestgen -sources sources.json -manifest manifest.json > changes.txt`. Keep the exit code; the job is failed at the end if it was non-zero.
    4. If `git diff --quiet manifest.json` reports changes: configure the bot user, commit with message `manifest: update` followed by `changes.txt`, then run `git pull --rebase` and `git push`.
    5. On push rejection, retry steps 3 and 4 up to 3 times.
  - Use `concurrency: manifest` so two runs never overlap.
- [ ] **Step 4: Verify the workflow locally.** Run `GITHUB_TOKEN=<your token> go run ./cmd/manifestgen -sources sources.json -manifest manifest.json`. Expected: `manifest.json` is filled with all 10 plugins, the modpack and the launcher, and `Failures` is empty. The launcher fails until the first release exists, which is expected; note this. Any plugin with "exactly one match" errors gets a more specific `assetPattern`; record the chosen pattern.
- [ ] **Step 5: Commit** with message `feat: sources, manifest workflow`.

---

### Task 6: Verified download and cache

**Files:**
- Create: `internal/download/download.go`, `internal/download/download_test.go`

**Interfaces:**
- Produces:

```go
type ProgressFunc func(done, total int64)
type Cache struct{ Dir string; HTTP *http.Client }
// Get returns a path inside Cache.Dir named by sha256, downloading if absent or corrupt.
// Downloads to "<sha>.part", verifies sha256 and size, renames; on mismatch deletes and retries once; then ErrChecksum.
func (c *Cache) Get(ctx context.Context, a manifest.Asset, p ProgressFunc) (string, error)
func (c *Cache) Prune(keep []string) error // delete cached files whose sha is not in keep, and stray *.part files
func FetchManifest(ctx context.Context, h *http.Client, url string) ([]byte, error) // 15s timeout, non-200 -> error
var ErrChecksum = errors.New("checksum mismatch")
```

- [ ] **Step 1: Write failing tests** using `httptest.Server`:

```go
func TestGetDownloadsAndVerifies(t *testing.T)
func TestGetUsesCache(t *testing.T)            // second call -> zero requests
func TestGetChecksumMismatchRetriesThenFails(t *testing.T) // server always wrong bytes -> 2 requests, ErrChecksum, no file and no .part left
func TestGetInterruptedNeverUsed(t *testing.T) { // Review Focus 3
    // server sends half the body then closes; Get errors; Dir contains no "<sha>" and no "<sha>.part"
}
func TestGetCorruptCacheRedownloads(t *testing.T) // pre-existing "<sha>" with wrong content -> re-downloaded
func TestPrune(t *testing.T)
```

- [ ] **Step 2: Run** `go test ./internal/download/`. Expected: FAIL.
- [ ] **Step 3: Implement.** Set the User-Agent to `CleanInstaller/<version>` (version passed through a package var set in `main`). Do not add the user's name or any other identifying information to requests.
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** with message `feat(download): verified cached downloads`.

---

### Task 7: File utilities (copy, zip, config preservation)

**Files:**
- Create: `internal/fsutil/copy.go`, `internal/fsutil/zip.go`, `internal/fsutil/fsutil_test.go`

**Interfaces:**
- Produces:

```go
// CopyTree copies src to dst, skipping root-level names in excludeRoot (case-insensitive). progress gets bytes.
func CopyTree(src, dst string, excludeRoot []string, progress func(done, total int64)) error
// ExtractZip extracts into dst; strips a single common top-level folder; rejects entries escaping dst (ErrZipSlip).
func ExtractZip(zipPath, dst string, progress func(done, total int)) error
// MergeDirKeepExisting copies files from src into dst only where dst has no file at that relative path.
func MergeDirKeepExisting(src, dst string) error
var ErrZipSlip = errors.New("zip entry escapes destination")
var VanillaExcludes = []string{"winhttp.dll", "doorstop_config.ini", "changelog.txt", ".doorstop_version", "BepInEx"}
```

- [ ] **Step 1: Write failing tests**

```go
func TestCopyTreeExcludes(t *testing.T)        // root BepInEx/ and winhttp.dll skipped; sub/BepInEx/ (non-root) copied
func TestExtractZipStripsSingleRoot(t *testing.T) // "Pack/BepInEx/x.dll" -> dst/BepInEx/x.dll
func TestExtractZipNoStripMultiRoot(t *testing.T)
func TestExtractZipSlip(t *testing.T)          // entry "../evil.txt" -> ErrZipSlip, nothing outside dst
func TestMergeDirKeepExisting(t *testing.T)    // existing user file kept; new file added
```

- [ ] **Step 2: Run** `go test ./internal/fsutil/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** with message `feat(fsutil): copy, zip extract, merge`.

---

### Task 8: Platform interface and Steam discovery

**Files:**
- Create: `internal/platform/platform.go`, `internal/platform/windows.go` (`//go:build windows`), `internal/platform/fake.go` (test double, exported for other packages' tests), `internal/steam/locate.go`, `internal/steam/users.go`, `internal/steam/steam_test.go`

**Interfaces:**
- Consumes: Task 1 `vdf.ParseText`.
- Produces:

```go
// platform
type Platform interface {
    SteamRoot() (string, error)                     // Windows: HKCU\Software\Valve\Steam SteamPath, cleaned with filepath.Clean
    DataDir() (string, error)                       // %LOCALAPPDATA%\CleanInstaller (created)
    ProcessRunning(exePath string) bool             // by full path, case-insensitive; Windows: Toolhelp32 + QueryFullProcessImageName
    SteamRunning() bool                             // any process named steam.exe
    ShutdownSteam(ctx context.Context, steamRoot string) error // starts "<root>\steam.exe -shutdown", waits until !SteamRunning or ctx done
    StartSteam(steamRoot string) error
    OpenURL(url string) error                       // windows: ShellExecute "open"
    OpenFolder(path string) error
    StartProcess(exe, dir string, args ...string) (*os.Process, error)
}
type Fake struct{ /* fields mirroring each method's return, plus call records */ }

// steam
const AppID = "945360"; const SteamID64Base = 76561197960265728
type Game struct{ LibraryDir, CommonDir, VanillaDir, ModdedDir string; BuildID string }
func FindGame(steamRoot string) (*Game, error)  // ErrGameNotInstalled if no library lists 945360 or vanilla dir/appmanifest missing
type User struct{ ID3 uint32; ID64 uint64; PersonaName string; MostRecent bool }
func Users(steamRoot string) ([]User, error)     // from config/loginusers.vdf, filtered to those with userdata/<ID3> dir
var ErrGameNotInstalled = errors.New("among us not installed")
```

- [ ] **Step 1: Write failing tests** against a temp dir laid out like a Steam root, reusing the Task 1 testdata files:

```go
func TestFindGameSecondLibrary(t *testing.T)   // game in library "1"; ModdedDir = <lib>\steamapps\common\Among Us Modded; BuildID "16000001"
func TestFindGameNotInstalled(t *testing.T)    // errors.Is(err, ErrGameNotInstalled)
func TestFindGameListedButFolderMissing(t *testing.T) // listed in vdf but no folder -> ErrGameNotInstalled
func TestUsersID3(t *testing.T)                // ID64 76561198000000000 -> ID3 39734272
func TestUsersFiltersMissingUserdata(t *testing.T)
```

- [ ] **Step 2: Run** `go test ./internal/steam/`. Expected: FAIL.
- [ ] **Step 3: Implement** `steam` and the Windows `Platform`.
  - Process enumeration uses `golang.org/x/sys/windows` (`CreateToolhelp32Snapshot`, `OpenProcess` + `QueryFullProcessImageName`). Never shell out.
  - `OpenURL` and `OpenFolder` use `windows.ShellExecute`.
- [ ] **Step 4: Run** `go test ./... && GOOS=windows go vet ./...`. Expected: PASS.
- [ ] **Step 5: Commit** with message `feat(steam): platform interface, game and user discovery`.

---

### Task 9: Steam shortcut and artwork

**Files:**
- Create: `internal/steam/shortcuts.go`, `internal/steam/shortcuts_test.go`, `assets/steam/grid.png`, `hero.png`, `logo.png`, `icon.png`, `logo.json`, `assets/assets.go`

**Interfaces:**
- Consumes: Task 2 `vdf.BNode` and `ReadBinary`/`WriteBinary`.
- Produces:

```go
// assets
//go:embed steam/*
var Steam embed.FS

// steam
const ShortcutName = "Among Us Modded"
type Shortcut struct{ Exe, StartDir, LaunchOptions, Icon string } // Exe/StartDir unquoted; quoted when written
func ShortcutAppID(exeQuoted, name string) uint32                 // crc32.ChecksumIEEE([]byte(exeQuoted+name)) | 0x80000000
// RemoveShortcuts deletes entries for which match returns true, renumbers keys "0".."n-1", returns count removed.
func RemoveShortcuts(root *vdf.BNode, match func(entry *vdf.BNode) bool) int
func AddShortcut(root *vdf.BNode, s Shortcut) uint32              // appends as key "<len>", returns appid
func LoadShortcuts(userConfigDir string) (*vdf.BNode, error)       // missing file -> empty root with "shortcuts" map
func SaveShortcuts(userConfigDir string, root *vdf.BNode) error    // write to .tmp then rename; keep one "shortcuts.vdf.bak" of the previous file
func WriteArtwork(userConfigDir string, appID uint32, fs embed.FS) error // grid/<id>p.png, <id>_hero.png, <id>_logo.png, <id>_icon.png, <id>.json; returns icon path via IconPath
func IconPath(userConfigDir string, appID uint32) string
func RemoveArtwork(userConfigDir string, prefix string) error      // deletes grid/<prefix>p.png, _hero, _logo, _icon, .json
func OurShortcutMatcher(exePath, startDir string) func(*vdf.BNode) bool // Exe (quotes stripped) or StartDir equal, case-insensitive
```

  Shortcut entry fields are written in this order:
  - `appid` (int32 of the uint32)
  - `AppName`, `Exe` (quoted), `StartDir` (quoted), `icon`, `ShortcutPath` `""`, `LaunchOptions` `--launch`
  - `IsHidden` 0, `AllowDesktopConfig` 1, `AllowOverlay` 1, `OpenVR` 0, `Devkit` 0, `DevkitGameID` `""`, `DevkitOverrideAppID` 0, `LastPlayTime` 0, `FlatpakAppID` `""`, `sortas` `""`
  - `tags` (empty map)

  `userConfigDir` is `<steamRoot>\userdata\<ID3>\config`.

- [ ] **Step 1: Assets.** Download the four images the old tool used into `assets/steam/`:
  - `https://cdn2.steamgriddb.com/grid/24330531679f7fd5318e3e9dde4e1c99.png` → `grid.png`
  - `.../hero/1cc1fab198176208789cf94b71412dc8.png` → `hero.png`
  - `.../logo/1d92bf06b68f0b08837d6d88412df8ec.png` → `logo.png`
  - `.../icon/588bc7654c8815a85a09b0bc6d82a29f.png` → `icon.png`
  
  Write `logo.json` as `{"nVersion":1,"logoPosition":{"pinnedPosition":"CenterCenter","nWidthPct":23.704171934260415,"nHeightPct":65.2777777777778}}`. The owner may swap the images later.
- [ ] **Step 2: Write failing tests**

```go
func TestShortcutAppIDHighBit(t *testing.T)      // result & 0x80000000 != 0; deterministic for same input
func TestRemoveShortcutsRenumbers(t *testing.T) { // entries 0,1,2; remove 1 -> keys "0","1"; former "2" is now "1"
}
func TestAddShortcutNoCollision(t *testing.T)    // after removal, Add uses "<len>" and no existing entry is overwritten
func TestLoadShortcutsMissingFile(t *testing.T)  // Review Focus 4: no file, no grid dir -> Load ok, Add+Save+WriteArtwork create everything
func TestSaveLoadRoundTrip(t *testing.T)         // Exe stored with quotes, LaunchOptions "--launch"
func TestOurShortcutMatcherQuoted(t *testing.T)
func TestRemoveArtwork(t *testing.T)
```

- [ ] **Step 3: Run** `go test ./internal/steam/`. Expected: FAIL.
- [ ] **Step 4: Implement.**
- [ ] **Step 5: Run.** Expected: PASS.
- [ ] **Step 6: Commit** with message `feat(steam): shortcuts and artwork`.

---

### Task 10: Sync (state, plan, apply)

**Files:**
- Create: `internal/syncer/state.go`, `internal/syncer/sync.go`, `internal/syncer/sync_test.go`

**Interfaces:**
- Consumes:
  - Task 3 `manifest.Manifest`
  - Task 6 `download.Cache`
  - Task 7 `fsutil.*`
  - Task 8 `steam.Game`
  - Task 12 `progress.Tracker` (Task 12 can be done first, or use its exact interface below)
- Produces:

```go
type State struct {
    BuildID      string            `json:"buildId"`
    ModpackSHA   string            `json:"modpackSha"`
    Plugins      map[string]string `json:"plugins"` // fileName -> sha256
    ModdedDir    string            `json:"moddedDir"`
}
func LoadState(dataDir string) (*State, error) // missing -> (nil, nil)
func SaveState(dataDir string, s *State) error
type Plan struct{ Rebuild bool; Reason string; PluginsToFetch []manifest.Asset; FilesToDelete []string }
func MakePlan(st *State, g *steam.Game, m *manifest.Manifest) (*Plan, error) // reads disk under g.ModdedDir to hash plugins
func (p *Plan) Empty() bool
type Syncer struct{ Cache *download.Cache; Tracker *progress.Tracker; Log *slog.Logger }
func (s *Syncer) Apply(ctx context.Context, p *Plan, g *steam.Game, m *manifest.Manifest, dataDir string) (*State, error)
var ErrModdedInUse = errors.New("modded game is in use")
```

  Rules from spec §8.3:
  - **Rebuild when:** state is nil, `ModdedDir` is missing, `BuildID` differs, or `ModpackSHA` differs.
  - **Rebuild steps:**
    1. Build into `<ModdedDir>.new` by `CopyTree(vanilla, VanillaExcludes)`, then `ExtractZip(modpack)`.
    2. Write `steam_appid.txt` containing `945360` if absent.
    3. Merge the old `BepInEx/config` with `MergeDirKeepExisting(old/BepInEx/config, new/BepInEx/config)`, the old files winning. Copy the old config over the new one first, then call `MergeDirKeepExisting(newPackConfig, …)`; a one-line comment in the code states which side wins.
    4. Swap: rename `ModdedDir` → `ModdedDir.old`, rename `.new` → `ModdedDir`, delete `.old`. If the first rename fails (files locked), delete `.new`, leave `ModdedDir` untouched, and return `ErrModdedInUse`.
  - **Remove list:** applied before plugin sync.
  - **Plugin sync:** in `BepInEx/plugins/` (top level only), every manifest plugin is present with matching SHA. Any other top-level file there is deleted.
  - **Tracker weights:** rebuild copy 40, modpack download 25, extract 15, plugins 20. A plugins-only sync is 100% plugins.

- [ ] **Step 1: Write failing tests** with a temp vanilla dir, an `httptest` server for assets and a fixture modpack zip built in the test:

```go
func TestPlanFreshInstallRebuilds(t *testing.T)
func TestPlanUpToDateEmpty(t *testing.T)
func TestPlanBuildIDChangeRebuilds(t *testing.T)
func TestPlanWrongPluginHashFetches(t *testing.T)
func TestPlanExtraPluginDeleted(t *testing.T)
func TestApplyRebuildKeepsUserConfig(t *testing.T)   // BepInEx/config/keybinds.cfg survives with user content
func TestApplyRebuildWritesSteamAppID(t *testing.T)
func TestApplyDoesNotTouchVanilla(t *testing.T)      // hash vanilla tree before/after: identical
func TestApplyLockedKeepsOldFolder(t *testing.T) {   // Review Focus 2
    // hold an open handle on ModdedDir/Among Us.exe (Windows blocks rename); Apply -> ErrModdedInUse;
    // ModdedDir contents unchanged; no ".new" left
}
```

  Use a `t.Skip` on non-Windows for the locked test.
- [ ] **Step 2: Run** `go test ./internal/syncer/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** with message `feat(syncer): plan and apply sync with safe rebuild`.

---

### Task 11: Old CInstaller migration

**Files:**
- Create: `internal/migrate/migrate.go`, `internal/migrate/migrate_test.go`

**Interfaces:**
- Consumes: Task 9 shortcut and artwork functions, Task 8 `steam.Users`.
- Produces:

```go
const OldAppID int32 = -305070; const OldArtPrefix = "4294662226"
const OldExe = "CInstaller.exe"; const OldConfig = "CInstallerConfig.json"
type Findings struct{ ShortcutUsers []uint32; ArtUsers []uint32; OldFolder string }
func (f Findings) Any() bool
func Detect(steamRoot string, users []steam.User, moddedDir string) (Findings, error)
func OldShortcutMatcher(entry *vdf.BNode) bool // Exe (unquoted) base name equals CInstaller.exe, or (appid == -305070 && AppName == "Among Us Modded")
func CleanSteam(steamRoot string, f Findings, log *slog.Logger) error // removes shortcuts + artwork for every listed user
```

  Folder handling: `Detect` sets `OldFolder` when `moddedDir` contains `CInstallerConfig.json`. Task 14 then forces a rebuild by deleting `state.json`. The Task 10 rebuild already keeps `BepInEx/config` and replaces the folder.

- [ ] **Step 1: Write failing tests**

```go
func TestDetectAllUsers(t *testing.T)       // two users, old shortcut only in the non-MostRecent one -> found
func TestOldShortcutMatcher(t *testing.T)   // by exe name; by appid+name; unrelated shortcut not matched
func TestCleanSteamKeepsOtherShortcuts(t *testing.T) // 3 shortcuts incl. old in middle -> 2 left, renumbered, other entries byte-identical
func TestCleanSteamRemovesArt(t *testing.T)
func TestDetectOldFolder(t *testing.T)
```

- [ ] **Step 2: Run** `go test ./internal/migrate/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** with message `feat(migrate): detect and clean old CInstaller installs`.

---

### Task 12: Progress tracker, fun models, logging, errors, UI interface

**Files:**
- Create: `internal/progress/tracker.go`, `internal/fun/fun.go`, `internal/applog/applog.go`, `internal/apperr/apperr.go`, `internal/ui/ui.go`, plus `_test.go` for progress, fun and applog

**Interfaces:**
- Produces:

```go
// progress — thread-safe; UI polls Snapshot
type Tracker struct{ /* mutex, stages */ }
func NewTracker() *Tracker
func (t *Tracker) Plan(weights ...float64)             // declare all stage weights up front (sum normalised to 100); resets progress
func (t *Tracker) Begin(status string)                 // starts the next declared stage (completing the previous one)
func (t *Tracker) Set(fraction float64)                // 0..1 within current stage; monotonic within stage
func (t *Tracker) Status(s string)
func (t *Tracker) Finish()                             // total = 100, done = true
type Snapshot struct{ Percent float64; Status string; Done bool }
func (t *Tracker) Snapshot() Snapshot

// fun — pure, rng injected
type JokeText struct{ /* lines, rng, next switch time */ }
func NewJokeText(lines []string, rng *rand.Rand) *JokeText
func (j *JokeText) Line(now time.Time) string // switches every 1.5–3 s (uniform); never same line twice in a row if len>1; "" if no lines
type ChaosBar struct{ /* displayed, velocity, rng */ }
func NewChaosBar(rng *rand.Rand) *ChaosBar
func (c *ChaosBar) Step(dt time.Duration, real Snapshot) float64 // nonsense 0..99 while !real.Done; exactly 100 once real.Done

// applog
func Open(logDir string, now time.Time) (*slog.Logger, io.Closer, error) // file "cleaninstaller-20261112-153000.log"; deletes oldest beyond 10

// apperr
type UserError struct{ Msg string; Err error } // Msg is German, shown to user; Err logged
func (e *UserError) Error() string; func (e *UserError) Unwrap() error
func New(msg string, err error) *UserError
func Message(err error) string // UserError.Msg if present, else "Ein unerwarteter Fehler ist aufgetreten."

// ui — implemented by Gio (Task 13) and by a fake in tests
type Display struct{ JokeText bool; ChaosBar bool; JokeLines []string } // zero value = honest progress
type UI interface {
    Progress(title string, t *progress.Tracker, d Display) // shows/updates the progress window until t.Snapshot().Done or Close
    Confirm(title, body, okLabel, cancelLabel string) bool
    Choose(title, body string, options []string) (index int, ok bool)
    Info(title, body string, buttons ...string) int        // returns pressed button index
    Error(err error, logDir string)                        // shows apperr.Message(err) + "Log-Ordner öffnen" button
    Close()
}
```

- [ ] **Step 1: Write failing tests**

```go
func TestTrackerWeighted(t *testing.T)     // Plan(40,60); Begin; Set(0.5) -> 20; Begin; Set(0.5) -> 70; Finish -> 100, Done
func TestTrackerConcurrent(t *testing.T)   // -race: Set from goroutine while Snapshot polled
func TestJokeTextInterval(t *testing.T)    // seeded rng: line constant for <1.5s, changed by 3s; no immediate repeat
func TestJokeTextEmpty(t *testing.T)
func TestChaosBarRangeAndFinish(t *testing.T) // 10k steps: always 0..99 while !Done; goes backwards at least once; ==100 on first step with Done
func TestApplogKeepsTen(t *testing.T)
func TestApperrMessage(t *testing.T)       // wrapped UserError found via errors.As
```

- [ ] **Step 2: Run** `go test -race ./internal/progress/ ./internal/fun/ ./internal/applog/ ./internal/apperr/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expected: PASS.
- [ ] **Step 5: Commit** with message `feat: progress tracker, joke/chaos models, logging, errors, UI interface`.

---

### Task 13: Gio UI

**Files:**
- Create: `internal/ui/gioui.go`, `internal/ui/gioui_windows.go` (dark title bar via `DwmSetWindowAttribute` attr 20, like the old tool)

**Interfaces:**
- Consumes: Task 12 `ui.UI`, `progress.Tracker`, `fun.*`.
- Produces: `func NewGio(appTitle string) ui.UI`. All methods are safe to call from the orchestration goroutine. The Gio event loop runs on the main goroutine, so `main` calls `ui.Run(func(u ui.UI){ ... })`. Add `func Run(title string, body func(ui.UI)) ` to `ui.go`, with Gio as the implementation.

**Layout:**
- Window: about 420×180, dark background `#1E1E1E`, white text, centred on screen.
- Progress screen: title, a status line above the bar, the bar, and no buttons. Closing the window during progress asks "Abbrechen?" through `Confirm`.

**Display behaviour:**
- With `Display.JokeText`, the status line shows `JokeText.Line(now)` instead of the tracker status.
- With `Display.ChaosBar`, the bar shows `ChaosBar.Step` instead of the tracker percent.
- The UI redraws at about 30 fps while the progress screen is visible.

**Errors:** `Error()` always uses `apperr.Message(err)`. It is never replaced by a joke line, even in joke mode.

- [ ] **Step 1: Add the dependency** with `go get gioui.org@latest`, then verify `CGO_ENABLED=0 GOOS=windows go build ./...`. Expected: success.
- [ ] **Step 2: Implement** the screens.
- [ ] **Step 3: Add a dev-only demo:** `cmd/cleaninstaller/demo_ui.go`, behind build tag `uidemo`. It runs each screen with a fake tracker: honest, joke text, chaos bar, then an error.
- [ ] **Step 4: Run** `go run -tags uidemo ./cmd/cleaninstaller`. Expected, checked visually:
  - honest bar tracks the fake tracker
  - joke lines rotate while the bar stays accurate
  - chaos bar jumps backwards at times and snaps to 100 at finish
  - error screen shows the German message and a working "Log-Ordner öffnen" button
- [ ] **Step 5: Commit** with message `feat(ui): Gio windows`.

---

### Task 14: Installer flow (install, repair, uninstall, migration)

**Files:**
- Create: `internal/install/install.go`, `internal/install/install_test.go`
- Modify: `cmd/cleaninstaller/main.go` (wire `ModeInstall`)

**Interfaces:**
- Consumes: everything above.
- Produces:

```go
type Deps struct {
    P        platform.Platform
    UI       ui.UI
    HTTP     *http.Client
    ManifestURL string
    SelfPath string            // os.Executable()
    Log      *slog.Logger
    TempDir  string            // os.TempDir()
}
type Outcome int // Installed, Repaired, Uninstalled, Cancelled, Relaunched
func Run(ctx context.Context, d Deps) (Outcome, error)
```

**Flow (spec §8.4, §8.6, §8.8):**

1. **Relaunch guard.** If `SelfPath` is inside `DataDir`, or inside any `ModdedDir`/`Among Us Modded` folder, copy self to `<TempDir>\CleanInstaller-setup.exe`, start it with the same arguments (visible), and return `Relaunched`.
2. **Fetch the manifest.** On failure, fall back to `manifest.cache.json`. On a successful fetch, write it to the cache. With no manifest at all, return `apperr.New("Keine Internetverbindung oder GitHub nicht erreichbar. Bitte später erneut versuchen.", err)`.
3. **Find Steam.** Missing Steam gives `apperr` "Steam wurde nicht gefunden. Bitte installiere Steam und starte den Installer erneut."
4. **Find the game.** `ErrGameNotInstalled` leads to `Confirm` "Among Us ist nicht installiert. Jetzt über Steam installieren?". On OK, `OpenURL("steam://install/945360")`. Then `Info` "Starte den Installer erneut, sobald Among Us installiert ist." and return `Cancelled`. **Never continue.**
5. **Pick the Steam user.** Choose from `steam.Users`: one user is used directly, several go through `Choose` listing persona names. No users gives `apperr` "Kein Steam-Konto gefunden. Bitte einmal in Steam anmelden."
6. **Detect.** Run `migrate.Detect` and check whether `state.json` exists.
7. **First screen:**
   - **Already installed:** `Choose` with "Reparieren", "Deinstallieren", "Abbrechen".
   - **Otherwise:** `Confirm` "Installieren", whose body lists the changes in German: "Erstellt den Ordner „Among Us Modded“ neben deinem Among Us (dein normales Among Us bleibt unverändert)", "Fügt in Steam den Eintrag „Among Us Modded“ hinzu", "Kopiert CleanInstaller nach %LOCALAPPDATA%\CleanInstaller". If `Findings.Any()`, add "Ersetzt deine alte CInstaller-Installation".
8. **Install or repair:**
   1. If an old folder was found, or Repair was chosen, delete `state.json`.
   2. Plan and apply sync with `ui.Display` built from `m.Fun()`. **Joke mode is only on if the manifest was fetched live** in this run (not from the cache).
   3. Copy self to `<DataDir>\CleanInstaller.exe`.
   4. **Steam must be closed:** if `SteamRunning`, `Confirm` "Steam muss kurz geschlossen werden, um den Eintrag hinzuzufügen. Jetzt schließen?". On Cancel, return `Cancelled` before any Steam file is touched. Call `ShutdownSteam` with a 60 s timeout; on timeout, `apperr` "Steam konnte nicht geschlossen werden. Bitte schließe Steam selbst und starte den Installer erneut."
   5. `migrate.CleanSteam`.
   6. For the chosen user: `RemoveShortcuts(OurShortcutMatcher)`, `AddShortcut`, `WriteArtwork`, `SaveShortcuts`.
   7. `Info` "Fertig! Starte „Among Us Modded“ in Steam.", with buttons "Steam starten" and "Schließen".
9. **Uninstall:**
   1. Close Steam as above.
   2. For every user: `RemoveShortcuts(OurShortcutMatcher)` and `RemoveArtwork` for our appid prefix.
   3. Delete `ModdedDir`.
   4. Delete `DataDir` (the relaunch guard ensures we're not running from it).
   5. `Info` "CleanInstaller wurde entfernt."
- **Every returned error** is shown with `UI.Error(err, logDir)` by `main`.

- [ ] **Step 1: Write failing tests** with `platform.Fake`, a fake UI that scripts answers and records calls, a temp Steam root, and an `httptest` manifest/asset server:

```go
func TestInstallFresh(t *testing.T)            // ends Installed; shortcut with --launch for chosen user; exe copied to DataDir; state.json written
func TestInstallGameMissingStops(t *testing.T) // OpenURL steam://install/945360 called; no ModdedDir created; Outcome Cancelled
func TestInstallSteamRunningCancel(t *testing.T) // Confirm=false -> shortcuts.vdf untouched
func TestInstallMultipleUsersChoose(t *testing.T) // Choose called with persona names; shortcut only for chosen
func TestInstallMigratesOld(t *testing.T)      // old shortcut in user A, old folder w/ CInstallerConfig.json + BepInEx/config/x.cfg -> old shortcut gone, art gone, x.cfg kept, one "Among Us Modded" entry
func TestInstallRelaunchFromDataDir(t *testing.T) // SelfPath in DataDir -> StartProcess(TempDir\CleanInstaller-setup.exe); Outcome Relaunched
func TestInstallJokeOnlyWhenLive(t *testing.T) // manifest fun.jokeText true: live -> Display.JokeText true; server down + cache -> false
func TestInstallOffline(t *testing.T)          // no server, no cache -> UserError message "Keine Internetverbindung…"
func TestUninstall(t *testing.T)               // shortcut removed for all users, ModdedDir and DataDir gone, vanilla untouched
```

- [ ] **Step 2: Run** `go test ./internal/install/`. Expected: FAIL.
- [ ] **Step 3: Implement** `Run` and wire it in `main`:
  1. Open the log (`applog`).
  2. Call `ui.Run`.
  3. Call `install.Run`.
  4. On error, `UI.Error`.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** with message `feat(install): installer, repair, uninstall, migration`.

---

### Task 15: Self-update and launcher flow

**Files:**
- Create: `internal/selfupdate/selfupdate.go`, `internal/selfupdate/selfupdate_test.go`, `internal/launch/launch.go`, `internal/launch/launch_test.go`
- Modify: `cmd/cleaninstaller/main.go` (wire `ModeLaunch`)

**Interfaces:**
- Produces:

```go
// selfupdate
func CleanupOld(selfPath string) error                       // delete "<self>.old" if present
func Needed(current string, m *manifest.Manifest) bool       // semver.Compare("v"+m.Launcher.Version, "v"+current) > 0; false if either invalid
func Apply(ctx context.Context, selfPath string, cache *download.Cache, a manifest.Asset) error // get+verify, rename self -> .old, copy new -> self
// launch
type Deps = install.Deps // same fields
func Run(ctx context.Context, d Deps, currentVersion string) error
```

**Launcher flow (spec §8.5):**
1. `CleanupOld`.
2. **If the modded game is already running** (`ProcessRunning(<ModdedDir>\Among Us.exe)`): log it and return nil. No sync, no second instance.
3. **Fetch the manifest, falling back to the cache.** If both fail, skip to step 6. **Never use joke mode.**
4. **Self-update:** if `Needed`, `Apply`, then `StartProcess(self, dir, "--launch")` and return nil.
5. **Sync:** `MakePlan`. If not `Empty()`, show `UI.Progress` with a zero `Display` and run `Apply`. `ErrModdedInUse` becomes `apperr` "Among Us Modded konnte nicht aktualisiert werden, weil Dateien noch benutzt werden. Schließe das Spiel und versuche es erneut." Other sync errors are logged; if the modded folder still has `Among Us.exe`, continue launching with what's on disk, otherwise return the error.
6. **Start** `<ModdedDir>\Among Us.exe` with dir `ModdedDir`, wait for it to exit, and return.
   - If state or the game is missing entirely: `apperr` "Among Us Modded ist nicht installiert. Bitte führe den CleanInstaller erneut aus."

- [ ] **Step 1: Write failing tests**

```go
func TestNeeded(t *testing.T)                 // "1.0.0" vs "1.0.1" true; equal false; dev "0.0.0-dev" vs "1.0.0" true; garbage false
func TestApplyRenamesAndReplaces(t *testing.T) // self file replaced, .old exists with old content
func TestCleanupOld(t *testing.T)
func TestLaunchUpToDateNoWindow(t *testing.T)  // empty plan -> UI.Progress never called; StartProcess(Among Us.exe)
func TestLaunchOfflineStillStarts(t *testing.T)
func TestLaunchAlreadyRunning(t *testing.T)    // Review Focus 2: Fake.ProcessRunning true -> no sync, no StartProcess
func TestLaunchSelfUpdateRestarts(t *testing.T) // StartProcess(self, "--launch") and no game start in this run
func TestLaunchIgnoresJokeMode(t *testing.T)   // manifest jokeText true -> Progress got zero Display
```

- [ ] **Step 2: Run** `go test ./internal/selfupdate/ ./internal/launch/`. Expected: FAIL.
- [ ] **Step 3: Implement** and wire it in `main`.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit** with message `feat(launch): launcher flow and self-update`.

---

### Task 16: Exe resources, release workflow, README, manual test checklist

**Files:**
- Create: `cmd/cleaninstaller/winres/winres.json`, `cmd/cleaninstaller/winres/icon.png` (from `assets/steam/icon.png` or `CInstaller.ico` of the old repo), `.github/workflows/release.yml`, `README.md`, `docs/manual-test-checklist.md`

**Exe resources.** `winres.json` declares:
- `ProductName` "CleanInstaller", `CompanyName` "Kiirb", `FileDescription` "Installer und Launcher für Among Us Mods", `LegalCopyright` "MIT License"
- version filled from the tag at build time
- application manifest: `execution-level: as invoker`, `dpi-awareness: per monitor v2`
- icon

**`release.yml`**
- Triggers: tag `v*`. `runs-on: windows-latest`.
- Steps:
  1. setup-go.
  2. `go test ./...`.
  3. `go install github.com/tc-hib/go-winres@latest`.
  4. `go-winres make --in cmd/cleaninstaller/winres/winres.json --out cmd/cleaninstaller/rsrc --product-version ${tag#v} --file-version ${tag#v}`.
  5. `CGO_ENABLED=0 go build -trimpath -ldflags "-H windowsgui -X main.version=${tag#v}" -o dist/CleanInstaller.exe ./cmd/cleaninstaller`.
  6. Upload as a GitHub Release asset with `softprops/action-gh-release`.
- **Forbidden:** UPX and `-s -w`.
- Leave a commented placeholder step `# SignPath signing (enable after SignPath Foundation approval)`, linking SignPath's GitHub Actions docs.

**`README.md`** (German for the user sections, English for the maintainer sections):
- **Download / Installation:** a link to the latest release, plus "Windows-Schutz: auf „Weitere Informationen“ → „Trotzdem ausführen“" with a screenshot placeholder.
- **Was das Programm macht:** the changes from §8.4, and that the vanilla game is untouched.
- **Deinstallation.**
- **Privacy / Datenschutz:** "Lädt nur von GitHub herunter, sendet keine Daten."
- **Code signing policy:** SignPath's required wording "Free code signing provided by SignPath.io, certificate by SignPath Foundation" (commented out until approved), and team roles (Kiirb: author, reviewer, approver).
- **Maintainer notes:**
  - editing `sources.json` and pins
  - toggling `fun.jokeText` / `fun.chaosBar` in `manifest.json` (takes effect within ~5 min)
  - releasing (tag `vX.Y.Z`)
  - Microsoft false-positive submission URL (`https://www.microsoft.com/wdsi/filesubmission`)
  - optional old-repo migration release: publish the new exe as release `0.2.0.0` on `Kiirb/CInstaller` (spec §8.8)
  - Linux is phase 2

**`docs/manual-test-checklist.md`:** the spec §14 manual list, one checkbox each.

- [ ] **Step 1: Write** `winres.json`, then run the `go-winres make` and `go build` commands locally. Expected: `dist/CleanInstaller.exe` exists. Its file Properties → Details show the product name, version and description.
- [ ] **Step 2: Write** `release.yml`, `README.md` and `docs/manual-test-checklist.md`.
- [ ] **Step 3: Verify** that `go vet ./...` and `go test ./...` pass.
- [ ] **Step 4: Check strings.** Run `strings dist/CleanInstaller.exe | grep -iE "kreditkart|passw|discord|token klauen"`. Expected: no output, so no joke strings are in the binary.
- [ ] **Step 5: Commit** with message `chore: exe resources, release workflow, README, test checklist`.
- [ ] **Step 6: Hand off to the owner** (needs their GitHub account):
  1. Create the public `Kiirb/CleanInstaller` repo and push.
  2. Enable MFA.
  3. Run the manifest workflow once.
  4. Tag `v0.1.0`.
  5. Work through `docs/manual-test-checklist.md` on a clean Windows 11 PC with Steam and Among Us.
  6. Apply to SignPath Foundation after the first working release.
