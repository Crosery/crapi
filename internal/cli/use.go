package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/crosery/crapi/internal/api"
	"github.com/crosery/crapi/internal/harness"
	"github.com/crosery/crapi/internal/term"
	"github.com/crosery/crapi/internal/ui"
)

const useHelp = `crapi use [model-id] [参数]

快速切换已接入 agent harness 的默认模型。不带 model-id 时进入交互式选择菜单。

别名：crapi switch, crapi set-model

示例：
  crapi use                       交互式选择模型并切换
  crapi use claude-opus-5-5       一键把全部已接入工具切换为 claude-opus-5-5
  crapi use gpt-5.6-sol --harness codex   仅切换 codex 的默认模型

参数：
  --harness <id>  只切换指定的 harness（如 claude、codex、opencode 等），默认切换全部已接入工具
  --dry-run       仅预览变更，不实际修改配置文件`

func cmdUse(a *App, args []string) error {
	fs := newFlags("use", useHelp)
	targetHarness := fs.String("harness", "", "")
	dry := fs.Bool("dry-run", false, "")
	positional, err := parse(fs, args, useHelp)
	if err != nil {
		return err
	}

	c, err := a.client()
	if err != nil {
		return err
	}
	ui.Banner(a.Version, a.Cfg.Base())

	models, err := a.fetchModels(c)
	if err != nil {
		return err
	}
	chat := api.ChatModels(models)
	if len(chat) == 0 {
		return errors.New("当前 API Key 未检测到任何可用的对话模型")
	}

	env := harness.DefaultEnv()

	// 确定目标 harness 列表
	var targets []harness.Harness
	if *targetHarness != "" {
		h, ok := harness.Get(*targetHarness)
		if !ok {
			return fmt.Errorf("不认识的 harness 标识：%s（可用 crapi status 查看支持列表）", *targetHarness)
		}
		targets = []harness.Harness{h}
	} else if len(positional) == 0 && term.Interactive() {
		// 交互模式下：询问是切换所有工具，还是切换指定工具
		var configured []harness.Harness
		for _, h := range harness.All() {
			_, managed := a.Cfg.Managed[h.ID()]
			det := h.Detect(env)
			if managed || (det.Installed && h.Status(env, a.Cfg.Base()).Configured) {
				configured = append(configured, h)
			}
		}
		if len(configured) == 0 {
			ui.Warn("未检测到已接入 crosery 的 agent harness，请先运行 crapi setup 完成初次配置。")
			return nil
		}

		scopeOpts := []ui.Option{
			{Label: fmt.Sprintf("全部已接入的 agent harness（共 %d 个）", len(configured)), Value: "all"},
		}
		for _, h := range configured {
			st := h.Status(env, a.Cfg.Base())
			modelHint := ""
			if st.Model != "" {
				modelHint = " [当前: " + st.Model + "]"
			}
			scopeOpts = append(scopeOpts, ui.Option{
				Label: ui.Pad(h.Name(), 18) + ui.Dim.Render(modelHint),
				Value: h.ID(),
			})
		}

		ui.Println()
		scopeChoice, err := ui.AskSelect("请选择切换范围：", "all", scopeOpts)
		if err != nil {
			return err
		}

		if scopeChoice == "all" {
			targets = configured
		} else {
			h, _ := harness.Get(scopeChoice)
			targets = []harness.Harness{h}
		}
	} else {
		for _, h := range harness.All() {
			_, managed := a.Cfg.Managed[h.ID()]
			det := h.Detect(env)
			if managed || (det.Installed && h.Status(env, a.Cfg.Base()).Configured) {
				targets = append(targets, h)
			}
		}
	}

	if len(targets) == 0 {
		ui.Warn("未检测到已接入 crosery 的 agent harness，请先运行 crapi setup 完成初次配置。")
		return nil
	}

	targetModel := ""
	if len(positional) > 0 {
		targetModel = strings.TrimSpace(positional[0])
		if _, ok := api.Find(chat, targetModel); !ok {
			ui.Warn("模型 %s 不在当前 Key 的可用列表中。", ui.Bold.Render(targetModel))
			var similar []string
			for _, m := range chat {
				if strings.Contains(strings.ToLower(m.ID), strings.ToLower(targetModel)) {
					similar = append(similar, m.ID)
				}
			}
			if len(similar) > 0 {
				ui.Note("相似可用模型：%s", strings.Join(similar, ", "))
			}
			return fmt.Errorf("无效的模型标识：%s", targetModel)
		}
	} else {
		if !term.Interactive() {
			return errors.New("请指定要切换的模型名称，例如：crapi use claude-opus-5-5")
		}
		targetModel, err = promptSelectModel(chat)
		if err != nil {
			return err
		}
	}

	ui.Section("切换默认模型", fmt.Sprintf("目标模型：%s · 影响 %d 个工具", ui.AccentS.Render(targetModel), len(targets)))
	plan := harness.Plan{
		BaseURL:    a.Cfg.Base(),
		Key:        c.Key,
		Models:     chat,
		Model:      targetModel,
		WebSearch:  false,
		UpdateOnly: false,
	}

	ok, _ := applyAll(a, env, plan, targets, *dry)
	if !*dry {
		if err := a.Cfg.Save(); err != nil {
			return err
		}
	}

	ui.Println()
	ui.Success("默认模型已切换为 %s（已更新 %d / %d 个工具配置）。",
		ui.AccentS.Render(targetModel), ok, len(targets))
	return nil
}

// promptSelectModel 呈现分层/常用模型的交互式选择框。
func promptSelectModel(chat []api.Model) (string, error) {
	// 常用主流模型按推荐顺序置顶
	recommended := []string{
		"claude-opus-5-5",
		"claude-sonnet-5",
		"claude-haiku-4-5-20251001",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"gemini-3.1-pro",
		"gemini-3-flash",
		"deepseek-v3.2",
		"glm-5",
	}

	recSet := map[string]bool{}
	for _, id := range recommended {
		recSet[id] = true
	}

	var opts []ui.Option
	// 1. 优先放入推荐且当前 Key 拥有的模型
	for _, id := range recommended {
		if m, ok := api.Find(chat, id); ok {
			ctxStr := ""
			if m.ContextLength > 0 {
				ctxStr = fmt.Sprintf(" (%s ctx)", ui.Ctx(m.ContextLength))
			}
			label := ui.Pad(m.ID, 28) + " " + ui.Dim.Render(m.OwnedBy+ctxStr)
			opts = append(opts, ui.Option{Label: label, Value: m.ID})
		}
	}

	// 2. 其余可用模型按字母序追加
	var others []api.Model
	for _, m := range chat {
		if !recSet[m.ID] {
			others = append(others, m)
		}
	}
	sort.Slice(others, func(i, j int) bool {
		return others[i].ID < others[j].ID
	})

	for _, m := range others {
		ctxStr := ""
		if m.ContextLength > 0 {
			ctxStr = fmt.Sprintf(" (%s ctx)", ui.Ctx(m.ContextLength))
		}
		label := ui.Pad(m.ID, 28) + " " + ui.Dim.Render(m.OwnedBy+ctxStr)
		opts = append(opts, ui.Option{Label: label, Value: m.ID})
	}

	ui.Println()
	choice, err := ui.AskSelect("请选择要设为默认的主流模型：", "", opts)
	if err != nil {
		return "", err
	}
	return choice, nil
}
