package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/crosery/crapi/internal/api"
	"github.com/crosery/crapi/internal/ui"
)

const usageHelp = `crapi usage [参数]

查看当前 API Key 的额度（日 / 周 / 总）、花费与按模型的用量排行。

参数：
  --days <N>      统计最近 N 天（默认 7）
  --requests      同时列出最近的请求明细
  --limit <N>     请求明细条数（默认 15）
  --json          输出 JSON`

func cmdUsage(a *App, args []string) error {
	fs := newFlags("usage", usageHelp)
	days := fs.Int("days", 7, "")
	withReq := fs.Bool("requests", false, "")
	limit := fs.Int("limit", 15, "")
	asJSON := fs.Bool("json", false, "")
	if _, err := parse(fs, args, usageHelp); err != nil {
		return err
	}
	c, err := a.client()
	if err != nil {
		return err
	}
	var u *api.Usage
	var reqs []api.RequestItem
	load := func(ctx context.Context) error {
		var err error
		u, err = c.GetUsage(a.ctx, *days)
		if err != nil {
			return err
		}
		if *withReq {
			reqs, err = c.GetRequests(a.ctx, *days, *limit)
		}
		return err
	}
	if *asJSON {
		if err := load(a.ctx); err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"usage": u, "requests": reqs})
	}
	if err := ui.Spin("正在读取用量…", load); err != nil {
		return err
	}
	renderUsage(u)
	if *withReq {
		renderRequests(reqs)
	}
	return nil
}

func renderUsage(u *api.Usage) {
	g := ui.G()
	name := u.Key.Name
	if name == "" {
		name = "当前 Key"
	}
	since := ""
	if u.TrackingSince != nil {
		since = "统计自 " + shortTime(*u.TrackingSince)
	}
	ui.Section("账号用量 · "+name, fmt.Sprintf("最近 %d 天  %s", u.Days, since))

	if u.BlockedReason != nil && *u.BlockedReason != "" {
		ui.Println("  " + ui.ErrS.Render(g.Fail+" 已被暂停："+*u.BlockedReason))
	}

	// 额度窗口
	q := u.Quota
	if q != nil {
		ui.Println()
		if q.Unlimited {
			ui.Println("  " + ui.OKS.Render(g.Dot+" 不限额度") + ui.Dim.Render("  以下为已消费金额（按官方单价估算）"))
		}
		for _, w := range []struct {
			label string
			win   *api.QuotaWindow
		}{{"今日", q.Daily}, {"本周", q.Weekly}, {"累计", q.Total}} {
			if w.win == nil {
				continue
			}
			ui.Println("  " + quotaLine(w.label, w.win, u.QuotaTimeZone))
		}
	}

	// 汇总卡片
	t := u.Totals
	cacheRatio := 0.0
	if in := t.NewInputTokens + t.CacheTokens; in > 0 {
		cacheRatio = t.CacheTokens / in
	}
	stats := []string{
		stat("请求", ui.Count(t.Requests)),
		stat("总 Token", ui.Tokens(t.TotalTokens)),
		stat("输入", ui.Tokens(t.NewInputTokens)),
		stat("输出", ui.Tokens(t.OutputTokens)),
		stat("缓存命中", ui.Tokens(t.CacheTokens)+ui.Dim.Render(fmt.Sprintf(" %.0f%%", cacheRatio*100))),
		stat("估算花费", ui.BrandS.Render(ui.USD(t.TotalCostUsd))),
	}
	ui.Println()
	ui.Println(indent(ui.Card(strings.Join(stats, ui.Dim.Render("   "+g.Sep+"   "))), "  "))

	// 模型排行
	models := append([]api.ModelUsage(nil), u.Models...)
	sort.Slice(models, func(i, j int) bool {
		ci, cj := cost(models[i]), cost(models[j])
		if ci != cj {
			return ci > cj
		}
		return models[i].TotalTokens > models[j].TotalTokens
	})
	if len(models) == 0 {
		ui.Println()
		ui.Note("这段时间还没有请求记录。")
		return
	}
	maxCost := 0.0
	for _, m := range models {
		if c := cost(m); c > maxCost {
			maxCost = c
		}
	}
	var rows [][]string
	for i, m := range models {
		if i >= 15 {
			break
		}
		share := ""
		if maxCost > 0 {
			share = ui.Bar(cost(m)/maxCost, 14, ui.Brand)
		}
		costStr := ui.Dim.Render("未定价")
		if m.TotalCostUsd != nil {
			costStr = ui.USD(*m.TotalCostUsd)
		}
		rows = append(rows, []string{
			ui.Truncate(m.Model, 30),
			ui.Count(m.Requests),
			ui.Tokens(m.NewInputTokens),
			ui.Tokens(m.OutputTokens),
			ui.Tokens(m.CacheTokens),
			costStr,
			share,
		})
	}
	ui.Println()
	ui.Println("  " + ui.Bold.Render("按模型") + ui.Dim.Render(fmt.Sprintf("  共 %d 个模型，按花费排序", len(models))))
	ui.Println(indent(ui.Table([]string{"模型", "请求", "输入", "输出", "缓存", "花费", "占比"}, rows,
		ui.Left, ui.Right, ui.Right, ui.Right, ui.Right, ui.Right, ui.Left), "  "))
	if len(models) > 15 {
		ui.Note("另有 %d 个模型未显示，用 --json 查看全部", len(models)-15)
	}
	if len(u.UnpricedModels) > 0 {
		ui.Note("未定价（不计入花费与额度）：%s", strings.Join(u.UnpricedModels, ", "))
	}
}

