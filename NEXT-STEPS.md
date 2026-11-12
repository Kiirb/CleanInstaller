# Next steps (still to do)

State on 2026-11-12: code is complete and pushed; no release exists yet.

1. **Check CI.** The first CI run on GitHub Actions (push of `main`) had not finished when work stopped. Check the Actions tab.
2. **Publish the first release.** Tagging was not done:
   ```
   git tag -a v0.1.0 -m "CleanInstaller 0.1.0"
   git push origin v0.1.0
   ```
   The Release workflow then builds `CleanInstaller.exe`.
3. **Run the "Update manifest" workflow** once by hand (Actions tab → Update manifest → Run workflow). It fails on the `launcher` entry until step 2 is done; after that it should be green.
4. **Test on a real PC** with Steam and Among Us, using [`docs/manual-test-checklist.md`](docs/manual-test-checklist.md).
5. **README:** add the SmartScreen screenshot (placeholder in the Installation section).
6. **Apply to SignPath Foundation** for free code signing once a working release exists. Then enable the commented signing step in `.github/workflows/release.yml` and the signing line in the README.
7. **Optional:** publish the new exe as release `0.2.0.0` on `Kiirb/CInstaller` so old launchers update into the new installer (design §8.8).
8. **Smaller known issues** (from the final review, not fixed yet):
   - Pin the `go-winres` version in `release.yml` (SignPath wants reproducible builds).
   - The manifest workflow shows green even if all three push attempts are rejected.
   - The temp installer copy `%TEMP%\CleanInstaller-setup.exe` is never cleaned up.
   - An empty `shortcuts.vdf` makes the shortcut step fail.
   - A crash between the folder-swap renames can leave only `Among Us Modded.old`.
   - A locked plugin file during a small update isn't reported as "files in use".
   - No guard against an endless self-update loop if a release's built-in version doesn't match its tag.
   - The workflow normalises whitespace in the `fun` block and drops unknown top-level manifest keys.
