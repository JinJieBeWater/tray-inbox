# Changelog

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
