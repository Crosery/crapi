package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/crosery/crapi/internal/api"
	"github.com/crosery/crapi/internal/ui"
)

const poolHelp = `crapi pool [--json]

查看 Crosery 号池状态：每个渠道的账号数、可用 / 冷却 / 停用数量，
以及 Claude、Codex 等订阅账号在各额度窗口（5 小时 / 每周）的平均占用与重置时间。
数据为脱敏聚合，不含任何账号标识。`

func cmdPool(a *App, args []string) error {
	fs := newFlags("pool", poolHelp)
	asJSON := fs.Bool("json", false, "")
	if _, err := parse(fs, args, poolHelp); err != nil {
		return err
	}
	c, err := a.client()
	if err != nil {
		return err
	}
	var pool *api.Pool
	var models []api.Model
	load := func(ctx context.Context) error {
		var err error
		pool, err = c.GetPool(a.ctx)
		if errors.Is(err, api.ErrUnsupported) {
			pool = nil
			err = nil
		}
		if err != nil {
			return err
		}
		models, err = c.ListModels(a.ctx)
		return err
	}
	if *asJSON {
		if err := load(a.ctx); err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pool)
	}
	if err := ui.Spin("正在读取号池状态…", load); err != nil {
		return err
	}
	if pool == nil {
		ui.Section("号池状态")
		ui.Warn("控制台暂未开放号池接口，先展示各渠道的在线模型。")
		return cmdChannelsFrom(models)
	}
	renderPool(pool)
	return nil
}

func renderPool(p *api.Pool) {
	g := ui.G()
	total := api.PoolAccounts{}
	for _, pr := range p.Providers {
		total.Total += pr.Accounts.Total
		total.Active += pr.Accounts.Active
		total.Cooling += pr.Accounts.Cooling
		total.Disabled += pr.Accounts.Disabled
		total.Error += pr.Accounts.Error
	}
	hint := fmt.Sprintf("%d 个渠道 · %d 个账号", len(p.Providers), total.Total)
	if p.GeneratedAt != "" {
		hint += " · " + shortTime(p.GeneratedAt)
	}
	ui.Section("号池状态", hint)
	ui.Println()
	ui.Println("  " + accountsBar(total, 40) + "  " + accountsLegend(total))

	providers := append([]api.PoolProvider(nil), p.Providers...)
	sort.SliceStable(providers, func(i, j int) bool { return providers[i].Accounts.Total > providers[j].Accounts.Total })
	for _, pr := range providers {
		ui.Println()
		label := pr.Label
		if label == "" {
			label = api.ChannelLabel(pr.Provider)
		}
		health := ui.OKS.Render(g.Dot + " 正常")
		switch {
		case pr.Accounts.Total == 0 || pr.Accounts.Active == 0:
			health = ui.ErrS.Render(g.Dot + " 无可用账号")
		case float64(pr.Accounts.Active)/float64(pr.Accounts.Total) < 0.5:
			health = ui.WarnS.Render(g.Dot + " 紧张")
		}
		ui.Println("  " + ui.Bold.Render(label) + ui.Dim.Render("  "+pr.Provider) + "  " + health)
		ui.Println("    " + accountsBar(pr.Accounts, 30) + "  " + accountsLegend(pr.Accounts))
		for _, w := range pr.Windows {
			reset := ""
			if w.ResetsAt != nil && *w.ResetsAt != "" {
				reset = ui.Dim.Render(" · " + untilReset(*w.ResetsAt) + "后重置")
			}
			n := ""
			if w.Accounts > 0 {
				n = ui.Dim.Render(fmt.Sprintf(" · %d 个账号平均", w.Accounts))
			}
			ui.Println("    " + ui.Pad(w.Label, 8) + ui.Bar(w.UsedPercent/100, 24) +
				fmt.Sprintf(" %3.0f%%", w.UsedPercent) + n + reset)
		}
		if len(pr.Models) > 0 {
			ui.Println("    " + ui.Dim.Render("模型 ") + ui.Truncate(strings.Join(pr.Models, ", "), 90))
		}
	}
	if p.QuotaAsOf != nil && *p.QuotaAsOf != "" {
		ui.Println()
		ui.Note("额度窗口数据更新于 %s", shortTime(*p.QuotaAsOf))
	}
}

// accountsBar 用分段色条表示「可用 / 冷却 / 异常 / 停用」的比例。
func accountsBar(a api.PoolAccounts, width int) string {
	if a.Total <= 0 {
		return ui.Bar(0, width)
	}
	g := ui.G()
	seg := func(n int) int { return int(float64(n)/float64(a.Total)*float64(width) + 0.5) }
	active, cooling, errN := seg(a.Active), seg(a.Cooling), seg(a.Error)
	disabled := width - active - cooling - errN
	if disabled < 0 {
		active += disabled
		disabled = 0
	}
	return ui.OKS.Render(strings.Repeat(g.BarFull, active)) +
		ui.WarnS.Render(strings.Repeat(g.BarFull, cooling)) +
		ui.ErrS.Render(strings.Repeat(g.BarFull, errN)) +
		ui.Dim.Render(strings.Repeat(g.BarEmpty, disabled))
}

func accountsLegend(a api.PoolAccounts) string {
	parts := []string{ui.OKS.Render(fmt.Sprintf("可用 %d", a.Active))}
	if a.Cooling > 0 {
		parts = append(parts, ui.WarnS.Render(fmt.Sprintf("冷却 %d", a.Cooling)))
	}
	if a.Error > 0 {
		parts = append(parts, ui.ErrS.Render(fmt.Sprintf("异常 %d", a.Error)))
	}
	if a.Disabled > 0 {
		parts = append(parts, ui.Dim.Render(fmt.Sprintf("停用 %d", a.Disabled)))
	}
	return strings.Join(parts, ui.Dim.Render(" · ")) + ui.Dim.Render(fmt.Sprintf(" / 共 %d", a.Total))
}

// cmdChannelsFrom 在没有号池接口时退化为按渠道展示模型。
func cmdChannelsFrom(models []api.Model) error {
	groups := groupByChannel(models)
	var rows [][]string
	for _, g := range groups {
		sample := ids(g.models)
		if len(sample) > 4 {
			sample = append(sample[:4], "…")
		}
		rows = append(rows, []string{api.ChannelLabel(g.channel), fmt.Sprint(len(g.models)), ui.Truncate(strings.Join(sample, ", "), 56)})
	}
	ui.Println(indent(ui.Table([]string{"渠道", "在线模型", "模型"}, rows, ui.Left, ui.Right, ui.Left), "  "))
	return nil
}
