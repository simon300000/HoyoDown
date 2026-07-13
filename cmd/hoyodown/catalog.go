package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type gameItem struct {
	Code   string `json:"code"`
	ID     string `json:"id"`
	Biz    string `json:"biz"`
	Name   string `json:"name"`
	Region string `json:"region"`
	Next   string `json:"-"`
}
type versionItem struct {
	Version    string   `json:"version"`
	Protocol   string   `json:"protocol"`
	Branch     string   `json:"branch,omitempty"`
	UpdateFrom []string `json:"update_from,omitempty"`
	Next       string   `json:"-"`
}
type packageItem struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Protocol    string `json:"protocol"`
	Version     string `json:"version"`
	FromVersion string `json:"from_version,omitempty"`
	Size        int64  `json:"size,omitempty"`
	URL         string `json:"url,omitempty"`
	MD5         string `json:"md5,omitempty"`
	Next        string `json:"-"`
}
type fileItem struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	MD5  string `json:"md5"`
	Next string `json:"-"`
}
type fileListResult struct {
	Game      string     `json:"game"`
	Version   string     `json:"version"`
	Package   string     `json:"package"`
	Region    string     `json:"region"`
	FileCount int        `json:"file_count"`
	TotalSize int64      `json:"total_size"`
	Files     []fileItem `json:"files"`
}

type gamesRoot struct {
	Retcode int    `json:"retcode"`
	Message string `json:"message"`
	Data    struct {
		Games []struct {
			ID      string `json:"id"`
			Biz     string `json:"biz"`
			Display struct {
				Name string `json:"name"`
			} `json:"display"`
		} `json:"games"`
	} `json:"data"`
}
type packagesRoot struct {
	Retcode int    `json:"retcode"`
	Message string `json:"message"`
	Data    struct {
		GamePackages []gamePackages `json:"game_packages"`
	} `json:"data"`
}
type gamePackages struct {
	Game        branchesGame  `json:"game"`
	Main        packageBranch `json:"main"`
	PreDownload packageBranch `json:"pre_download"`
}
type packageBranch struct {
	Major   *packageVersion  `json:"major"`
	Patches []packageVersion `json:"patches"`
}
type packageVersion struct {
	Version   string        `json:"version"`
	GamePkgs  []httpPackage `json:"game_pkgs"`
	AudioPkgs []httpPackage `json:"audio_pkgs"`
}
type httpPackage struct {
	Language         string       `json:"language"`
	URL              string       `json:"url"`
	MD5              string       `json:"md5"`
	Size             numberString `json:"size"`
	DecompressedSize numberString `json:"decompressed_size"`
}

func runGame(ctx context.Context, args []string, o opts) error {
	ep, e := pick(ctx, o.region, false)
	if e != nil {
		return e
	}
	c := makeHTTPClient(128)
	switch len(args) {
	case 0:
		return listGames(ctx, c, ep, o)
	case 1:
		return listVersions(ctx, c, ep, args[0], o)
	case 2:
		return listPackages(ctx, c, ep, args[0], normalizeVersion(args[1]), o)
	case 3:
		return listFiles(ctx, c, ep, args[0], normalizeVersion(args[1]), args[2], o)
	default:
		file := ""
		if len(args) > 4 {
			file = args[4]
		}
		return downloadSelection(ctx, c, ep, args[0], normalizeVersion(args[1]), args[2], args[3], file, o)
	}
}

func listGames(ctx context.Context, c *http.Client, ep endpoint, o opts) error {
	var r gamesRoot
	if e := getJSON(ctx, c, ep.API+"/hyp/hyp-connect/api/getGames?launcher_id="+ep.Launcher+"&language=en-us", &r); e != nil {
		return e
	}
	if r.Retcode != 0 {
		return fmt.Errorf("getGames: %s", r.Message)
	}
	out := make([]gameItem, 0, len(r.Data.Games))
	for _, g := range r.Data.Games {
		code := shortCode(g.Biz)
		out = append(out, gameItem{code, g.ID, g.Biz, g.Display.Name, ep.Name, "hoyodown game " + code})
	}
	if o.json {
		return emitJSON(out)
	}
	fmt.Printf("%-8s %-12s %s\n", "GAME", "ID", "NAME")
	for _, x := range out {
		fmt.Printf("%-8s %-12s %s\n", x.Code, x.ID, x.Name)
	}
	fmt.Println("\n./hoyodown game [game]")
	return nil
}

