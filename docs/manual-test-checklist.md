# Manual test checklist

Run on a clean Windows 11 PC (or VM) with Steam and Among Us installed. Keep the log folder (`%LOCALAPPDATA%\CleanInstaller\logs`) open while testing.

- [ ] **Fresh install:** download the release exe, run it. SmartScreen appears once ("Weitere Informationen" → "Trotzdem ausführen"). The first screen lists the changes. After install, "Among Us Modded" is in Steam with artwork, and `Among Us Modded` sits next to `Among Us`.
- [ ] **Play:** start "Among Us Modded" from Steam. No window appears, the game starts modded, and Steam shows you as playing until the game closes.
- [ ] **Second run of the installer:** offers Reparieren / Deinstallieren / Abbrechen. Repair rebuilds and keeps the shortcut.
- [ ] **Plugin update:** change a plugin in `manifest.json` (or wait for the workflow). The next launch shows an update window briefly, then starts the game.
- [ ] **Game update:** after Steam updates Among Us, the next launch rebuilds `Among Us Modded`. `BepInEx\config` settings are kept.
- [ ] **Offline launch:** disconnect the network, start from Steam. The game starts with what's installed.
- [ ] **Launch while the game is running:** press Play again. Nothing breaks and no second game starts.
- [ ] **Multiple Steam accounts:** the installer asks which account. The shortcut appears only for that account.
- [ ] **Steam running during install:** the installer asks to close Steam. After "Steam starten", the new entry is there.
- [ ] **Uninstall:** removes the Steam entry, `Among Us Modded` and `%LOCALAPPDATA%\CleanInstaller` (except logs). Vanilla Among Us still works.
- [ ] **Joke text:** set `fun.jokeText: true` in `manifest.json`, wait ~5 min, and run the installer. The status lines are jokes and the bar is accurate. The launcher stays normal.
- [ ] **Chaos bar:** set `fun.chaosBar: true`. The bar misbehaves, then jumps to 100% when done. Errors still show the real message.
- [ ] **Old CInstaller PC:** on a PC with the old tool installed, run the new installer. It mentions replacing the old install. Afterwards there's exactly one "Among Us Modded" entry, the old artwork is gone and `BepInEx\config` is kept.
- [ ] **Optional old-tool auto-update path** (design §8.8): publish the new exe as a release on `Kiirb/CInstaller` and start the old shortcut. It updates into the new installer.
- [ ] **Antivirus:** check Windows Security protection history. Nothing was quarantined.
