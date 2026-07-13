# HoyoDown

[English](README.md) | 中文

HoyoDown 是一个使用 Go 编写的命令行工具，用于浏览和下载米哈游 / HoYoverse 官方 PC 游戏文件及语音包。支持 Sophon manifest 与分块下载、CN/Global 自动选线、包体浏览、单文件下载和 JSON 输出。

## 构建

```bash
go build -o hoyodown ./cmd/hoyodown
```

Windows 可以将输出文件命名为 `hoyodown.exe`：

```powershell
go build -o hoyodown.exe ./cmd/hoyodown
```

## 浏览游戏和包体

`game` 是一个层级命令。每增加一个位置参数，就依次进入游戏、版本、包体、文件和下载层级。

```bash
# 列出游戏
./hoyodown game

# 列出官方当前可获取的版本
./hoyodown game hk4e

# 列出指定版本的游戏本体、语音包和增量包
./hoyodown game hk4e 6.7.0

# 显示包内文件数和还原后的总大小
./hoyodown game hk4e 6.7.0 zh-cn
```

需要完整文件列表或结构化结果时使用 `--json`：

```bash
./hoyodown game --json
./hoyodown game hk4e --json
./hoyodown game hk4e 6.7.0 --json
./hoyodown game hk4e 6.7.0 zh-cn --json
```

每个命令层级都支持 `--help`。

## 下载

下载并还原整个包：

```bash
./hoyodown game hk4e 6.7.0 zh-cn ./genshin
```

只下载包内的一个文件：

```bash
./hoyodown game hk4e 6.7.0 zh-cn ./genshin \
  YuanShen_Data/StreamingAssets/AudioAssets/Chinese/External0.pck
```

只解析 manifest 并计算下载内容，不写入文件：

```bash
./hoyodown game nap 3.0.0 zh-cn ./zzz-voice --dry-run
```

## Sophon 命令

下载完整 Sophon 包：

```bash
./hoyodown sophon full gopR6Cufr3 zh-cn 6.7.0 ./output --region OSREL
```

下载增量更新：

```bash
./hoyodown sophon update gopR6Cufr3 game 6.6.0 6.7.0 ./output --region OSREL
```

Sophon 命令的游戏参数必须填写对应区域的官方游戏 ID，不接受游戏短代码。还可以使用 `--launcherId`、`--platApp`、`--handles`、`--threads`、`--branch`、`--silent` 和 `--dry-run` 定制下载流程。

## 选项

- `--region auto|cn|global`：选择 API/CDN 区域。`auto` 会探测两个官方端点，并选择可连接且延迟较低的端点。
- `--branch main|predownload`：选择正式发布或预下载 Sophon 分支。
- `--threads N`：设置并发下载的 Sophon 文件数量。
- `--yes`：不显示交互确认，直接开始下载。
- `--silent`：关闭普通输出、进度信息和下载确认，适合只依赖进程退出状态的脚本。
- `--dry-run`：解析 API 和 manifest，但不写入文件。
- `--json`：输出便于程序处理的 JSON；在包体层级会包含完整文件列表。
- `-h`、`--help`：显示当前命令层级的帮助。

## 游戏和包名

游戏：

- `hk4e`：原神
- `hkrpg`：崩坏：星穹铁道
- `nap`：绝区零
- `bh3`：崩坏3

包体：

- `game`：游戏本体
- `zh-cn`：中文语音包
- `en-us`：英文语音包
- `ja-jp`：日文语音包
- `ko-kr`：韩文语音包

## 致谢

- cocogoat — [YuehaiTeam/cocogoat](https://github.com/YuehaiTeam/cocogoat)
- hoyoUpdates — [Escartem/HoyoUpdates](https://github.com/Escartem/HoyoUpdates)
- hoyo-files — [orilights/hoyo-files](https://github.com/orilights/hoyo-files)、[orilights/pkg_version](https://github.com/orilights/pkg_version)
- SophonDownloader — [Escartem/SophonDownloader](https://github.com/Escartem/SophonDownloader)
