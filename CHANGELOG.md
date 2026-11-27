# Changelog

## 0.1.4

- The Inbox and Working headers stay at the top of the panel while their threads scroll under them.
- The scroll bar is thinner and lighter, and no longer covers the highlighted row.

## 0.1.3

- Waiting threads in Working now fade like other working threads.

## 0.1.2

When no agents are listed, the menu bar icon shows T3 and clicks straight back to T3 Code.

- Replaced the empty menu bar icon with a bare T3 mark sized to match neighboring menu bar icons.
- Clicking the empty T3 icon activates the running T3 Code window instead of opening an empty panel. Right-click still opens the panel and settings.
- Activating T3 Code and jumping to a thread now target the running `.app` bundle from `server-runtime.json`, so multiple installed T3 builds do not conflict.
- TrayInbox now picks up the `t3` command if it is installed after startup.

## 0.1.1

Thread states, timers, and ordering now match T3 Code's sidebar.

- Added the Waiting, Setting up, Goal, Limited, Woke, and Request states. Waiting threads appear in Working and count toward Working in the menu bar.
- Visiting a failed thread in T3 Code now clears its Failed status.
- Removed subagent working counts and side chat rows. Pending approvals from delegated subagents and pending approvals or questions from side chats roll up onto their parent thread and wake snoozed parents.
- Matched T3 Code's working durations, 1-second refresh under a minute, and inbox and working sort order.

## 0.1.0

The first release: a menu bar inbox of the agents that T3 Code runs.

- The menu bar counts the agents by state: failed, blocked, done, idle and working. It shows when T3 Code is disconnected.
- Click the icon for the inbox; click an agent to jump to its thread in T3 Code.
- Updates install in the background once a day and take effect at the next launch.
