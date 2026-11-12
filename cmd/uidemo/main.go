//go:build uidemo

// Command uidemo shows every screen with a fake tracker, for checking the UI
// by eye: go run -tags uidemo ./cmd/uidemo [honest|joke|chaos|dialogs]
package main

import (
	"errors"
	"os"
	"time"

	"github.com/Kiirb/CleanInstaller/internal/apperr"
	"github.com/Kiirb/CleanInstaller/internal/platform"
	"github.com/Kiirb/CleanInstaller/internal/progress"
	"github.com/Kiirb/CleanInstaller/internal/ui"
)

func main() {
	mode := "all"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	lines := []string{"Demo-Zeile eins", "Demo-Zeile zwei", "Demo-Zeile drei"}
	ui.Run("CleanInstaller (Demo)", platform.Windows{}.OpenFolder, func(u ui.UI) {
		run := func(title string, d ui.Display) {
			t := progress.NewTracker()
			t.Plan(25, 40, 15, 20)
			u.Progress(title, t, d)
			for _, stage := range []string{"Mod-Paket wird heruntergeladen", "Among Us wird kopiert", "Mod-Paket wird entpackt", "Mods werden installiert"} {
				t.Begin(stage)
				for i := 0; i <= 40; i++ {
					t.Set(float64(i) / 40)
					time.Sleep(60 * time.Millisecond)
				}
			}
			t.Finish()
			time.Sleep(time.Second)
		}
		if mode == "all" || mode == "honest" {
			run("Among Us Modded wird installiert", ui.Display{})
		}
		if mode == "all" || mode == "joke" {
			run("Among Us Modded wird installiert", ui.Display{JokeText: true, JokeLines: lines})
		}
		if mode == "all" || mode == "chaos" {
			run("Among Us Modded wird installiert", ui.Display{JokeText: true, ChaosBar: true, JokeLines: lines})
		}
		if mode == "reopen" {
			t := progress.NewTracker()
			t.Plan(1)
			u.Progress("Erstes Fenster", t, ui.Display{})
			time.Sleep(time.Second)
			u.Close()
			time.Sleep(2 * time.Second)
			u.Info("Zweites Fenster", "Prozess lebt noch nach Close().", "OK")
		}
		if mode == "all" || mode == "dialogs" {
			u.Confirm("CleanInstaller", "Das passiert jetzt:\n• Erstellt den Ordner „Among Us Modded“\n• Fügt in Steam den Eintrag hinzu", "Installieren", "Abbrechen")
			u.Choose("Steam-Konto wählen", "Für welches Steam-Konto soll der Eintrag angelegt werden?", []string{"Alice", "Bob"})
			u.Error(apperr.New("Steam wurde nicht gefunden. Bitte installiere Steam und starte den Installer erneut.", errors.New("demo")), os.TempDir())
		}
	})
}
