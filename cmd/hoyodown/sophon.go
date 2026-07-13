package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const version = "0.1.0"

type config struct {
	Action     string
	GameID     string
	Matching   string
	UpdateFrom string
	UpdateTo   string
	OutputDir  string
	Region     string
	Branch     string
	LauncherID string
	PlatApp    string
	Threads    int
	Handles    int
	Silent     bool
	DryRun     bool
	ShowHelp   bool
}

type region int

const (
	regionOSREL region = iota
	regionCNREL
)

type branchType int

const (
	branchMain branchType = iota
	branchPreDownload
)

var gameMap = map[string]map[string]string{
	"OSREL": {
		"nap":   "U5hbdsT9W7",
		"hkrpg": "4ziysqXOQ8",
		"hk4e":  "gopR6Cufr3",
		"bh3":   "5TIVvvcwtM",
	},
	"CNREL": {
		"nap":   "x6znKlJ0xK",
		"hkrpg": "64kMb5iAWu",
		"hk4e":  "1Z8W5NHUQb",
		"bh3":   "osvnlOc0S8",
	},
}

func legacyMain(args []string) {
	ctx := context.Background()
	cfg, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\nUse --help for usage.\n", err)
		os.Exit(1)
	}
	if !cfg.Silent {
		fmt.Printf("Sophon.Downloader v%s - Go rewrite\n", version)
	}
	if cfg.ShowHelp {
		printUsage(filepath.Base(os.Args[0]) + " sophon")
		return
	}
	if _, isAlias := gameMap["OSREL"][strings.ToLower(cfg.GameID)]; isAlias {
		fmt.Fprintf(os.Stderr, "Error: Sophon commands require an official game ID, not the short code %q\n", cfg.GameID)
		os.Exit(1)
	}
	if err := run(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (config, error) {
	cfg := config{
		Region:  "OSREL",
		Branch:  "main",
		Threads: runtime.NumCPU(),
		Handles: 128,
	}

	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") {
			key, value, hasValue := splitOption(arg)
			readValue := func() (string, error) {
				if hasValue {
					return value, nil
				}
				if i+1 >= len(args) {
					return "", fmt.Errorf("option %s requires a value", arg)
				}
				i++
				return args[i], nil
			}
			switch key {
			case "region":
				v, err := readValue()
				if err != nil {
					return cfg, err
				}
				cfg.Region = v
			case "branch":
				v, err := readValue()
				if err != nil {
					return cfg, err
				}
				cfg.Branch = v
			case "launcherId":
				v, err := readValue()
				if err != nil {
					return cfg, err
				}
				cfg.LauncherID = v
			case "platApp":
				v, err := readValue()
				if err != nil {
					return cfg, err
				}
				cfg.PlatApp = v
			case "threads":
				v, err := readValue()
				if err != nil {
					return cfg, err
				}
				n, err := strconv.Atoi(v)
				if err != nil {
					return cfg, fmt.Errorf("invalid --threads value %q", v)
				}
				cfg.Threads = n
			case "handles":
				v, err := readValue()
				if err != nil {
					return cfg, err
				}
				n, err := strconv.Atoi(v)
				if err != nil {
					return cfg, fmt.Errorf("invalid --handles value %q", v)
				}
				cfg.Handles = n
			case "silent":
				cfg.Silent = true
			case "dry-run":
				cfg.DryRun = true
			case "h", "help":
				cfg.ShowHelp = true
			default:
				return cfg, fmt.Errorf("unknown option %q", arg)
			}
			continue
		}
		positional = append(positional, arg)
	}

	if cfg.ShowHelp {
		return cfg, nil
	}
	if len(positional) == 0 {
		cfg.ShowHelp = true
		return cfg, nil
	}

	cfg.Action = strings.ToLower(positional[0])
	switch cfg.Action {
	case "full":
		if len(positional) < 5 {
			cfg.ShowHelp = true
			return cfg, nil
		}
		cfg.GameID = positional[1]
		cfg.Matching = positional[2]
		cfg.UpdateFrom = normalizeVersion(positional[3])
		cfg.OutputDir = positional[4]
	case "update":
		if len(positional) < 6 {
			cfg.ShowHelp = true
			return cfg, nil
		}
		cfg.GameID = positional[1]
		cfg.Matching = positional[2]
		cfg.UpdateFrom = normalizeVersion(positional[3])
		cfg.UpdateTo = normalizeVersion(positional[4])
		cfg.OutputDir = positional[5]
	default:
		cfg.ShowHelp = true
	}
	if cfg.Threads < 1 {
		cfg.Threads = 1
	}
	if cfg.Handles < 1 {
		cfg.Handles = 1
	}
	return cfg, nil
}

