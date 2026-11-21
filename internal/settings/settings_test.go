package settings

import (
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

func TestMain(m *testing.M) {
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	os.Exit(m.Run())
}

type harness struct {
	*testing.T
	tt      *ui.Tester
	chosen  string
	login   bool
	loginEr error
	sets    []string
	logins  []bool
}

func (h *harness) Languages() []string  { return []string{"en", "zh-Hans"} }
func (h *harness) ChosenLocale() string { return h.chosen }
func (h *harness) SetLocale(tag string) { h.sets = append(h.sets, tag); h.chosen = tag }
func (h *harness) OpenAtLogin() bool    { return h.login }
func (h *harness) SetOpenAtLogin(on bool) error {
	h.logins = append(h.logins, on)
	if h.loginEr != nil {
		return h.loginEr
	}
	h.login = on
	return nil
}

func open(t *testing.T, chosen string, login bool) *harness {
	h := &harness{T: t, chosen: chosen, login: login}
	h.tt = ui.NewTester(New(h).View, 420, 240)
	return h
}

func (h *harness) click(s string) {
	h.Helper()
	if err := h.tt.Click(s); err != nil {
		h.Fatalf("click %q: %v; texts %q", s, err, h.tt.Texts())
	}
}

func (h *harness) shows(s string, want bool) {
	h.Helper()
	if got := slices.Contains(h.tt.Texts(), s); got != want {
		h.Errorf("shows %q: %v, want %v; texts %q", s, got, want, h.tt.Texts())
	}
}

func TestLanguageShowsTheChosenOne(t *testing.T) {
	open(t, "", false).shows("系统默认", true)
	open(t, "en", false).shows("English", true)
	open(t, "zh-Hans", false).shows("简体中文", true)
}

func TestChoosingALanguageSetsIt(t *testing.T) {
	h := open(t, "", false)
	h.click("系统默认")
	h.click("English")
	h.click("English")
	h.click("系统默认")
	if want := []string{"en", ""}; !slices.Equal(h.sets, want) {
		t.Fatalf("SetLocale %q, want %q", h.sets, want)
	}
}

func TestOpenAtLoginTurnsOnAndOff(t *testing.T) {
	h := open(t, "", false)
	h.click("开机启动")
	h.click("开机启动")
	if want := []bool{true, false}; !slices.Equal(h.logins, want) {
		t.Fatalf("SetOpenAtLogin %v, want %v", h.logins, want)
	}
}

func TestOpenAtLoginFailureShowsWhyAndKeepsTheSystemState(t *testing.T) {
	h := open(t, "", false)
	h.loginEr = errors.New("needs an app bundle")
	h.click("开机启动")
	h.shows("needs an app bundle", true)
	h.loginEr = nil
	h.click("开机启动")
	if want := []bool{true, true}; !slices.Equal(h.logins, want) {
		t.Fatalf("SetOpenAtLogin %v, want %v: a failed turn-on leaves it off", h.logins, want)
	}
	h.shows("needs an app bundle", false)
}