func listVersions(ctx context.Context, c *http.Client, ep endpoint, game string, o opts) error {
	gid := resolveGameID(code(ep), game)
	br, be := fetchBranches(ctx, c, ep, gid)
	pk, _ := fetchPackages(ctx, c, ep, gid)
	seen := map[string]bool{}
	var out []versionItem
	add := func(x versionItem) {
		if x.Version != "" && !seen[x.Protocol+":"+x.Branch+":"+x.Version] {
			seen[x.Protocol+":"+x.Branch+":"+x.Version] = true
			out = append(out, x)
		}
	}
	if be == nil {
		for _, g := range br.Data.GameBranches {
			add(versionItem{g.Main.Tag, "sophon", "main", g.Main.DiffTags, "hoyodown game " + game + " " + g.Main.Tag})
			if g.PreDownload.Tag != "" {
				add(versionItem{g.PreDownload.Tag, "sophon", "predownload", g.PreDownload.DiffTags, "hoyodown game " + game + " " + g.PreDownload.Tag + " --branch predownload"})
			}
		}
	}
	for _, g := range pk.Data.GamePackages {
		collectPackageVersions(g.Main, "main", game, &out, seen)
		collectPackageVersions(g.PreDownload, "predownload", game, &out, seen)
	}
	if len(out) == 0 && be != nil {
		return be
	}
	sort.SliceStable(out, func(i, j int) bool { return versionLess(out[j].Version, out[i].Version) })
	if o.json {
		return emitJSON(out)
	}
	fmt.Printf("%-10s %-12s %-12s %s\n", "VERSION", "PROTOCOL", "BRANCH", "UPDATE FROM")
	for _, x := range out {
		fmt.Printf("%-10s %-12s %-12s %s\n", x.Version, x.Protocol, x.Branch, strings.Join(x.UpdateFrom, ","))
	}
	fmt.Printf("\n./hoyodown game %s [version]\n", game)
	return nil
}

func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var ai, bi int
		if i < len(pa) {
			ai, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			bi, _ = strconv.Atoi(pb[i])
		}
		if ai != bi {
			return ai < bi
		}
	}
	return a < b
}
func collectPackageVersions(b packageBranch, branch, game string, out *[]versionItem, seen map[string]bool) {
	if b.Major != nil {
		key := "http:" + branch + ":" + b.Major.Version
		if !seen[key] {
			seen[key] = true
			*out = append(*out, versionItem{b.Major.Version, "http-archive", branch, nil, "hoyodown game " + game + " " + b.Major.Version})
		}
	}
	for _, p := range b.Patches {
		key := "http:" + branch + ":" + p.Version
		if !seen[key] {
			seen[key] = true
			*out = append(*out, versionItem{p.Version, "http-archive", branch, nil, "hoyodown game " + game + " " + p.Version})
		}
	}
}

func listPackages(ctx context.Context, c *http.Client, ep endpoint, game, ver string, o opts) error {
	out, e := packagesFor(ctx, c, ep, game, ver, o.branch)
	if e != nil {
		return e
	}
	if o.json {
		return emitJSON(out)
	}
	fmt.Printf("%-12s %-12s %-14s %-10s %-10s %s\n", "PACKAGE", "KIND", "PROTOCOL", "VERSION", "FROM", "SIZE")
	for _, x := range out {
		size := "-"
		if x.Size > 0 {
			size = formatSize(float64(x.Size))
		}
		fmt.Printf("%-12s %-12s %-14s %-10s %-10s %s\n", x.Name, x.Kind, x.Protocol, x.Version, x.FromVersion, size)
	}
	fmt.Printf("\n./hoyodown game %s %s [package]\n", game, ver)
	fmt.Println("Package names include game, zh-cn, en-us, ja-jp, ko-kr and update:<from>:<package>.")
	return nil
}