func splitOption(arg string) (key, value string, hasValue bool) {
	key = strings.TrimLeft(arg, "-")
	if before, after, ok := strings.Cut(key, "="); ok {
		return before, after, true
	}
	return key, "", false
}

func printUsage(exe string) {
	fmt.Printf(`Usage:
    %[1]s full <gameId> <package> <version> <outputDir> [options]                 Download full game assets
    %[1]s update <gameId> <package> <updateFrom> <updateTo> <outputDir> [options] Download update assets

Arguments:
    <gameId>      Official region-specific game ID (for example gopR6Cufr3); short codes are not accepted
    <package>     What to download, either "game" or audio "zh-cn", "en-us", "ja-jp" or "ko-kr"
    <version>     Version to download
    <updateFrom>  Version to update from
    <updateTo>    Version to update to
    <outputDir>   Output directory to save the downloaded files

Options:
    --region=<value>      Region to use, OSREL or CNREL, defaults to OSREL
    --branch=<value>      Branch name, main or predownload, defaults to main
    --launcherId=<value>  Override launcher ID used when fetching packages
    --platApp=<value>     Override platform application ID used when fetching packages
    --threads=<value>     Number of concurrent asset downloads
    --handles=<value>     Maximum idle HTTP connections per host
    --silent              Suppress normal output, progress, and the download confirmation
    --dry-run             Fetch and parse manifests without downloading assets
    -h, --help            Show this help message
`, exe)
}

func run(ctx context.Context, cfg config) error {
	reg, err := parseRegion(cfg.Region)
	if err != nil {
		return err
	}
	br, err := parseBranch(cfg.Branch)
	if err != nil {
		return err
	}

	gameID := cfg.GameID
	client := makeHTTPClient(cfg.Handles)
	sophon := newSophonURL(reg, gameID, br, cfg.LauncherID, cfg.PlatApp)
	if err := sophon.getBuildData(ctx, client); err != nil {
		return err
	}

	if !cfg.Silent {
		fmt.Printf("Running with %d threads and %d handles\n", cfg.Threads, cfg.Handles)
	}

	prevURL, err := sophon.buildURL(cfg.UpdateFrom, cfg.Action == "update", false)
	if err != nil {
		return err
	}
	var newURL string
	if cfg.Action == "update" {
		newURL, err = sophon.buildURL(cfg.UpdateTo, true, true)
		if err != nil {
			return err
		}
	}

	assets, totalSize, diffSize, err := getAssetsFromManifests(ctx, client, cfg.Matching, prevURL, newURL)
	if err != nil {
		return err
	}

	if !cfg.Silent {
		fmt.Printf("* Found %d assets\n", len(assets))
		if newURL != "" {
			fmt.Printf("* Update data is %s\n", formatSize(float64(diffSize)))
			fmt.Printf("* Because the full assets will be downloaded, total download size is %s\n", formatSize(float64(totalSize)))
		} else {
			fmt.Printf("* Total download size is %s\n", formatSize(float64(totalSize)))
		}
	}

	if cfg.DryRun {
		if !cfg.Silent {
			fmt.Println("Dry run complete; no files were downloaded.")
		}
		return nil
	}

	if !cfg.Silent {
		fmt.Print("Continue? (y/n): ")
		var answer string
		_, _ = fmt.Fscan(os.Stdin, &answer)
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborting...")
			return nil
		}
	}

	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return err
	}

	return downloadAssets(ctx, client, assets, cfg.OutputDir, cfg.Threads, totalSize, diffSize, cfg.Silent)
}

func makeHTTPClient(handles int) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        handles,
			MaxIdleConnsPerHost: handles,
			MaxConnsPerHost:     handles,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: 0,
	}
}

func parseRegion(v string) (region, error) {
	switch strings.ToUpper(v) {
	case "OSREL", "":
		return regionOSREL, nil
	case "CNREL":
		return regionCNREL, nil
	default:
		return regionOSREL, fmt.Errorf("unknown region %q", v)
	}
}

