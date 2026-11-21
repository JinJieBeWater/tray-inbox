package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/jinjiebewater/tray-inbox/internal/settings"
	"github.com/jinjiebewater/tray-inbox/locales"
)

// settingsWindow is at most one settings window. Main thread only.
type settingsWindow struct{ win *mygo.Window }

func newSettingsWindow() *settingsWindow {
	s := &settingsWindow{}
	mygo.App.OnLocaleChanged(func(string) {
		if s.win != nil {
			s.win.SetTitle(locales.Settings())
		}
	})
	return s
}

func (s *settingsWindow) Open() {
	mygo.App.Focus()
	if s.win != nil {
		s.win.Focus()
		return
	}
	view := settings.New(mygo.App)
	s.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             locales.Settings(),
		Width:             420,
		Height:            180, // room for the language list to open below the select
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		Content:           ui.View(view.View),
	})
	s.win.OnClosed(func() { s.win = nil })
}
