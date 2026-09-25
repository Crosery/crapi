package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/crosery/crapi/internal/api"
	"github.com/crosery/crapi/internal/ui"
)

const modelsHelp = `crapi models [关键字] [参数]

列出当前 API Key 可以调用的全部模型，按渠道分组。

参数：
  --channel <名称>  只看某个渠道（owned_by，如 anthropic / openai / antigravity / openrouter）
  --kind <类型>     chat / image / audio
  --free            只看免费模型
  --json            输出 JSON（便于脚本处理）`

func cmdModels(a *App, args []string) error {
	fs := newFlags("models", modelsHelp)
	channel := fs.String("channel", "", "")
	kind := fs.String("kind", "", "")
	free := fs.Bool("free", false, "")
	asJSON := fs.Bool("json", false, "")
	pos, err := parse(fs, args, modelsHelp)
	if err != nil {
		return err
	}
	c, err := a.client()
	if err != nil {
		return err
	}
	var models []api.Model
	if *asJSON {
		models, err = c.ListModels(a.ctx)
	} else {
		models, err = a.fetchModels(c)
	}
	if err != nil {
		return err
	}
	kw := strings.ToLower(strings.Join(pos, " "))
	var list []api.Model
	for _, m := range models {
		if *channel != "" && !strings.EqualFold(m.OwnedBy, *channel) {
			continue
		}
		if *kind != "" && string(m.Kind()) != strings.ToLower(*kind) {
			continue
		}
		if *free && !m.Free() {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(m.ID+" "+m.Name()), kw) {
			continue
		}
		list = append(list, m)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	}

	groups := groupByChannel(list)
	ui.Section("可用模型", fmt.Sprintf("%d 个模型 · %d 个渠道", len(list), len(groups)))
	g := ui.G()
	for _, grp := range groups {
		ui.Println()
		ui.Println("  " + ui.BrandS.Render(g.Dot) + " " + ui.Bold.Render(api.ChannelLabel(grp.channel)) +
			ui.Dim.Render(fmt.Sprintf("  %s · %d 个", grp.channel, len(grp.models))))
		var rows [][]string
		for _, m := range grp.models {
			rows = append(rows, []string{
				m.ID,
				ui.Truncate(m.Name(), 26),
				ui.Ctx(m.ContextLength),
				ui.Ctx(m.MaxCompletionTokens),
				caps(m),
				price(m),
			})
		}
		ui.Println(indent(ui.Table([]string{"模型 ID", "名称", "上下文", "输出", "能力", "价格 $/M 入/出"}, rows,
			ui.Left, ui.Left, ui.Right, ui.Right, ui.Left, ui.Right), "  "))
	}
	ui.Println()
	ui.Note("能力：视=识图 推=推理 工=工具调用 搜=联网 图=生图 音=音频。模型列表已按你的 Key 权限过滤。")
	return nil
}

type channelGroup struct {
	channel string
	models  []api.Model
}

func groupByChannel(list []api.Model) []channelGroup {
	idx := map[string]int{}
	var groups []channelGroup
	for _, m := range list {
		ch := m.OwnedBy
		if _, ok := idx[ch]; !ok {
			idx[ch] = len(groups)
			groups = append(groups, channelGroup{channel: ch})
		}
		groups[idx[ch]].models = append(groups[idx[ch]].models, m)
	}
	rank := map[string]int{"anthropic": 0, "openai": 1, "antigravity": 2}
	sort.SliceStable(groups, func(i, j int) bool {
		ri, iok := rank[groups[i].channel]
		rj, jok := rank[groups[j].channel]
		switch {
		case iok && jok:
			return ri < rj
		case iok != jok:
			return iok
		}
		return len(groups[i].models) > len(groups[j].models)
	})
	return groups
}

func caps(m api.Model) string {
	var parts []string
	switch m.Kind() {
	case api.KindImage:
		return ui.AccentS.Render("图")
	case api.KindAudio:
		return ui.AccentS.Render("音")
	}
	if m.Vision() {
		parts = append(parts, "视")
	}
	if m.SupportsReasoning {
		parts = append(parts, "推")
	}
	if m.SupportsToolCall == nil || *m.SupportsToolCall {
		parts = append(parts, "工")
	}
	if m.SupportsWebSearch {
		parts = append(parts, "搜")
	}
	return strings.Join(parts, " ")
}

func price(m api.Model) string {
	if m.Free() {
		return ui.OKS.Render("免费")
	}
	if m.Cost == nil || (m.Cost.Input == 0 && m.Cost.Output == 0) {
		return ui.Dim.Render("-")
	}
	return fmt.Sprintf("%s / %s", trimNum(m.Cost.Input), trimNum(m.Cost.Output))
}

func trimNum(v float64) string {
	s := fmt.Sprintf("%.3f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

const channelsHelp = `crapi channels [--json]

按渠道汇总当前 Key 可用的模型数量与代表模型。`

func cmdChannels(a *App, args []string) error {
	fs := newFlags("channels", channelsHelp)
	asJSON := fs.Bool("json", false, "")
	if _, err := parse(fs, args, channelsHelp); err != nil {
		return err
	}
	c, err := a.client()
	if err != nil {
		return err
	}
	models, err := a.fetchModels(c)
	if err != nil {
		return err
	}
	groups := groupByChannel(models)
	if *asJSON {
		out := map[string][]string{}
		for _, g := range groups {
			out[g.channel] = ids(g.models)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	ui.Section("可用渠道", fmt.Sprintf("%d 个渠道 · %d 个模型", len(groups), len(models)))
	max := 0
	for _, g := range groups {
		if len(g.models) > max {
			max = len(g.models)
		}
	}
	var rows [][]string
	for _, g := range groups {
		kinds := map[api.Kind]int{}
		for _, m := range g.models {
			kinds[m.Kind()]++
		}
		var kindParts []string
		for _, k := range []api.Kind{api.KindChat, api.KindImage, api.KindAudio} {
			if kinds[k] > 0 {
				kindParts = append(kindParts, fmt.Sprintf("%s %d", map[api.Kind]string{api.KindChat: "对话", api.KindImage: "生图", api.KindAudio: "音频"}[k], kinds[k]))
			}
		}
		sample := ids(g.models)
		if len(sample) > 3 {
			sample = append(sample[:3], "…")
		}
		rows = append(rows, []string{
			api.ChannelLabel(g.channel),
			g.channel,
			ui.Bar(float64(len(g.models))/float64(max), 12, ui.Accent) + fmt.Sprintf(" %d", len(g.models)),
			strings.Join(kindParts, " · "),
			ui.Truncate(strings.Join(sample, ", "), 46),
		})
	}
	ui.Println(indent(ui.Table([]string{"渠道", "标识", "模型数", "类型", "代表模型"}, rows), "  "))
	ui.Println()
	ui.Note("查看某个渠道的全部模型：crapi models --channel <标识>；号池账号状态：crapi pool")
	return nil
}
