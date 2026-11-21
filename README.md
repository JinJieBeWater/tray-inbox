# TrayInbox

English | [简体中文](README.zh-Hans.md)

A macOS menu bar inbox for coding agents. TrayInbox shows the agents that [T3 Code](https://github.com/pingdotgg/t3code) runs on your Mac. The menu bar icon counts them by state: failed, blocked, done, idle and working. Click the icon to open the inbox, then click an agent to open its thread in T3 Code.

TrayInbox only reads. It never replies to an agent, approves a request or sends a notification, and it leaves T3 Code's read state as it is.

## Install

TrayInbox runs on Macs with Apple silicon. Install it with one command:

```sh
curl -fsSL https://github.com/JinJieBeWater/tray-inbox/releases/latest/download/install.sh | sh
```

The script checks the download's signature, installs TrayInbox in `/Applications` and opens it.

You can also download the disk image from the [latest release](https://github.com/JinJieBeWater/tray-inbox/releases/latest). The app has no Developer ID signature, so macOS blocks the first launch of a downloaded copy. Allow it in System Settings > Privacy & Security, or run:

```sh
xattr -dr com.apple.quarantine /Applications/TrayInbox.app
```

## Use

TrayInbox needs T3 Code running and its `t3` command installed. On first launch, TrayInbox uses `t3` to issue itself a read-only session and keeps the token in your keychain.

- Click the menu bar icon to open the inbox. Agents that wait on you are listed under Inbox, and running ones under Working.
- Right-click the icon for settings, which hold the language and Open at Login, and to check for updates.
- When T3 Code is closed or unreachable, the icon shows that it is disconnected and the inbox says why.

TrayInbox checks for updates once a day and installs them in the background. An update takes effect the next time TrayInbox starts.
