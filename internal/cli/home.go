package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/crosery/crapi/internal/store"
	"github.com/crosery/crapi/internal/term"
	"github.com/crosery/crapi/internal/ui"
)

// cmdHome：不带参数运行 crapi。没有 Key 时直接进入 setup；有 Key 时显示交互菜单。
func cmdHome(a *App) error {
	if a.Cfg.Key() == "" {
		if !term.Interactive() {
			return cmdHelp(a, nil)
		}
		return cmdSetup(a, nil)
	}
	if !term.Interactive() {
		return cmdHelp(a, nil)
	}
	ui.Banner(a.Version, a.Cfg.Base())
	ui.KV("API Key", store.MaskKey(a.Cfg.Key()))
	if len(a.Cfg.Managed) > 0 {
		var names []string
		for id := range a.Cfg.Managed {
			names = append(names, id)
		}
		ui.KV("已接入", strings.Join(sortStrings(names), ", "))
	}
	if !a.Cfg.LastUpdate.IsZero() {
		ui.KV("上次同步", a.Cfg.LastUpdate.Local().Format("2006-01-02 15:04"))
	}
	for {
		ui.Println()
		choice, err := ui.AskSelect("要做什么？", "", []ui.Option{
			{Label: "查看用量与额度", Value: "usage"},
			{Label: "查看号池状态", Value: "pool"},
			{Label: "同步最新模型到所有 harness", Value: "update"},
			{Label: "配置 / 新增 agent harness", Value: "setup"},
			{Label: "查看可用模型", Value: "models"},
			{Label: "查看渠道", Value: "channels"},
			{Label: "生图", Value: "image"},
			{Label: "harness 接入状态", Value: "status"},
			{Label: "更换 API Key", Value: "key"},
			{Label: "诊断", Value: "doctor"},
			{Label: "退出", Value: "quit"},
		})
		if err != nil || choice == "quit" {
			return nil
		}
		var runErr error
		switch choice {
		case "usage":
			runErr = cmdUsage(a, nil)
		case "pool":
			runErr = cmdPool(a, nil)
		case "update":
			runErr = cmdUpdate(a, nil)
		case "setup":
			runErr = cmdSetup(a, nil)
		case "models":
			runErr = cmdModels(a, nil)
		case "channels":
			runErr = cmdChannels(a, nil)
		case "image":
			runErr = cmdImage(a, nil)
		case "status":
			runErr = cmdStatus(a, nil)
		case "key":
			runErr = cmdKey(a, []string{"set"})
		case "doctor":
			runErr = cmdDoctor(a, nil)
		}
		if runErr != nil && !errors.Is(runErr, ui.ErrAborted) {
			ui.Fail("%v", runErr)
		}
	}
}

func sortStrings(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s
}

// ---------- upgrade ----------

// DownloadBase 是发布产物的下载根地址；需要 VERSION、SHA256SUMS 与 crapi-<os>-<arch>[.exe]。
// 可用环境变量 CRAPI_DOWNLOAD_BASE 覆盖（镜像 / 内网）。
var DownloadBase = "https://github.com/crosery/crapi/releases/latest/download"

func downloadBase() string {
	if v := strings.TrimSpace(os.Getenv("CRAPI_DOWNLOAD_BASE")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return DownloadBase
}

func assetName() string {
	name := fmt.Sprintf("crapi-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func httpGet(url string, limit int64) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s 返回 HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func cmdUpgrade(a *App, args []string) error {
	fs := newFlags("upgrade", "crapi upgrade [--force]")
	force := fs.Bool("force", false, "")
	if _, err := parse(fs, args, "crapi upgrade [--force]   升级到最新版本"); err != nil {
		return err
	}
	base := downloadBase()
	var raw []byte
	if err := ui.Spin("正在检查新版本…", func(context.Context) error {
		var err error
		raw, err = httpGet(base+"/VERSION", 1<<10)
		return err
	}); err != nil {
		return fmt.Errorf("检查新版本失败：%w", err)
	}
	latest := strings.TrimSpace(string(raw))
	if latest == a.Version && !*force {
		ui.Success("已经是最新版本 %s", latest)
		return nil
	}
	ui.KV("当前版本", a.Version)
	ui.KV("最新版本", latest)
	var bin []byte
	if err := ui.Spin("正在下载 "+assetName()+"…", func(context.Context) error {
		var err error
		bin, err = httpGet(base+"/"+assetName(), 200<<20)
		return err
	}); err != nil {
		return err
	}
	if sums, err := httpGet(base+"/SHA256SUMS", 64<<10); err == nil {
		sum := sha256.Sum256(bin)
		want := ""
		for _, line := range strings.Split(string(sums), "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && strings.TrimPrefix(f[1], "*") == assetName() {
				want = f[0]
			}
		}
		if want != "" && !strings.EqualFold(want, hex.EncodeToString(sum[:])) {
			return errors.New("校验和不一致，已中止升级")
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return fmt.Errorf("写入新版本失败（可能需要管理员权限）：%w", err)
	}
	if runtime.GOOS == "windows" {
		// 运行中的 exe 不能被覆盖，但可以改名；旧文件下次启动时清理。
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, exe); err != nil {
		return err
	}
	ui.Success("已升级到 %s", latest)
	return nil
}

// cleanupOld 删除 Windows 升级留下的旧 exe。
func cleanupOld() {
	if runtime.GOOS != "windows" {
		return
	}
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
}