func stat(label, value string) string {
	return ui.Dim.Render(label) + " " + ui.Bold.Render(value)
}

func cost(m api.ModelUsage) float64 {
	if m.TotalCostUsd == nil {
		return 0
	}
	return *m.TotalCostUsd
}

func quotaLine(label string, w *api.QuotaWindow, tz string) string {
	reset := ""
	if w.ResetsAt != nil && *w.ResetsAt != "" {
		reset = ui.Dim.Render(" · " + untilReset(*w.ResetsAt) + "后重置")
	} else if label == "累计" {
		reset = ui.Dim.Render(" · 不自动重置")
	}
	if w.LimitUsd <= 0 {
		return ui.Pad(label, 5) + ui.Bar(0, 24) + "  " + ui.Bold.Render(ui.USD(w.SpentUsd)) + ui.Dim.Render(" / 不限") + reset
	}
	ratio := w.SpentUsd / w.LimitUsd
	pct := fmt.Sprintf(" %3.0f%%", ratio*100)
	status := ""
	if w.Exceeded {
		status = " " + ui.ErrS.Render("已超额")
	}
	return ui.Pad(label, 5) + ui.Bar(ratio, 24) + pct + "  " +
		ui.Bold.Render(ui.USD(w.SpentUsd)) + ui.Dim.Render(" / "+ui.USD(w.LimitUsd)) + status + reset
}

func untilReset(ts string) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return ts
	}
	d := time.Until(t)
	if d <= 0 {
		return "即将"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	switch {
	case h >= 48:
		return fmt.Sprintf("%d 天 %d 小时", h/24, h%24)
	case h > 0:
		return fmt.Sprintf("%d 小时 %d 分", h, m)
	default:
		return fmt.Sprintf("%d 分钟", m)
	}
}

func shortTime(ts string) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("2006-01-02 15:04")
}

func renderRequests(items []api.RequestItem) {
	ui.Println()
	ui.Println("  " + ui.Bold.Render("最近请求") + ui.Dim.Render(fmt.Sprintf("  %d 条", len(items))))
	if len(items) == 0 {
		ui.Note("没有请求记录")
		return
	}
	g := ui.G()
	var rows [][]string
	for _, it := range items {
		st := ui.OKS.Render(g.OK + fmt.Sprintf(" %d", it.StatusCode))
		if it.Success == 0 {
			st = ui.ErrS.Render(g.Fail + fmt.Sprintf(" %d", it.StatusCode))
		}
		rows = append(rows, []string{
			shortTime(it.Timestamp)[5:],
			ui.Truncate(it.Model, 26),
			strings.TrimPrefix(it.Endpoint, "POST "),
			st,
			fmt.Sprintf("%.1fs", it.LatencyMs/1000),
			ui.Tokens(it.InputTokens) + "/" + ui.Tokens(it.OutputTokens),
			ui.Tokens(it.CachedTokens),
		})
	}
	ui.Println(indent(ui.Table([]string{"时间", "模型", "接口", "状态", "耗时", "入/出", "缓存"}, rows,
		ui.Left, ui.Left, ui.Left, ui.Left, ui.Right, ui.Right, ui.Right), "  "))
}
