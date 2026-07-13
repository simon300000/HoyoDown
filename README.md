# HoyoDown

English | [中文](readme_cn.md)

HoyoDown is a Go command-line tool for browsing and downloading official miHoYo/HoYoverse PC game files and voice packs. It supports Sophon manifests and chunks, automatic CN/Global endpoint selection, package inspection, individual file downloads, and machine-readable JSON output.

## Build

```bash
go build -o hoyodown ./cmd/hoyodown
```

On Windows, you may name the output `hoyodown.exe`:

```powershell
go build -o hoyodown.exe ./cmd/hoyodown
```

## Browse games and packages

The `game` command is hierarchical. Add one positional argument at a time to move from games to versions, packages, files, and downloads.

```bash
# List games
./hoyodown game

# List the officially available versions
./hoyodown game hk4e

# List game, voice, and incremental packages for a version
./hoyodown game hk4e 6.7.0

# Show the package file count and total restored size
./hoyodown game hk4e 6.7.0 zh-cn
```

Use `--json` when the complete file list or structured output is needed:

```bash
./hoyodown game --json
./hoyodown game hk4e --json
./hoyodown game hk4e 6.7.0 --json
./hoyodown game hk4e 6.7.0 zh-cn --json
```

Every command level supports `--help`.

## Download

Download and reconstruct an entire package:

```bash
./hoyodown game hk4e 6.7.0 zh-cn ./genshin
```

Download one file from the package:

```bash
./hoyodown game hk4e 6.7.0 zh-cn ./genshin \
  YuanShen_Data/StreamingAssets/AudioAssets/Chinese/External0.pck
```

Resolve the manifest and calculate the download without writing files:

```bash
./hoyodown game nap 3.0.0 zh-cn ./zzz-voice --dry-run
```

## Sophon commands

Download a complete Sophon package:

```bash
./hoyodown sophon full gopR6Cufr3 zh-cn 6.7.0 ./output --region OSREL
```

Download an incremental update:

```bash
./hoyodown sophon update gopR6Cufr3 game 6.6.0 6.7.0 ./output --region OSREL
```

The Sophon commands take an official region-specific game ID instead of a short game code. They also support `--launcherId`, `--platApp`, `--handles`, `--threads`, `--branch`, `--silent`, and `--dry-run` for customized downloader workflows.

## Options

- `--region auto|cn|global`: Select an API/CDN region. `auto` probes both official endpoints and uses the lower-latency reachable endpoint.
- `--branch main|predownload`: Select the released or pre-download Sophon branch.
- `--threads N`: Set the number of concurrent Sophon file downloads.
- `--yes`: Start downloading without an interactive confirmation prompt.
- `--silent`: Suppress normal output, progress messages, and the interactive download confirmation. This is intended for scripts that only use the process exit status.
- `--dry-run`: Resolve APIs and manifests without writing files.
- `--json`: Print machine-readable JSON. At the package level, JSON includes the complete file list.
- `-h`, `--help`: Show help for the current command level.

## Game and package names

Games:

- `hk4e`: Genshin Impact
- `hkrpg`: Honkai: Star Rail
- `nap`: Zenless Zone Zero
- `bh3`: Honkai Impact 3rd

Packages:

- `game`: Base game files
- `zh-cn`: Chinese voice pack
- `en-us`: English voice pack
- `ja-jp`: Japanese voice pack
- `ko-kr`: Korean voice pack

## Acknowledgements

- cocogoat — [YuehaiTeam/cocogoat](https://github.com/YuehaiTeam/cocogoat)
- hoyoUpdates — [Escartem/HoyoUpdates](https://github.com/Escartem/HoyoUpdates)
- hoyo-files — [orilights/hoyo-files](https://github.com/orilights/hoyo-files), [orilights/pkg_version](https://github.com/orilights/pkg_version)
- SophonDownloader — [Escartem/SophonDownloader](https://github.com/Escartem/SophonDownloader)