func parseBranch(v string) (branchType, error) {
	switch strings.ToLower(strings.ReplaceAll(v, "_", "")) {
	case "main", "":
		return branchMain, nil
	case "predownload":
		return branchPreDownload, nil
	default:
		return branchMain, fmt.Errorf("unknown branch %q", v)
	}
}

func resolveGameID(regionName, id string) string {
	regionGames := gameMap[strings.ToUpper(regionName)]
	if regionGames == nil {
		return id
	}
	if mapped, ok := regionGames[strings.ToLower(id)]; ok {
		return mapped
	}
	return id
}

func normalizeVersion(v string) string {
	if strings.Count(v, ".") == 1 {
		return v + ".0"
	}
	return v
}

type sophonURL struct {
	apiBase      string
	sophonBase   string
	gameID       string
	branch       branchType
	launcherID   string
	platApp      string
	packageID    string
	password     string
	branchBackup branchesRoot
}

func newSophonURL(reg region, gameID string, branch branchType, launcherID, platApp string) sophonURL {
	s := sophonURL{gameID: gameID, branch: branch}
	switch reg {
	case regionCNREL:
		s.apiBase = "https://hyp-api.mihoyo.com/hyp/hyp-connect/api/getGameBranches"
		s.sophonBase = "https://api-takumi.mihoyo.com/downloader/sophon_chunk/api/getBuild"
		s.launcherID = "jGHBHlcOq1"
		s.platApp = "ddxf5qt290cg"
	default:
		s.apiBase = "https://sg-hyp-api.hoyoverse.com/hyp/hyp-connect/api/getGameBranches"
		s.sophonBase = "https://sg-public-api.hoyoverse.com/downloader/sophon_chunk/api/getBuild"
		s.launcherID = "VYTpXlbWo8"
		s.platApp = "ddxf6vlr1reo"
	}
	if launcherID != "" {
		s.launcherID = launcherID
	}
	if platApp != "" {
		s.platApp = platApp
	}
	return s
}

func (s *sophonURL) getBuildData(ctx context.Context, client *http.Client) error {
	u, err := url.Parse(s.apiBase)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("game_ids[]", s.gameID)
	q.Set("launcher_id", s.launcherID)
	u.RawQuery = q.Encode()

	var root branchesRoot
	if err := getJSON(ctx, client, u.String(), &root); err != nil {
		return err
	}
	branch, err := root.selectBranch(s.branch)
	if err != nil {
		return err
	}
	s.packageID = branch.PackageID
	s.password = branch.Password
	s.branchBackup = root
	return nil
}

