# CleanInstaller

Installiert und aktualisiert das gemeinsame Among-Us-Mod-Setup unserer Gruppe. Einmal ausführen, danach in Steam einfach **„Among Us Modded“** starten. Die Mods sind dann immer auf dem gleichen Stand wie bei allen anderen.

## Installation

1. Lade die neueste **`CleanInstaller.exe`** von der [Releases-Seite](https://github.com/Kiirb/CleanInstaller/releases/latest) herunter.
2. Starte sie per Doppelklick.
3. Falls Windows „Der Computer wurde durch Windows geschützt“ anzeigt: auf **„Weitere Informationen“** klicken, dann auf **„Trotzdem ausführen“**. Das passiert nur beim ersten Mal.
   <!-- TODO: Screenshot der SmartScreen-Meldung einfügen -->
4. Folge den Schritten im Fenster. Danach findest du **„Among Us Modded“** in deiner Steam-Bibliothek.

## Was das Programm macht

- Erstellt den Ordner `Among Us Modded` neben deinem normalen `Among Us`. **Dein normales Among Us bleibt unverändert.**
- Fügt in Steam den Eintrag „Among Us Modded“ (mit Bildern) hinzu. Dafür wird Steam kurz geschlossen.
- Kopiert sich selbst nach `%LOCALAPPDATA%\CleanInstaller`.
- Beim Start über Steam: prüft auf Updates der Mods und des Programms, lädt nur Geändertes herunter und startet dann das Spiel.

## Deinstallation

`CleanInstaller.exe` erneut starten und **„Deinstallieren“** wählen. Das entfernt den Steam-Eintrag, `Among Us Modded` und `%LOCALAPPDATA%\CleanInstaller`.

## Datenschutz

CleanInstaller lädt nur Dateien von GitHub herunter (die Mod-Liste und die Mod-Dateien) und **sendet keine Daten**. Logs bleiben lokal unter `%LOCALAPPDATA%\CleanInstaller\logs`.

## Code signing policy

<!-- Enable once SignPath Foundation has approved the project:
Free code signing provided by [SignPath.io](https://signpath.io), certificate by [SignPath Foundation](https://signpath.org).
-->

- Committers and reviewers: [Kiirb](https://github.com/Kiirb)
- Approvers: [Kiirb](https://github.com/Kiirb)

Releases are built from this repository by GitHub Actions (`.github/workflows/release.yml`).

---

## Maintainer notes

### Mods (`sources.json`)

- `sources.json` lists where each mod comes from: a GitHub repo plus an `assetPattern`. The pattern is a case-insensitive substring that must match **exactly one** release asset.
- The **Update manifest** workflow runs every 3 hours, and immediately when `sources.json` changes. It writes `manifest.json` with exact versions, URLs and SHA-256 checksums. Everyone installs exactly what's in `manifest.json`.
- **Hold a mod at a version:** add `"pin": "<release tag>"` to its entry. Remove the pin to follow the latest stable release again. Prereleases are always skipped.
- When a source fails (repo gone, pattern matches nothing or several assets), the workflow keeps the previous entry and the run fails. GitHub emails you.
- Plugins the mod pack zip ships itself (e.g. MiraAPI, AchievementsAPI) are kept. Only files in `BepInEx/plugins` that are neither in the manifest nor from the pack are deleted.

### Joke mode (`manifest.json` → `fun`)

Edit directly on GitHub. Changes reach clients within ~5 minutes (GitHub caches raw files). Joke mode only affects the **installer**, never the launcher.

- `jokeText`: the status text shows random `jokeLines`. The progress bar stays accurate.
- `chaosBar`: the progress bar misbehaves. It still ends at 100% exactly when the work is done.
- When a friend is stuck, set both to `false` and ask them to rerun the installer. Errors always show the real message anyway.

The workflow never changes the `fun` block.

### Releasing

Push a tag `vX.Y.Z`. The **Release** workflow builds `CleanInstaller.exe` and publishes it. The manifest workflow then picks it up as the new `launcher` entry, and installed launchers update themselves on their next start.

The manifest workflow fails on the `launcher` entry until the first release exists. That's expected.

### Antivirus false positives

If a release gets flagged, submit it at <https://www.microsoft.com/wdsi/filesubmission> (choose "Incorrectly detected as malware/malicious"). Don't add packers (UPX) or strip flags to the build.

### Old CInstaller users

Running the new installer cleans up the old install automatically (old shortcut, artwork, folder).

Optional: publishing the new exe as a release `0.2.0.0` on `Kiirb/CInstaller` makes old launchers update into the new installer. This only works if the old tool's built-in GitHub token still works.

### Linux

Planned as phase 2 (design doc §12). The code keeps OS specifics in `internal/platform`.

### Development

```
go test ./...
go run -tags uidemo ./cmd/uidemo [honest|joke|chaos|dialogs]   # look at the windows
go run ./cmd/manifestgen -sources sources.json -manifest manifest.json
```

Manual release test: [`docs/manual-test-checklist.md`](docs/manual-test-checklist.md). Design: [`docs/superpowers/specs`](docs/superpowers/specs).
