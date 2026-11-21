// TrayInbox is a menu bar inbox of the agents that T3 Code runs.
package main

import (
	"context"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	updaternative "github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"

	"github.com/jinjiebewater/tray-inbox/internal/adapter/t3"
	"github.com/jinjiebewater/tray-inbox/internal/agent"
	"github.com/jinjiebewater/tray-inbox/internal/inbox"
	"github.com/jinjiebewater/tray-inbox/internal/system"
	"github.com/jinjiebewater/tray-inbox/internal/trayicon"
	"github.com/jinjiebewater/tray-inbox/locales"
)

type tokens struct{ keychain system.Keychain }

func (t tokens) Get() (string, error) { return t.keychain.Get("t3") }
func (t tokens) Set(s string) error   { return t.keychain.Set("t3", s) }

// app's fields are used only on the main thread, except adapters (see started).
type app struct {
	ctx      context.Context
	cancel   context.CancelFunc
	stopped  chan struct{}
	quitting bool

	started  chan struct{} // closed once adapters is set; adapters is read only after it
	adapters []agent.Adapter

	tray     *mygo.Tray
	icon     *trayicon.Icon
	flyout   *flyout
	settings *settingsWindow
	empty    bool
}

func main() {
	a := &app{stopped: make(chan struct{}), started: make(chan struct{})}
	a.ctx, a.cancel = context.WithCancel(context.Background())
	mygo.App.SetName("TrayInbox")
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.Use(updaternative.Plugin)
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnBeforeQuit(a.beforeQuit)
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *app) ready() {
	// Background checks install silently: an update window opening on its own would be a notification.
	updater.SetAutomaticDownloads(true)
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "trayinbox",
		Icon:           trayicon.Render(inbox.Counts{}, false, 2),
		IconIsTemplate: true,
	})
	if err != nil {
		log.Fatal("tray: ", err)
	}
	a.tray = tray
	a.icon = trayicon.New(func(png []byte) { _ = tray.SetIcon(png, true) })
	a.flyout = newFlyout(tray, a.jump)
	a.settings = newSettingsWindow()
	tray.OnClick(a.click)
	tray.OnRightClick(a.menu)
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { tray.Destroy() })

	go func() {
		defer close(a.stopped)
		a.adapters = []agent.Adapter{
			t3.New(t3.Config{
				LookT3:     func(ctx context.Context) string { return look(ctx, "t3") },
				Tokens:     tokens{system.Keychain{Service: "TrayInbox"}},
				OpenURL:    mygo.Shell.OpenExternal,
				OpenForPID: system.OpenForPID,
			}),
		}
		close(a.started)
		run(a.ctx, a.adapters, time.Now, a.publish)
	}()
}

func look(ctx context.Context, name string) string {
	path, err := system.LookPathContext(ctx, name)
	if err != nil {
		return ""
	}
	return path
}

// publish is called by the loop goroutine.
func (a *app) publish(v inbox.View) {
	mygo.RunOnMain(func() {
		a.empty = v.Counts == (inbox.Counts{}) && len(v.Disconnected) == 0
		a.icon.Update(v.Counts, len(v.Disconnected) > 0)
		a.flyout.SetView(v)
	})
}

func (a *app) click() {
	if a.flyout.IsOpen() {
		a.flyout.Close()
		return
	}
	if a.empty {
		go func() {
			if err := a.open(); err != nil && a.ctx.Err() == nil {
				log.Printf("open: %v", err)
				mygo.RunOnMain(a.flyout.Open)
			}
		}()
		return
	}
	a.flyout.Open()
}

// open may block; it is called off the main thread.
func (a *app) open() error {
	select {
	case <-a.started:
	case <-a.ctx.Done():
		return a.ctx.Err()
	}
	return openApp(a.ctx, a.adapters)
}

// jump may block; it is called off the main thread.
func (a *app) jump(id agent.ID) error {
	select {
	case <-a.started:
	case <-a.ctx.Done():
		return a.ctx.Err()
	}
	return jump(a.ctx, a.adapters, id)
}

func (a *app) menu() {
	a.flyout.Close()
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.OpenPanel(), Icon: menuIcon(`<polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/>`), Click: func(*mygo.MenuItem, *mygo.Window) { a.flyout.Open() }},
		{Label: locales.SettingsMenu(), Icon: menuIcon(`<path d="M9.671 4.136a2.34 2.34 0 0 1 4.659 0 2.34 2.34 0 0 0 3.319 1.915 2.34 2.34 0 0 1 2.33 4.033 2.34 2.34 0 0 0 0 3.831 2.34 2.34 0 0 1-2.33 4.033 2.34 2.34 0 0 0-3.319 1.915 2.34 2.34 0 0 1-4.659 0 2.34 2.34 0 0 0-3.32-1.915 2.34 2.34 0 0 1-2.33-4.033 2.34 2.34 0 0 0 0-3.831A2.34 2.34 0 0 1 6.35 6.051a2.34 2.34 0 0 0 3.319-1.915"/><circle cx="12" cy="12" r="3"/>`), Click: func(*mygo.MenuItem, *mygo.Window) { a.settings.Open() }},
		checkForUpdates(),
		mygo.Separator(),
		{Label: locales.Quit(), Icon: menuIcon(`<path d="M12 2v10"/><path d="M18.4 6.6a9 9 0 1 1-12.77.04"/>`), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	a.tray.SetMenu(menu)
	defer a.tray.SetMenu(nil)
	a.tray.PopUpMenu()
}

func checkForUpdates() *mygo.MenuItem {
	item := updater.MenuItem()
	item.Icon = menuIcon(`<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>`)
	return item
}

func menuIcon(body string) *mygo.MenuIcon {
	s := ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#000" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
	icon, err := mygo.NewMenuIconSVG(s, true)
	if err != nil {
		return nil
	}
	return icon
}

// beforeQuit holds the quit until the adapters have stopped, so no connection is left behind.
// The main thread stays free meanwhile: the loop may be waiting on it to publish.
func (a *app) beforeQuit(e *mygo.QuitEvent) {
	a.cancel()
	if a.quitting || e.SessionEnding {
		return
	}
	a.quitting = true
	e.PreventDefault()
	go func() {
		select {
		case <-a.stopped:
		case <-time.After(3 * time.Second):
		}
		mygo.App.Quit()
	}()
}
