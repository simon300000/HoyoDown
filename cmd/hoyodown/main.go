package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type endpoint struct {
	Name, API, Launcher, PlatApp string
	Region                       region
}

var endpoints = []endpoint{{"cn", "https://hyp-api.mihoyo.com", "jGHBHlcOq1", "ddxf5qt290cg", regionCNREL}, {"global", "https://sg-hyp-api.hoyoverse.com", "VYTpXlbWo8", "ddxf6vlr1reo", regionOSREL}}

type opts struct {
	region, branch string
	threads        int
	yes, dry, json bool
}

func main() {
	a := os.Args[1:]
	if len(a) == 0 || a[0] == "help" || a[0] == "--help" || a[0] == "-h" {
		rootHelp()
		return
	}
	if a[0] == "sophon" {
		legacyMain(a[1:])
		return
	}
	if err := route(context.Background(), a); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func rootHelp() {
	fmt.Printf(`HoyoDown %s - browse and download official miHoYo/HoYoverse game files

USAGE
  hoyodown game [game [version [package [output [file]]]]] [flags]
  hoyodown endpoints
  hoyodown sophon full <game-id> <package> <version> <output> [options]
  hoyodown sophon update <game-id> <package> <from> <to> <output> [options]

Run "hoyodown game" and add one positional argument at a time to browse from
games to versions, packages, files and downloads.

GLOBAL FLAGS
  --region auto|cn|global  API/CDN region. auto probes both official APIs and
                           selects the lowest-latency reachable endpoint.
  --branch main|predownload
                           Select the released or pre-download Sophon branch.
  --threads N              Concurrent Sophon file downloads (default: CPU count).
  --yes                    Download without an interactive confirmation prompt.
  --dry-run                Resolve and parse metadata without writing files.
  --json                   Print machine-readable JSON instead of the table/help.
  -h, --help               Print help for the current command level.

SOPHON OPTIONS
  --region OSREL|CNREL    Select the region matching the supplied game ID.
  --launcherId ID         Override the launcher ID used by getGameBranches.
  --platApp ID            Override the platform application ID used by getBuild.
  --handles N             Maximum HTTP connections per host (default: 128).
  --silent                Suppress normal output, progress, and confirmation;
                          intended for scripts that use the process exit status.
`, version)
}

func parseOptions(a []string) ([]string, opts, bool, error) {
	o := opts{region: "auto", branch: "main"}
	var p []string
	help := false
	for i := 0; i < len(a); i++ {
		x := a[i]
		if !strings.HasPrefix(x, "-") {
			p = append(p, x)
			continue
		}
		k, v, has := splitOption(x)
		read := func() (string, error) {
			if has {
				return v, nil
			}
			if i+1 >= len(a) {
				return "", fmt.Errorf("%s requires a value", x)
			}
			i++
			return a[i], nil
		}
		switch k {
		case "region":
			v, e := read()
			if e != nil {
				return nil, o, false, e
			}
			o.region = v
		case "branch":
			v, e := read()
			if e != nil {
				return nil, o, false, e
			}
			o.branch = v
		case "threads":
			v, e := read()
			if e != nil {
				return nil, o, false, e
			}
			if _, e = fmt.Sscan(v, &o.threads); e != nil {
				return nil, o, false, fmt.Errorf("invalid --threads %q", v)
			}
		case "yes", "silent":
			o.yes = true
		case "dry-run":
			o.dry = true
		case "json":
			o.json = true
		case "h", "help":
			help = true
		default:
			return nil, o, false, fmt.Errorf("unknown option %s", x)
		}
	}
	return p, o, help, nil
}

func route(ctx context.Context, a []string) error {
	p, o, help, e := parseOptions(a)
	if e != nil {
		return e
	}
	if len(p) == 0 {
		rootHelp()
		return nil
	}
	if p[0] == "endpoints" {
		_, e = pick(ctx, "auto", true)
		return e
	}
	if p[0] != "game" {
		return fmt.Errorf("unknown command %q (try: hoyodown game)", p[0])
	}
	if help {
		gameHelp(p[1:])
		return nil
	}
	return runGame(ctx, p[1:], o)
}

func gameHelp(p []string) {
	switch len(p) {
	case 0:
		fmt.Print(`GAME LEVEL: list games
  ./hoyodown game [--region auto|cn|global] [--json]

  ./hoyodown game [game]
`)
	case 1:
		fmt.Printf(`VERSION LEVEL: list versions for %s
  ./hoyodown game %s [--region ...] [--json]

  ./hoyodown game %s [version]
`, p[0], p[0], p[0])
	case 2:
		fmt.Printf(`PACKAGE LEVEL: list packages for %s %s
  ./hoyodown game %s %s [--region ...] [--json]

  ./hoyodown game %s %s [package]
`, p[0], p[1], p[0], p[1], p[0], p[1])
	case 3:
		fmt.Printf(`FILE LEVEL: list files in package %s
  ./hoyodown game %s %s %s [--json]

  ./hoyodown game %s %s %s [output]
`, p[2], p[0], p[1], p[2], p[0], p[1], p[2])
	case 4:
		fmt.Printf(`DOWNLOAD LEVEL: download package %s into %s
  ./hoyodown game %s %s %s %s [--yes] [--dry-run]

  ./hoyodown game %s %s %s %s [file]
`, p[2], p[3], p[0], p[1], p[2], p[3], p[0], p[1], p[2], p[3])
	default:
		fmt.Printf(`SINGLE FILE LEVEL
  ./hoyodown game %s %s %s %s %s
`, p[0], p[1], p[2], p[3], p[4])
	}
}

func emitJSON(v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e == nil {
		fmt.Println(string(b))
	}
	return e
}
func code(e endpoint) string {
	if e.Region == regionCNREL {
		return "CNREL"
	}
	return "OSREL"
}
func pick(ctx context.Context, want string, verbose bool) (endpoint, error) {
	w := strings.ToLower(want)
	if w == "cn" || w == "cnrel" {
		return endpoints[0], nil
	}
	if w == "global" || w == "osrel" {
		return endpoints[1], nil
	}
	if w != "" && w != "auto" {
		return endpoint{}, fmt.Errorf("unknown region %q", want)
	}
	type result struct {
		ep      endpoint
		latency time.Duration
		err     error
	}
	ch := make(chan result, 2)
	for _, ep := range endpoints {
		go func(ep endpoint) {
			s := time.Now()
			c := http.Client{Timeout: 4 * time.Second}
			r, e := c.Get(ep.API + "/hyp/hyp-connect/api/getGames?launcher_id=" + ep.Launcher)
			if r != nil {
				r.Body.Close()
			}
			ch <- result{ep, time.Since(s), e}
		}(ep)
	}
	var ok []result
	for range endpoints {
		r := <-ch
		if verbose {
			if r.err != nil {
				fmt.Printf("%-6s unavailable (%v)\n", r.ep.Name, r.err)
			} else {
				fmt.Printf("%-6s %s\n", r.ep.Name, r.latency.Round(time.Millisecond))
			}
		}
		if r.err == nil {
			ok = append(ok, r)
		}
	}
	if len(ok) == 0 {
		return endpoint{}, errors.New("both CN and global APIs are unreachable")
	}
	sort.Slice(ok, func(i, j int) bool { return ok[i].latency < ok[j].latency })
	return ok[0].ep, nil
}
