package ui

import (
	"image/color"
	"math/rand"
	"os"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Kiirb/CleanInstaller/internal/apperr"
	"github.com/Kiirb/CleanInstaller/internal/fun"
	"github.com/Kiirb/CleanInstaller/internal/progress"
)

var (
	bgColor     = color.NRGBA{0x1e, 0x1e, 0x1e, 0xff}
	fgColor     = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	accentColor = color.NRGBA{0xc5, 0x11, 0x11, 0xff} // Among Us red
	mutedColor  = color.NRGBA{0xb0, 0xb0, 0xb0, 0xff}
)

type screenKind int

const (
	kindProgress screenKind = iota
	kindDialog
)

type screen struct {
	kind    screenKind
	title   string
	body    string
	buttons []string
	clicks  []widget.Clickable
	result  chan int         // dialog: index of pressed button, -1 when the window closed
	onClick func(i int) bool // optional: return false to keep the dialog open

	tracker *progress.Tracker
	display Display
	joke    *fun.JokeText
	chaos   *fun.ChaosBar
	last    time.Time
}

type gioUI struct {
	title string
	open  func(path string) error

	mu     sync.Mutex
	w      *app.Window
	cur    *screen
	closed chan struct{}
	once   sync.Once
}

// Run creates the UI, runs body on a separate goroutine and blocks the main
// goroutine in the Gio event loop. The process exits when body returns or the
// user closes the window. open is used for the "Log-Ordner öffnen" button.
func Run(title string, open func(path string) error, body func(UI)) {
	u := &gioUI{title: title, open: open, closed: make(chan struct{})}
	go func() {
		body(u)
		u.Close()
		os.Exit(0)
	}()
	app.Main()
}

// window lazily creates the window, so a launcher with nothing to do never
// shows one.
func (u *gioUI) window() *app.Window {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.w == nil {
		w := new(app.Window)
		w.Option(app.Title(u.title), app.Size(unit.Dp(500), unit.Dp(300)), app.MinSize(unit.Dp(460), unit.Dp(260)))
		u.w = w
		go u.loop(w)
		go u.ticker()
	}
	return u.w
}

func (u *gioUI) loop(w *app.Window) {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette = material.Palette{Bg: bgColor, Fg: fgColor, ContrastBg: accentColor, ContrastFg: fgColor}
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			u.once.Do(func() { close(u.closed) })
			// The user closed the window: stop everything. The sync never
			// leaves a half-built folder in place (see syncer), so exiting is safe.
			os.Exit(0)
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			u.layout(gtx, th)
			e.Frame(gtx.Ops)
		default:
			handleViewEvent(e)
		}
	}
}

// ticker redraws ~30 times a second while a progress screen is visible.
func (u *gioUI) ticker() {
	t := time.NewTicker(33 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-u.closed:
			return
		case <-t.C:
			u.mu.Lock()
			s, w := u.cur, u.w
			u.mu.Unlock()
			if s != nil && s.kind == kindProgress && w != nil {
				w.Invalidate()
			}
		}
	}
}

func (u *gioUI) show(s *screen) {
	w := u.window()
	u.mu.Lock()
	u.cur = s
	u.mu.Unlock()
	w.Invalidate()
}

func (u *gioUI) dialog(title, body string, buttons []string, onClick func(int) bool) int {
	s := &screen{kind: kindDialog, title: title, body: body, buttons: buttons,
		clicks: make([]widget.Clickable, len(buttons)), result: make(chan int, 1), onClick: onClick}
	u.show(s)
	select {
	case i := <-s.result:
		return i
	case <-u.closed:
		return -1
	}
}

func (u *gioUI) Progress(title string, t *progress.Tracker, d Display) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	s := &screen{kind: kindProgress, title: title, tracker: t, display: d}
	if d.JokeText {
		s.joke = fun.NewJokeText(d.JokeLines, rng)
	}
	if d.ChaosBar {
		s.chaos = fun.NewChaosBar(rng)
	}
	u.show(s)
}

func (u *gioUI) Confirm(title, body, okLabel, cancelLabel string) bool {
	return u.dialog(title, body, []string{okLabel, cancelLabel}, nil) == 0
}

func (u *gioUI) Choose(title, body string, options []string) (int, bool) {
	i := u.dialog(title, body, options, nil)
	return i, i >= 0
}

func (u *gioUI) Info(title, body string, buttons ...string) int {
	if len(buttons) == 0 {
		buttons = []string{"OK"}
	}
	return u.dialog(title, body, buttons, nil)
}

func (u *gioUI) Error(err error, logDir string) {
	u.dialog("Fehler", apperr.Message(err), []string{"Log-Ordner öffnen", "Schließen"}, func(i int) bool {
		if i == 0 {
			if u.open != nil {
				_ = u.open(logDir) // nothing more useful to do if Explorer fails
			}
			return false
		}
		return true
	})
}

func (u *gioUI) Close() {
	u.mu.Lock()
	w := u.w
	u.mu.Unlock()
	if w != nil {
		w.Perform(system.ActionClose)
	}
}

func (u *gioUI) layout(gtx layout.Context, th *material.Theme) {
	paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
	u.mu.Lock()
	s := u.cur
	u.mu.Unlock()
	if s == nil {
		return
	}
	layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.H6(th, s.title)
				l.Color = fgColor
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		}
		if s.kind == kindProgress {
			children = append(children, u.progressChildren(gtx, th, s)...)
		} else {
			children = append(children, u.dialogChildren(gtx, th, s)...)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (u *gioUI) progressChildren(gtx layout.Context, th *material.Theme, s *screen) []layout.FlexChild {
	now := time.Now()
	snap := s.tracker.Snapshot()
	status := snap.Status
	if s.joke != nil {
		if l := s.joke.Line(now); l != "" {
			status = l
		}
	}
	value := snap.Percent
	if s.chaos != nil {
		dt := 33 * time.Millisecond
		if !s.last.IsZero() {
			dt = now.Sub(s.last)
		}
		value = s.chaos.Step(dt, snap)
	}
	s.last = now
	return []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Body1(th, status)
			l.Color = mutedColor
			l.MaxLines = 2
			return l.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			pb := material.ProgressBar(th, float32(value/100))
			pb.Color = accentColor
			pb.TrackColor = color.NRGBA{0x3a, 0x3a, 0x3a, 0xff}
			pb.Height = unit.Dp(18)
			return pb.Layout(gtx)
		}),
	}
}

func (u *gioUI) dialogChildren(gtx layout.Context, th *material.Theme, s *screen) []layout.FlexChild {
	for i := range s.clicks {
		if s.clicks[i].Clicked(gtx) {
			if s.onClick == nil || s.onClick(i) {
				select {
				case s.result <- i:
				default:
				}
			}
		}
	}
	buttons := make([]layout.FlexChild, 0, 2*len(s.buttons))
	for i := range s.buttons {
		i := i
		if i > 0 {
			buttons = append(buttons, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
		}
		buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			b := material.Button(th, &s.clicks[i], s.buttons[i])
			if i > 0 {
				b.Background = color.NRGBA{0x44, 0x44, 0x44, 0xff}
			}
			return b.Layout(gtx)
		}))
	}
	return []layout.FlexChild{
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			l := material.Body1(th, s.body)
			l.Color = fgColor
			return l.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, buttons...)
		}),
	}
}
