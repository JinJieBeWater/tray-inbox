// Package settings is the settings window's view. The system keeps both
// choices (the app's language and the login item); TrayInbox stores neither.
package settings

import (
	"slices"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/jinjiebewater/tray-inbox/locales"
)

// System keeps both settings; *mygo.Application is one.
type System interface {
	Languages() []string // tags the app is written in
	ChosenLocale() string
	SetLocale(tag string) // "" follows the system
	OpenAtLogin() bool
	SetOpenAtLogin(on bool) error
}

// Settings' fields are used only on the main thread.
type Settings struct {
	sys      System
	lang     string
	login    bool
	loginErr string
}

func New(sys System) *Settings {
	return &Settings{sys: sys, login: sys.OpenAtLogin()}
}

func (s *Settings) View(c *ui.Context) {
	tags := append([]string{""}, s.sys.Languages()...)
	labels := make([]string, len(tags))
	for i, t := range tags {
		labels[i] = i18n.Autonym(t)
	}
	labels[0] = locales.FollowSystem()
	if i := slices.Index(tags, s.sys.ChosenLocale()); i >= 0 {
		s.lang = labels[i]
	}

	ui.Column(c).Padding(20, 24).AlignItems(ui.Stretch).Children(func() {
		ui.Form(c, func() {
			ui.Field(c, locales.Language(), func() {
				if ui.Select(c, &s.lang, labels).Changed() {
					if i := slices.Index(labels, s.lang); i >= 0 {
						s.sys.SetLocale(tags[i])
					}
				}
			})
			ui.Field(c, locales.OpenAtLogin(), func() {
				if ui.Switch(c, &s.login).Changed() {
					s.loginErr = ""
					if err := s.sys.SetOpenAtLogin(s.login); err != nil {
						s.loginErr = err.Error()
					}
					s.login = s.sys.OpenAtLogin()
				}
			}).Error(s.loginErr)
		})
	})
}
