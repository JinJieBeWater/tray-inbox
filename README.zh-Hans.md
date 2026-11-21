# TrayInbox

[English](README.md) | 简体中文

TrayInbox 是给编程 agent 用的 macOS 菜单栏收件箱。它显示 [T3 Code](https://github.com/pingdotgg/t3code) 在你的 Mac 上运行的 agent，菜单栏图标按状态计数：失败、受阻、完成、空闲和进行中。点击图标打开收件箱，再点击某个 agent，就会在 T3 Code 里打开它的会话。

TrayInbox 只读取状态。它不回复 agent，不批准请求，不发通知，也不改动 T3 Code 的已读状态。

## 安装

TrayInbox 支持 Apple 芯片的 Mac。用一条命令安装：

```sh
curl -fsSL https://github.com/JinJieBeWater/tray-inbox/releases/latest/download/install.sh | sh
```

这个脚本会校验下载文件的签名，把 TrayInbox 装进 `/Applications` 并打开它。

也可以从 [最新版本](https://github.com/JinJieBeWater/tray-inbox/releases/latest) 下载磁盘映像。应用没有 Developer ID 签名，所以下载的副本第一次启动会被 macOS 拦下。可以在「系统设置 > 隐私与安全性」里允许打开，或者运行：

```sh
xattr -dr com.apple.quarantine /Applications/TrayInbox.app
```

## 使用

TrayInbox 需要 T3 Code 正在运行，并且装好了 `t3` 命令。第一次启动时，TrayInbox 用 `t3` 给自己签发一个只读会话，把 token 存进钥匙串。

- 点击菜单栏图标打开收件箱。等你处理的 agent 列在「收件箱」下，正在运行的列在「进行中」下。没有任何 agent 时，图标显示 T3，点击直接切回 T3 Code。
- 右键点击图标，可以打开「设置…」调整语言和开机启动，也可以选「检查更新…」。
- T3 Code 关闭或连不上时，图标会显示已断开，收件箱里写明原因。

TrayInbox 每天在后台检查一次更新并自动安装，下次启动时生效。
