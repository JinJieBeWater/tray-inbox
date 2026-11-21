package panel

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/jinjiebewater/tray-inbox/internal/agent"
	"github.com/jinjiebewater/tray-inbox/internal/inbox"
)

var t0 = time.Date(2026, 11, 20, 10, 0, 30, 0, time.UTC)

// The tests read Chinese; lang switches it for one test.
var lang = "zh-Hans"

func TestMain(m *testing.M) {
	i18n.SetLocaleFunc(func() string { return lang })
	os.Exit(m.Run())
}

func inEnglish(t *testing.T) {
	lang = "en"
	t.Cleanup(func() { lang = "zh-Hans" })
}

func TestSaysTheAppLanguage(t *testing.T) {
	inEnglish(t)
	b := row("a", agent.Blocked, "Review", 0)
	b.BlockedWhy = agent.WhyPermission
	w := row("c", agent.Working, "Waiting on task", 6*time.Minute)
	w.WorkingKind = agent.WorkWaiting
	h := open(t, inbox.View{
		Inbox:   []inbox.Row{b},
		Running: []inbox.Row{row("b", agent.Working, "Busy", 3*time.Minute), w},
	})
	h.shows("Inbox", true)
	h.shows("1 unread", true)
	h.shows("Approval", true)
	h.shows("Working", true)
	h.click("Working")
	h.shows("Waiting", true)
	h.shows("6m", true)
	if got := ago(2 * time.Hour); got != "2h" {
		t.Errorf("ago = %q, want 2h", got)
	}
	if got := ago(10 * time.Second); got != "now" {
		t.Errorf("ago(10s) = %q, want now", got)
	}
	if got := workingAgo(45 * time.Second); got != "45s" {
		t.Errorf("workingAgo(45s) = %q, want 45s", got)
	}
	if got := workingAgo(65 * time.Minute); got != "1h 5m" {
		t.Errorf("workingAgo(65m) = %q, want 1h 5m", got)
	}
}

type harness struct {
	*testing.T
	p      *Panel
	tt     *ui.Tester
	now    time.Time
	jumps  []agent.ID
	done   func(error)
	closed int
}

func open(t *testing.T, v inbox.View) *harness {
	h := &harness{T: t, now: t0}
	h.p = New(Options{
		Now: func() time.Time { return h.now },
		Jump: func(id agent.ID, done func(error)) {
			h.jumps = append(h.jumps, id)
			h.done = done
		},
		Close: func() { h.closed++ },
	})
	h.p.SetView(v)
	h.tt = ui.NewTester(h.p.View, 360, 640)
	return h
}

func (h *harness) advance(d time.Duration) {
	h.now = h.now.Add(d)
	h.tt.Advance(d)
}

func (h *harness) click(s string) {
	h.Helper()
	if err := h.tt.Click(s); err != nil {
		h.Fatal(err)
	}
}

func (h *harness) shows(s string, want bool) {
	h.Helper()
	if got := slices.Contains(h.tt.Texts(), s); got != want {
		h.Errorf("shows %q: %v, want %v; texts %q", s, got, want, h.tt.Texts())
	}
}

func row(native string, s agent.State, title string, ago time.Duration) inbox.Row {
	return inbox.Row{
		ID:    agent.ID{Orchestrator: "t3", Source: "local", Native: native},
		Info:  agent.Info{Project: "perch", Title: title},
		State: s,
		At:    t0.Add(-ago),
	}
}

func TestEmptyInboxSaysAllDone(t *testing.T) {
	open(t, inbox.View{}).shows("没有等你处理的任务", true)
	open(t, inbox.View{Running: []inbox.Row{row("a", agent.Working, "Busy", 0)}}).shows("没有等你处理的任务", true)
	open(t, inbox.View{Inbox: []inbox.Row{row("a", agent.Idle, "Seen", 0)}}).shows("没有等你处理的任务", false)
}