func (s *sophonURL) buildURL(version string, isUpdateAction bool, isUpdateTarget bool) (string, error) {
	u, err := url.Parse(s.sophonBase)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if isUpdateAction && !isUpdateTarget {
		branch, err := s.branchBackup.selectBranch(branchMain)
		if err != nil {
			return "", err
		}
		q.Set("branch", "main")
		q.Set("package_id", branch.PackageID)
		q.Set("password", branch.Password)
	} else {
		q.Set("branch", branchQueryName(s.branch))
		q.Set("package_id", s.packageID)
		q.Set("password", s.password)
	}
	q.Set("plat_app", s.platApp)
	if isUpdateAction && !isUpdateTarget && s.branch == branchPreDownload {
		q.Set("tag", version)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func branchQueryName(b branchType) string {
	if b == branchPreDownload {
		return "predownload"
	}
	return "main"
}

type branchesRoot struct {
	Retcode int          `json:"retcode"`
	Message string       `json:"message"`
	Data    branchesData `json:"data"`
}

type branchesData struct {
	GameBranches []branchesGameBranch `json:"game_branches"`
}

type branchesGameBranch struct {
	Game        branchesGame `json:"game"`
	Main        branchesMain `json:"main"`
	PreDownload branchesMain `json:"pre_download"`
}

type branchesGame struct {
	ID  string `json:"id"`
	Biz string `json:"biz"`
}

type branchesMain struct {
	PackageID  string           `json:"package_id"`
	Branch     string           `json:"branch"`
	Password   string           `json:"password"`
	Tag        string           `json:"tag"`
	DiffTags   []string         `json:"diff_tags"`
	Categories []branchCategory `json:"categories"`
}

type branchCategory struct {
	CategoryID    string `json:"category_id"`
	MatchingField string `json:"matching_field"`
}

func (r branchesRoot) selectBranch(search branchType) (branchesMain, error) {
	if r.Retcode != 0 {
		return branchesMain{}, fmt.Errorf("getGameBranches failed: %s (%d)", r.Message, r.Retcode)
	}
	if len(r.Data.GameBranches) == 0 {
		return branchesMain{}, errors.New("getGameBranches returned no branches")
	}
	gb := r.Data.GameBranches[0]
	if search == branchPreDownload {
		if gb.PreDownload.PackageID == "" {
			return branchesMain{}, errors.New("pre-download branch not found")
		}
		return gb.PreDownload, nil
	}
	if gb.Main.PackageID == "" {
		return branchesMain{}, errors.New("main branch not found")
	}
	return gb.Main, nil
}

func getJSON(ctx context.Context, client *http.Client, requestURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("GET %s failed: %s: %s", requestURL, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type numberString int64

func (n *numberString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		*n = 0
		return nil
	}
	var s string
	if data[0] == '"' {
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
	} else {
		s = string(data)
	}
	if s == "" {
		*n = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*n = numberString(v)
	return nil
}

type boolString bool

func (b *boolString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		*b = false
		return nil
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch x := v.(type) {
	case bool:
		*b = boolString(x)
	case float64:
		*b = boolString(x != 0)
	case string:
		switch strings.ToLower(x) {
		case "1", "true", "yes":
			*b = true
		default:
			*b = false
		}
	default:
		*b = false
	}
	return nil
}

type buildBranch struct {
	Retcode int       `json:"retcode"`
	Message string    `json:"message"`
	Data    buildData `json:"data"`
}

type buildData struct {
	BuildID   string             `json:"build_id"`
	Tag       string             `json:"tag"`
	Manifests []manifestIdentity `json:"manifests"`
}

type manifestIdentity struct {
	CategoryID       numberString  `json:"category_id"`
	CategoryName     string        `json:"category_name"`
	MatchingField    string        `json:"matching_field"`
	Manifest         fileInfoJSON  `json:"manifest"`
	ManifestDownload urlInfoJSON   `json:"manifest_download"`
	Stats            chunkInfoJSON `json:"stats"`
	ChunkDownload    urlInfoJSON   `json:"chunk_download"`
	Deduplicated     chunkInfoJSON `json:"deduplicated_stats"`
}

type fileInfoJSON struct {
	ID               string       `json:"id"`
	Checksum         string       `json:"checksum"`
	CompressedSize   numberString `json:"compressed_size"`
	UncompressedSize numberString `json:"uncompressed_size"`
}

type urlInfoJSON struct {
	Password    string     `json:"password"`
	URLPrefix   string     `json:"url_prefix"`
	URLSuffix   string     `json:"url_suffix"`
	Encryption  boolString `json:"encryption"`
	Compression boolString `json:"compression"`
}

type chunkInfoJSON struct {
	CompressedSize   numberString `json:"compressed_size"`
	UncompressedSize numberString `json:"uncompressed_size"`
	FileCount        numberString `json:"file_count"`
	ChunkCount       numberString `json:"chunk_count"`
}

type manifestInfoPair struct {
	Chunks   chunksInfo
	Manifest manifestInfo
	Build    buildData
}

type chunksInfo struct {
	BaseURL             string
	ChunksCount         int
	FilesCount          int
	TotalSize           int64
	TotalCompressedSize int64
	UseCompression      bool
}

type manifestInfo struct {
	BaseURL        string
	ID             string
	ChecksumMD5    string
	UseCompression bool
	Size           int64
	CompressedSize int64
}

func (m manifestInfo) fileURL() string {
	return strings.TrimRight(m.BaseURL, "/") + "/" + m.ID
}

func createManifestInfoPair(ctx context.Context, client *http.Client, requestURL, matching string) (manifestInfoPair, error) {
	var branch buildBranch
	if err := getJSON(ctx, client, requestURL, &branch); err != nil {
		return manifestInfoPair{}, err
	}
	if branch.Retcode != 0 {
		return manifestInfoPair{}, fmt.Errorf("getBuild failed: %s (%d)", branch.Message, branch.Retcode)
	}
	if matching == "" {
		matching = "game"
	}
	var found *manifestIdentity
	for i := range branch.Data.Manifests {
		if branch.Data.Manifests[i].MatchingField == matching {
			found = &branch.Data.Manifests[i]
			break
		}
	}
	if found == nil {
		return manifestInfoPair{}, fmt.Errorf("Sophon manifest with matching field %q is not found", matching)
	}
	if found.Manifest.ID == "" || found.ManifestDownload.URLPrefix == "" {
		return manifestInfoPair{}, fmt.Errorf("manifest information for %q is incomplete", matching)
	}
	if found.ChunkDownload.URLPrefix == "" {
		return manifestInfoPair{}, fmt.Errorf("chunk information for %q is incomplete", matching)
	}
	return manifestInfoPair{
		Chunks: chunksInfo{
			BaseURL:             found.ChunkDownload.URLPrefix,
			ChunksCount:         int(found.Stats.ChunkCount),
			FilesCount:          int(found.Stats.FileCount),
			TotalSize:           int64(found.Stats.UncompressedSize),
			TotalCompressedSize: int64(found.Stats.CompressedSize),
			UseCompression:      bool(found.ChunkDownload.Compression),
		},
		Manifest: manifestInfo{
			BaseURL:        found.ManifestDownload.URLPrefix,
			ID:             found.Manifest.ID,
			ChecksumMD5:    found.Manifest.Checksum,
			UseCompression: bool(found.ManifestDownload.Compression),
			Size:           int64(found.Manifest.UncompressedSize),
			CompressedSize: int64(found.Manifest.CompressedSize),
		},
		Build: branch.Data,
	}, nil
}

type asset struct {
	Name       string
	Size       int64
	Hash       string
	IsDir      bool
	HasPatch   bool
	Chunks     []chunk
	ChunksInfo chunksInfo
	AltChunks  *chunksInfo
}

type chunk struct {
	Name             string
	HashDecompressed []byte
	OldOffset        int64
	Offset           int64
	Size             int64
	SizeDecompressed int64
}

type protoAsset struct {
	Name   string
	Chunks []protoChunk
	Type   int32
	Size   int64
	Hash   string
}

type protoChunk struct {
	Name             string
	HashDecompressed string
	Offset           int64
	Size             int64
	SizeDecompressed int64
}

func getAssetsFromManifests(ctx context.Context, client *http.Client, matching, prevURL, newURL string) ([]asset, int64, int64, error) {
	prevPair, err := createManifestInfoPair(ctx, client, prevURL, matching)
	if err != nil {
		return nil, 0, 0, err
	}
	prevProto, err := readManifestProto(ctx, client, prevPair.Manifest)
	if err != nil {
		return nil, 0, 0, err
	}
	if newURL == "" {
		assets := make([]asset, 0, len(prevProto))
		var total int64
		for _, p := range prevProto {
			a, err := assetFromProto(p, prevPair.Chunks)
			if err != nil {
				return nil, 0, 0, err
			}
			if !a.IsDir {
				total += a.Size
				assets = append(assets, a)
			}
		}
		return assets, total, total, nil
	}

	newPair, err := createManifestInfoPair(ctx, client, newURL, matching)
	if err != nil {
		return nil, 0, 0, err
	}
	newProto, err := readManifestProto(ctx, client, newPair.Manifest)
	if err != nil {
		return nil, 0, 0, err
	}
	assets, total, diff, err := updateAssets(prevProto, newProto, prevPair.Chunks, newPair.Chunks)
	if err != nil {
		return nil, 0, 0, err
	}
	return assets, total, diff, nil
}

func assetFromProto(p protoAsset, info chunksInfo) (asset, error) {
	if p.Type != 0 || p.Hash == "" {
		return asset{Name: p.Name, IsDir: true}, nil
	}
	chunks := make([]chunk, 0, len(p.Chunks))
	for _, c := range p.Chunks {
		hash, err := hex.DecodeString(c.HashDecompressed)
		if err != nil {
			return asset{}, fmt.Errorf("invalid chunk md5 for %s: %w", p.Name, err)
		}
		chunks = append(chunks, chunk{
			Name:             c.Name,
			HashDecompressed: hash,
			OldOffset:        -1,
			Offset:           c.Offset,
			Size:             c.Size,
			SizeDecompressed: c.SizeDecompressed,
		})
	}
	return asset{Name: p.Name, Size: p.Size, Hash: p.Hash, Chunks: chunks, ChunksInfo: info}, nil
}

func updateAssets(oldProto, newProto []protoAsset, oldChunks, newChunks chunksInfo) ([]asset, int64, int64, error) {
	oldByName := make(map[string]protoAsset, len(oldProto))
	for _, p := range oldProto {
		oldByName[p.Name] = p
	}
	assets := make([]asset, 0)
	var total int64
	var diff int64
	for _, next := range newProto {
		a, err := patchedAsset(oldByName, next, oldChunks, newChunks)
		if err != nil {
			return nil, 0, 0, err
		}
		if a.IsDir {
			continue
		}
		updated := false
		for _, c := range a.Chunks {
			if c.OldOffset == -1 {
				updated = true
				diff += c.SizeDecompressed
			}
		}
		if updated {
			total += a.Size
			assets = append(assets, a)
		}
	}
	return assets, total, diff, nil
}

func patchedAsset(oldByName map[string]protoAsset, next protoAsset, oldChunks, newChunks chunksInfo) (asset, error) {
	if next.Type != 0 || next.Hash == "" {
		return asset{Name: next.Name, IsDir: true}, nil
	}
	old, ok := oldByName[next.Name]
	if !ok {
		return assetFromProto(next, newChunks)
	}
	oldChunkByHash := make(map[string]protoChunk, len(old.Chunks))
	for _, c := range old.Chunks {
		oldChunkByHash[c.HashDecompressed] = c
	}
	chunks := make([]chunk, 0, len(next.Chunks))
	hasPatch := false
	for _, c := range next.Chunks {
		hash, err := hex.DecodeString(c.HashDecompressed)
		if err != nil {
			return asset{}, fmt.Errorf("invalid chunk md5 for %s: %w", next.Name, err)
		}
		ch := chunk{
			Name:             c.Name,
			HashDecompressed: hash,
			OldOffset:        -1,
			Offset:           c.Offset,
			Size:             c.Size,
			SizeDecompressed: c.SizeDecompressed,
		}
		if oldChunk, ok := oldChunkByHash[c.HashDecompressed]; ok {
			ch.OldOffset = oldChunk.Offset
			hasPatch = true
		}
		chunks = append(chunks, ch)
	}
	return asset{
		Name:       next.Name,
		Size:       next.Size,
		Hash:       next.Hash,
		Chunks:     chunks,
		ChunksInfo: newChunks,
		AltChunks:  &oldChunks,
		HasPatch:   hasPatch,
	}, nil
}

func readManifestProto(ctx context.Context, client *http.Client, info manifestInfo) ([]protoAsset, error) {
	data, err := getBytes(ctx, client, info.fileURL())
	if err != nil {
		return nil, err
	}
	if info.UseCompression {
		data, err = zstdDecompress(data, info.Size)
		if err != nil {
			return nil, fmt.Errorf("decompress manifest %s: %w", info.ID, err)
		}
	}
	if info.ChecksumMD5 != "" {
		sum := md5.Sum(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), info.ChecksumMD5) {
			fmt.Fprintf(os.Stderr, "Warning: manifest md5 mismatch for %s\n", info.ID)
		}
	}
	return parseManifestProto(data)
}

func getBytes(ctx context.Context, client *http.Client, requestURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GET %s failed: %s: %s", requestURL, resp.Status, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

func parseManifestProto(data []byte) ([]protoAsset, error) {
	var assets []protoAsset
	for len(data) > 0 {
		field, wt, rest, err := readTag(data)
		if err != nil {
			return nil, err
		}
		data = rest
		if field != 1 || wt != 2 {
			next, err := skipProtoValue(wt, data)
			if err != nil {
				return nil, err
			}
			data = next
			continue
		}
		msg, next, err := readBytes(data)
		if err != nil {
			return nil, err
		}
		p, err := parseProtoAsset(msg)
		if err != nil {
			return nil, err
		}
		assets = append(assets, p)
		data = next
	}
	return assets, nil
}

func parseProtoAsset(data []byte) (protoAsset, error) {
	var a protoAsset
	for len(data) > 0 {
		field, wt, rest, err := readTag(data)
		if err != nil {
			return a, err
		}
		data = rest
		switch field {
		case 1:
			if wt != 2 {
				return a, fmt.Errorf("asset name has wire type %d", wt)
			}
			var s string
			s, data, err = readString(data)
			a.Name = s
		case 2:
			if wt != 2 {
				return a, fmt.Errorf("asset chunk has wire type %d", wt)
			}
			var msg []byte
			msg, data, err = readBytes(data)
			if err != nil {
				return a, err
			}
			ch, err := parseProtoChunk(msg)
			if err != nil {
				return a, err
			}
			a.Chunks = append(a.Chunks, ch)
		case 3:
			var v uint64
			v, data, err = readVarintValue(wt, data)
			a.Type = int32(v)
		case 4:
			var v uint64
			v, data, err = readVarintValue(wt, data)
			a.Size = int64(v)
		case 5:
			if wt != 2 {
				return a, fmt.Errorf("asset hash has wire type %d", wt)
			}
			var s string
			s, data, err = readString(data)
			a.Hash = s
		default:
			data, err = skipProtoValue(wt, data)
		}
		if err != nil {
			return a, err
		}
	}
	return a, nil
}

func parseProtoChunk(data []byte) (protoChunk, error) {
	var c protoChunk
	for len(data) > 0 {
		field, wt, rest, err := readTag(data)
		if err != nil {
			return c, err
		}
		data = rest
		switch field {
		case 1:
			if wt != 2 {
				return c, fmt.Errorf("chunk name has wire type %d", wt)
			}
			var s string
			s, data, err = readString(data)
			c.Name = s
		case 2:
			if wt != 2 {
				return c, fmt.Errorf("chunk md5 has wire type %d", wt)
			}
			var s string
			s, data, err = readString(data)
			c.HashDecompressed = s
		case 3:
			var v uint64
			v, data, err = readVarintValue(wt, data)
			c.Offset = int64(v)
		case 4:
			var v uint64
			v, data, err = readVarintValue(wt, data)
			c.Size = int64(v)
		case 5:
			var v uint64
			v, data, err = readVarintValue(wt, data)
			c.SizeDecompressed = int64(v)
		default:
			data, err = skipProtoValue(wt, data)
		}
		if err != nil {
			return c, err
		}
	}
	return c, nil
}

func readTag(data []byte) (int, int, []byte, error) {
	v, rest, err := readVarint(data)
	if err != nil {
		return 0, 0, nil, err
	}
	return int(v >> 3), int(v & 0x7), rest, nil
}

func readVarint(data []byte) (uint64, []byte, error) {
	var v uint64
	for i := 0; i < len(data) && i < 10; i++ {
		b := data[i]
		v |= uint64(b&0x7f) << (7 * i)
		if b < 0x80 {
			return v, data[i+1:], nil
		}
	}
	return 0, nil, errors.New("invalid protobuf varint")
}

func readVarintValue(wt int, data []byte) (uint64, []byte, error) {
	if wt != 0 {
		return 0, nil, fmt.Errorf("expected protobuf varint, got wire type %d", wt)
	}
	return readVarint(data)
}

func readBytes(data []byte) ([]byte, []byte, error) {
	n, rest, err := readVarint(data)
	if err != nil {
		return nil, nil, err
	}
	if uint64(len(rest)) < n {
		return nil, nil, io.ErrUnexpectedEOF
	}
	return rest[:n], rest[n:], nil
}

func readString(data []byte) (string, []byte, error) {
	b, rest, err := readBytes(data)
	if err != nil {
		return "", nil, err
	}
	return string(b), rest, nil
}

func skipProtoValue(wt int, data []byte) ([]byte, error) {
	switch wt {
	case 0:
		_, rest, err := readVarint(data)
		return rest, err
	case 1:
		if len(data) < 8 {
			return nil, io.ErrUnexpectedEOF
		}
		return data[8:], nil
	case 2:
		_, rest, err := readBytes(data)
		return rest, err
	case 5:
		if len(data) < 4 {
			return nil, io.ErrUnexpectedEOF
		}
		return data[4:], nil
	default:
		return nil, fmt.Errorf("unsupported protobuf wire type %d", wt)
	}
}

func downloadAssets(ctx context.Context, client *http.Client, assets []asset, outputDir string, threads int, totalSize, diffSize int64, silent bool) error {
	jobs := make(chan asset)
	errs := make(chan error, 1)
	var downloaded atomic.Int64
	started := time.Now()
	var wg sync.WaitGroup

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for a := range jobs {
				if err := writeAsset(ctx, client, a, outputDir, func(n int64) {
					cur := downloaded.Add(n)
					if !silent {
						speed := float64(cur) / time.Since(started).Seconds()
						fmt.Printf("%s/%s (%s diff) (%s/s)\r", formatSize(float64(cur)), formatSize(float64(totalSize)), formatSize(float64(diffSize)), formatSize(speed))
					}
				}); err != nil {
					select {
					case errs <- err:
					default:
					}
					return
				}
			}
		}()
	}

	for _, a := range assets {
		select {
		case err := <-errs:
			close(jobs)
			wg.Wait()
			return err
		case jobs <- a:
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errs:
		return err
	default:
		if !silent {
			fmt.Println()
		}
		return nil
	}
}

func writeAsset(ctx context.Context, client *http.Client, a asset, outputDir string, progress func(int64)) error {
	outputPath := filepath.Join(outputDir, filepath.FromSlash(a.Name))
	tempPath := outputPath + "_tempUpdate"
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}

	oldPath := outputPath
	oldFile, _ := os.Open(oldPath)
	if oldFile != nil {
		defer oldFile.Close()
	}

	out, err := os.OpenFile(tempPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if err := out.Truncate(a.Size); err != nil {
		return err
	}

	for _, ch := range a.Chunks {
		if err := writeChunk(ctx, client, a, ch, oldFile, out, progress); err != nil {
			return err
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, outputPath)
}

func writeChunk(ctx context.Context, client *http.Client, a asset, ch chunk, oldFile *os.File, out *os.File, progress func(int64)) error {
	existing := make([]byte, ch.SizeDecompressed)
	if _, err := out.ReadAt(existing, ch.Offset); err == nil && verifyMD5(existing, ch.HashDecompressed) {
		progress(ch.SizeDecompressed)
		return nil
	}

	if ch.OldOffset != -1 && oldFile != nil {
		buf := make([]byte, ch.SizeDecompressed)
		if _, err := oldFile.ReadAt(buf, ch.OldOffset); err == nil && verifyMD5(buf, ch.HashDecompressed) {
			if _, err := out.WriteAt(buf, ch.Offset); err != nil {
				return err
			}
			progress(ch.SizeDecompressed)
			return nil
		}
	}

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		data, err := downloadChunk(ctx, client, a, ch)
		if err == nil && int64(len(data)) != ch.SizeDecompressed {
			err = fmt.Errorf("chunk %s decompressed to %d bytes, expected %d", ch.Name, len(data), ch.SizeDecompressed)
		}
		if err == nil && !verifyMD5(data, ch.HashDecompressed) {
			err = fmt.Errorf("chunk %s md5 mismatch", ch.Name)
		}
		if err == nil {
			if _, err := out.WriteAt(data, ch.Offset); err != nil {
				return err
			}
			progress(int64(len(data)))
			return nil
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	return fmt.Errorf("download %s for %s failed: %w", ch.Name, a.Name, lastErr)
}

func downloadChunk(ctx context.Context, client *http.Client, a asset, ch chunk) ([]byte, error) {
	data, compressed, err := tryDownloadChunk(ctx, client, a.ChunksInfo, ch.Name)
	if err != nil && a.AltChunks != nil {
		data, compressed, err = tryDownloadChunk(ctx, client, *a.AltChunks, ch.Name)
	}
	if err != nil {
		return nil, err
	}
	if compressed {
		return zstdDecompress(data, ch.SizeDecompressed)
	}
	return data, nil
}

func tryDownloadChunk(ctx context.Context, client *http.Client, info chunksInfo, name string) ([]byte, bool, error) {
	requestURL := strings.TrimRight(info.BaseURL, "/") + "/" + name
	data, err := getBytes(ctx, client, requestURL)
	return data, info.UseCompression, err
}

func verifyMD5(data []byte, expected []byte) bool {
	sum := md5.Sum(data)
	return bytes.Equal(sum[:], expected)
}

func formatSize(value float64) string {
	if value <= 0 {
		return "0 B"
	}
	suffixes := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	mag := 0
	for value >= 1000 && mag < len(suffixes)-1 {
		value /= 1024
		mag++
	}
	return fmt.Sprintf("%.2f %s", value, suffixes[mag])
}
