package main

import (
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
	"github.com/jinjiebewater/tray-inbox/internal/inbox"
	"github.com/jinjiebewater/tray-inbox/internal/panel"
)

const (
	panelWidth  = 320
	panelHeight = 0.7 // of the work area
)

// flyout is the panel's window under the tray icon. Its methods run on the main thread;
// jump may block and is called from another goroutine.
type flyout struct {
	tray  *mygo.Tray
	jump  func(agent.ID) error
	panel *panel.Panel
	win   *mygo.Window
}

func newFlyout(tray *mygo.Tray, jump func(agent.ID) error) *flyout {
	f := &flyout{tray: tray, jump: jump}
	f.panel = panel.New(panel.Options{Now: time.Now, Jump: f.startJump, Close: f.Close})
	return f
}

func (f *flyout) SetView(v inbox.View) {
	f.panel.SetView(v)
	f.invalidate()
}

func (f *flyout) IsOpen() bool { return f.win != nil }

func (f *flyout) Open() {
	if f.win != nil {
		return
	}
	height := int(float64(f.workArea().Height) * panelHeight)
	f.panel.SetMaxHeight(height)
	f.win = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:         f.tray,
		Placement:    mygo.PlacementBottomStart,
		Gap:          4,
		Width:        panelWidth,
		Height:       height,
		FitContent:   true,
		MinWidth:     panelWidth,
		MaxWidth:     panelWidth,
		MaxHeight:    height,
		Focusable:    true,
		Shadow:       true,
		CornerRadius: 10,
		Hidden:       true,
		Content:      ui.View(f.panel.View),
	})
	f.win.OnClosed(func() {
		f.win = nil
		f.panel.Reset()
	})
	// MyGo fits a flyout before it shows only once it has drawn a frame, so a new one would show at Height first.
	// Capturing draws that frame without showing it.
	if _, err := f.win.CapturePage(); err != nil {
		log.Print("panel: ", err)
	}
	f.win.Show()
}

func (f *flyout) Close() {
	if f.win != nil {
		f.win.Close()
	}
}

func (f *flyout) startJump(id agent.ID, done func(error)) {
	go func() {
		err := f.jump(id)
		mygo.RunOnMain(func() {
			done(err)
			f.invalidate()
		})
	}()
}

func (f *flyout) invalidate() {
	if f.win != nil {
		f.win.Invalidate()
	}
}

func (f *flyout) workArea() mygo.Rectangle {
	b := f.tray.Bounds()
	if b.Width == 0 || b.Height == 0 {
		return mygo.Screen.Displays()[0].WorkArea
	}
	return mygo.Screen.DisplayNearestPoint(mygo.Point{X: b.X + b.Width/2, Y: b.Y + b.Height/2}).WorkArea
}