func TestNoAllDoneWhileASourceIsDisconnected(t *testing.T) {
	h := open(t, inbox.View{Disconnected: []inbox.Disconnect{{Orchestrator: "t3", Reason: agent.ReasonAppNotRunning}}})
	h.shows("没有等你处理的任务", false)
}

func TestInboxHeaderCountsUnread(t *testing.T) {
	h := open(t, inbox.View{Inbox: []inbox.Row{
		row("a", agent.Blocked, "Waits", 0), row("b", agent.Done, "Finished", 0), row("c", agent.Idle, "Seen", 0),
	}})
	h.shows("收件箱", true)
	h.shows("2 未读", true)
	h = open(t, inbox.View{Inbox: []inbox.Row{row("c", agent.Idle, "Seen", 0)}})
	h.shows("收件箱", true)
	for _, s := range h.tt.Texts() {
		if strings.HasSuffix(s, "未读") {
			t.Errorf("all read but shows %q", s)
		}
	}
}

func TestBlockedRowShowsWhyWithoutDuration(t *testing.T) {
	r := row("a", agent.Blocked, "Review the plan", 4*time.Minute)
	r.BlockedWhy = agent.WhyPermission
	h := open(t, inbox.View{Inbox: []inbox.Row{r}})
	h.shows("等你批准", true)
	h.shows("4 分钟", false)
	h.shows("Review the plan", true)
	h.shows("perch", true)
}

func TestStatusWords(t *testing.T) {
	for _, c := range []struct {
		state agent.State
		why   agent.BlockedWhy
		word  string
	}{
		{agent.Blocked, agent.WhyPermission, "等你批准"},
		{agent.Blocked, agent.WhyQuestion, "等你回答"},
		{agent.Blocked, agent.WhyRequest, "访客消息"},
		{agent.Blocked, agent.WhyLimited, "额度受限"},
		{agent.Blocked, agent.WhyWoke, "已唤醒"},
		{agent.Blocked, agent.WhyOther, "等你处理"},
		{agent.Error, "", "失败"},
		{agent.Done, "", "完成"},
	} {
		r := row("a", c.state, "T", time.Hour)
		r.BlockedWhy = c.why
		h := open(t, inbox.View{Inbox: []inbox.Row{r}})
		h.shows(c.word, true)
		h.shows("1 小时", false)
	}
	for _, c := range []struct {
		kind agent.WorkingKind
		word string
	}{
		{agent.WorkRunning, "进行中"},
		{agent.WorkPreparing, "准备中"},
		{agent.WorkGoal, "目标"},
		{agent.WorkWaiting, "等待中"},
	} {
		r := row("a", agent.Working, "T", 6*time.Minute)
		r.WorkingKind = c.kind
		h := open(t, inbox.View{Running: []inbox.Row{r}})
		h.shows(c.word, true)
		h.shows("6 分钟", true)
	}
	// Waiting with zero At (blockedOnThreadWait) shows Waiting without a duration.
	w := row("w", agent.Working, "T", 0)
	w.WorkingKind, w.At = agent.WorkWaiting, time.Time{}
	hw := open(t, inbox.View{Running: []inbox.Row{w}})
	hw.shows("等待中", true)
	hw.shows("0 秒", false)
	hw.shows("刚刚", false)

	h := open(t, inbox.View{Inbox: []inbox.Row{row("a", agent.Idle, "T", time.Hour)}})
	h.shows("1 小时", true)
	for _, w := range []string{"等你处理", "失败", "完成", "进行中", "等待中"} {
		h.shows(w, false)
	}
}

func TestWorkingAgoAndSecondRefresh(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{-time.Second, "0 秒"},
		{0, "0 秒"},
		{45 * time.Second, "45 秒"},
		{time.Minute, "1 分钟"},
		{59*time.Minute + 59*time.Second, "59 分钟"},
		{time.Hour, "1 小时 0 分钟"},
		{time.Hour + 5*time.Minute, "1 小时 5 分钟"},
	} {
		if got := workingAgo(c.d); got != c.want {
			t.Errorf("workingAgo(%v) = %q, want %q", c.d, got, c.want)
		}
	}
	h := open(t, inbox.View{Running: []inbox.Row{row("a", agent.Working, "T", 58*time.Second)}})
	h.shows("58 秒", true)
	h.advance(time.Second)
	h.shows("59 秒", true)
	h.advance(time.Second)
	h.shows("1 分钟", true)
	h.advance(59 * time.Second)
	h.shows("1 分钟", true)
	if h.tt.FramePending() {
		t.Error("asks for frames before the next minute after crossing 1m")
	}
}