func packagesFor(ctx context.Context, c *http.Client, ep endpoint, game, ver, branch string) ([]packageItem, error) {
	gid := resolveGameID(code(ep), game)
	var out []packageItem
	br, _ := fetchBranches(ctx, c, ep, gid)
	for _, g := range br.Data.GameBranches {
		b := g.Main
		if branch == "predownload" {
			b = g.PreDownload
		}
		if b.Tag == ver {
			for _, cat := range b.Categories {
				out = append(out, packageItem{cat.MatchingField, packageKind(cat.MatchingField), "sophon", ver, "", 0, "", "", "hoyodown game " + game + " " + ver + " " + cat.MatchingField})
			}
			for _, from := range b.DiffTags {
				for _, cat := range b.Categories {
					name := "update:" + from + ":" + cat.MatchingField
					out = append(out, packageItem{name, "incremental", "sophon", ver, from, 0, "", "", ""})
				}
			}
		}
	}
	pk, _ := fetchPackages(ctx, c, ep, gid)
	for _, g := range pk.Data.GamePackages {
		pb := g.Main
		if branch == "predownload" {
			pb = g.PreDownload
		}
		appendHTTPPackages(&out, pb.Major, ver, "full", game)
		for i := range pb.Patches {
			p := &pb.Patches[i]
			if p.Version == ver {
				appendHTTPPackages(&out, p, ver, "update", game)
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("version %s not found for %s in %s", ver, game, ep.Name)
	}
	return out, nil
}
func appendHTTPPackages(out *[]packageItem, p *packageVersion, ver, kind, game string) {
	if p == nil || p.Version != ver {
		return
	}
	for i, x := range p.GamePkgs {
		n := "game"
		if len(p.GamePkgs) > 1 {
			n = fmt.Sprintf("game.%03d", i+1)
		}
		*out = append(*out, packageItem{n, kind, "http-archive", ver, "", int64(x.Size), x.URL, x.MD5, ""})
	}
	for _, x := range p.AudioPkgs {
		*out = append(*out, packageItem{x.Language, kind, "http-archive", ver, "", int64(x.Size), x.URL, x.MD5, ""})
	}
}

func listFiles(ctx context.Context, c *http.Client, ep endpoint, game, ver, pkg string, o opts) error {
	if strings.HasPrefix(pkg, "update:") {
		return errors.New("incremental package file listing is available through the advanced update command")
	}
	assets, total, e := sophonAssets(ctx, c, ep, game, ver, pkg, o.branch)
	if e != nil {
		return fmt.Errorf("file listing requires a Sophon package: %w", e)
	}
	out := make([]fileItem, 0, len(assets))
	for _, a := range assets {
		if a.IsDir {
			continue
		}
		out = append(out, fileItem{a.Name, a.Size, a.Hash, fmt.Sprintf("hoyodown game %s %s %s <output> %s", game, ver, pkg, a.Name)})
	}
	if o.json {
		return emitJSON(fileListResult{game, ver, pkg, ep.Name, len(out), total, out})
	}
	fmt.Printf("Game:       %s\n", game)
	fmt.Printf("Version:    %s\n", ver)
	fmt.Printf("Package:    %s\n", pkg)
	fmt.Printf("Region:     %s\n", ep.Name)
	fmt.Printf("Files:      %d\n", len(out))
	fmt.Printf("Total size: %s (%d bytes)\n", formatSize(float64(total)), total)
	fmt.Printf("\n./hoyodown game %s %s %s [output] [file]\n", game, ver, pkg)
	return nil
}

func downloadSelection(ctx context.Context, c *http.Client, ep endpoint, game, ver, pkg, outDir, file string, o opts) error {
	if strings.HasPrefix(pkg, "update:") {
		return errors.New("use: hoyodown sophon update <game-id> <package> <from> <to> <output>")
	}
	assets, total, e := sophonAssets(ctx, c, ep, game, ver, pkg, o.branch)
	if e != nil {
		return fmt.Errorf("download currently requires a Sophon package: %w", e)
	}
	if file != "" {
		found := assets[:0]
		for _, a := range assets {
			actual, requested := filepath.ToSlash(a.Name), filepath.ToSlash(file)
			// CN builds use YuanShen_Data while global builds use
			// GenshinImpact_Data. Accept the user-facing path from either region
			// when the path below the Unity data root is an exact match.
			sameDataPath := strings.Contains(actual, "_Data/") && strings.Contains(requested, "_Data/") && strings.SplitN(actual, "_Data/", 2)[1] == strings.SplitN(requested, "_Data/", 2)[1]
			if actual == requested || sameDataPath {
				found = append(found, a)
				total = a.Size
			}
		}
		assets = found
		if len(assets) == 0 {
			return fmt.Errorf("file %q not found in package %s", file, pkg)
		}
	}
	summary := map[string]any{"game": game, "version": ver, "package": pkg, "output": outDir, "file": file, "files": len(assets), "bytes": total, "region": ep.Name}
	if o.json {
		if e := emitJSON(summary); e != nil {
			return e
		}
	} else {
		fmt.Printf("Download: %d file(s), %s -> %s\n", len(assets), formatSize(float64(total)), outDir)
	}
	if o.dry {
		return nil
	}
	if !o.yes {
		fmt.Print("Continue? [y/N]: ")
		var answer string
		fmt.Fscan(os.Stdin, &answer)
		if strings.ToLower(answer) != "y" && strings.ToLower(answer) != "yes" {
			return nil
		}
	}
	threads := o.threads
	if threads < 1 {
		threads = runtime.NumCPU()
	}
	return downloadAssets(ctx, c, assets, outDir, threads, total, total, o.silent)
}

func sophonAssets(ctx context.Context, c *http.Client, ep endpoint, game, ver, pkg, branch string) ([]asset, int64, error) {
	br, e := parseBranch(branch)
	if e != nil {
		return nil, 0, e
	}
	s := newSophonURL(ep.Region, resolveGameID(code(ep), game), br, ep.Launcher, ep.PlatApp)
	if e = s.getBuildData(ctx, c); e != nil {
		return nil, 0, e
	}
	selected, e := s.branchBackup.selectBranch(br)
	if e != nil {
		return nil, 0, e
	}
	if selected.Tag != ver {
		return nil, 0, fmt.Errorf("Sophon branch version is %s, not %s", selected.Tag, ver)
	}
	u, e := s.buildURL(ver, false, false)
	if e != nil {
		return nil, 0, e
	}
	a, total, _, e := getAssetsFromManifests(ctx, c, pkg, u, "")
	return a, total, e
}
func fetchBranches(ctx context.Context, c *http.Client, ep endpoint, gid string) (branchesRoot, error) {
	s := newSophonURL(ep.Region, gid, branchMain, ep.Launcher, ep.PlatApp)
	e := s.getBuildData(ctx, c)
	return s.branchBackup, e
}
func fetchPackages(ctx context.Context, c *http.Client, ep endpoint, gid string) (packagesRoot, error) {
	var r packagesRoot
	e := getJSON(ctx, c, ep.API+"/hyp/hyp-connect/api/getGamePackages?game_ids%5B%5D="+gid+"&launcher_id="+ep.Launcher, &r)
	if e == nil && r.Retcode != 0 {
		e = fmt.Errorf("getGamePackages: %s", r.Message)
	}
	return r, e
}
func shortCode(s string) string {
	s = strings.TrimSuffix(s, "_global")
	return strings.TrimSuffix(s, "_cn")
}
func packageKind(s string) string {
	if s == "game" {
		return "base-game"
	}
	return "voice"
}