func TestAgo(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{-time.Hour, "刚刚"},
		{0, "刚刚"},
		{59 * time.Second, "刚刚"},
		{time.Minute, "1 分钟"},
		{59*time.Minute + 59*time.Second, "59 分钟"},
		{time.Hour, "1 小时"},
		{23*time.Hour + 59*time.Minute, "23 小时"},
		{24 * time.Hour, "1 天"},
		{50 * time.Hour, "2 天"},
	} {
		if got := ago(c.d); got != c.want {
			t.Errorf("ago(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestDurationsRefreshAtTheMinute(t *testing.T) {
	h := open(t, inbox.View{Inbox: []inbox.Row{row("a", agent.Idle, "T", 30*time.Second)}})
	h.shows("刚刚", true)
	h.advance(29 * time.Second)
	h.shows("刚刚", true)
	h.advance(time.Second)
	h.shows("1 分钟", true)
	h.advance(time.Minute)
	h.shows("2 分钟", true)
	if h.tt.FramePending() {
		t.Error("asks for frames before the next minute")
	}
}

func TestRunningIsCollapsedUntilClicked(t *testing.T) {
	h := open(t, inbox.View{
		Inbox:   []inbox.Row{row("a", agent.Done, "Finished", time.Minute)},
		Running: []inbox.Row{row("b", agent.Working, "Busy one", 3*time.Minute), row("c", agent.Working, "Busy two", 0)},
	})
	h.shows("进行中", true)
	h.shows("2", true)
	h.shows("Busy one", false)
	h.click("进行中")
	h.shows("Busy one", true)
	h.shows("Busy two", true)
	h.shows("3 分钟", true)
	h.click("进行中")
	h.shows("Busy one", false)
	h.click("进行中")
	h.p.Reset()
	h.tt.Frame()
	h.shows("Busy one", false)
}

func TestRunningIsOpenWhileInboxIsEmpty(t *testing.T) {
	v := inbox.View{Running: []inbox.Row{row("b", agent.Working, "Busy one", 0)}}
	h := open(t, v)
	h.shows("Busy one", true)
	h.click("进行中")
	h.shows("Busy one", false)
	h.p.SetView(v)
	h.tt.Frame()
	h.shows("Busy one", false)
	h.p.Reset()
	h.tt.Frame()
	h.shows("Busy one", true)
	h.p.SetView(inbox.View{Inbox: []inbox.Row{row("a", agent.Done, "Finished", 0)}, Running: v.Running})
	h.tt.Frame()
	h.shows("Busy one", false)
}

func TestNoRunningHeaderWithoutRunningRows(t *testing.T) {
	h := open(t, inbox.View{Inbox: []inbox.Row{row("a", agent.Done, "Finished", 0)}})
	h.shows("进行中", false)
}

func TestClickJumpsAndClosesOnSuccess(t *testing.T) {
	r := row("a", agent.Done, "Finished", time.Minute)
	h := open(t, inbox.View{Inbox: []inbox.Row{r}})
	h.click("Finished")
	if len(h.jumps) != 1 || h.jumps[0] != r.ID {
		t.Fatalf("jumps %v, want [%v]", h.jumps, r.ID)
	}
	if h.closed != 0 {
		t.Fatal("closed before the jump finished")
	}
	h.done(nil)
	if h.closed != 1 {
		t.Errorf("closed %d times after the jump", h.closed)
	}
}

func TestSynchronousDone(t *testing.T) {
	h := open(t, inbox.View{Inbox: []inbox.Row{row("a", agent.Done, "Finished", 0)}})
	h.p.opts.Jump = func(id agent.ID, done func(error)) { done(errors.New("no app")) }
	h.click("Finished")
	h.shows("打不开", true)
}

func TestFailedJumpShowsCannotOpenUntilReset(t *testing.T) {
	h := open(t, inbox.View{Inbox: []inbox.Row{
		row("a", agent.Done, "Finished", 0),
		row("b", agent.Done, "Other", 0),
	}})
	h.click("Finished")
	h.done(errors.New("no app"))
	h.tt.Frame()
	if h.closed != 0 {
		t.Error("closed after a failed jump")
	}
	if n := strings.Count(strings.Join(h.tt.Texts(), "\n"), "打不开"); n != 1 {
		t.Errorf("打不开 shows %d times, want once; texts %q", n, h.tt.Texts())
	}
	h.p.Reset()
	h.tt.Frame()
	h.shows("打不开", false)
}

func TestDoneAfterResetIsIgnored(t *testing.T) {
	h := open(t, inbox.View{Inbox: []inbox.Row{row("a", agent.Done, "Finished", 0)}})
	h.click("Finished")
	h.p.Reset()
	h.done(errors.New("late"))
	h.tt.Frame()
	h.shows("打不开", false)
	h.click("Finished")
	first := h.done
	h.p.Reset()
	first(nil)
	if h.closed != 0 {
		t.Error("a jump from before Reset closed the panel")
	}
}

func TestDisconnectLines(t *testing.T) {
	for _, c := range []struct {
		reason agent.Reason
		text   string
	}{
		{agent.ReasonReconnecting, "断开，正在重连"},
		{agent.ReasonAppNotRunning, "应用没在运行"},
		{agent.ReasonNoCommand, "找不到命令"},
	} {
		h := open(t, inbox.View{Disconnected: []inbox.Disconnect{{Orchestrator: "t3", Reason: c.reason}}})
		h.shows(c.text, true)
	}
}

func TestThirdLine(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	a := row("a", agent.Done, "Local", 0)
	a.Where = home + "/my/perch"
	b := row("b", agent.Working, "Branch", 0)
	b.Where = "main"
	d := row("d", agent.Idle, "Elsewhere", 0)
	d.Where = home + "other"
	h := open(t, inbox.View{Inbox: []inbox.Row{a, d}, Running: []inbox.Row{b}})
	h.click("进行中")
	h.shows("~/my/perch", true)
	h.shows(home+"other", true)
	h.shows("main", true)
}

func TestNoProject(t *testing.T) {
	r := row("a", agent.Done, "Loose", 0)
	r.Project = ""
	h := open(t, inbox.View{Inbox: []inbox.Row{r}})
	h.shows("无项目", true)
}

func TestMonogramFollowsT3(t *testing.T) {
	for name, want := range map[string]string{
		"perch":     "PH",
		"herdr-t3":  "HT",
		"智能算量":      "智量",
		"Engineer":  "ER",
		"my app v2": "MV",
		"app2go":    "A2",
		" ｐｅｒｃｈ ":   "PH",
		"--":        "PR",
	} {
		if got := monogram(name); got != want {
			t.Errorf("monogram(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestTintFollowsT3(t *testing.T) {
	for name, want := range map[string][2]ui.Color{
		"perch":    {ui.Hex("#ea580c"), ui.Hex("#fb923c")},
		"herdr-t3": {ui.Hex("#7c3aed"), ui.Hex("#a78bfa")},
		"智能算量":     {ui.Hex("#dc2626"), ui.Hex("#f87171")},
		" Perch ":  {ui.Hex("#ea580c"), ui.Hex("#fb923c")},
	} {
		if light, dark := tint(name, false), tint(name, true); light != want[0] || dark != want[1] {
			t.Errorf("tint(%q) = %v %v, want %v", name, light, dark, want)
		}
	}
}

func TestEveryAgentKindHasAMark(t *testing.T) {
	for _, k := range agent.Kinds {
		if _, ok := marks[k]; !ok {
			t.Errorf("no mark for %q", k)
		}
	}
}
