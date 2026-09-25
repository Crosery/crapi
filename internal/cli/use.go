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

const useHelp = `crapi use [harness] [model-id] [参数]

快速查看与切换已接入 agent harness 的默认模型。
不带参数时进入交互式界面：动态浏览各工具当前使用的默认模型，支持选择工具、切换模型并可随时返回上一级。

别名：crapi switch, crapi set-model

示例：
  crapi use                               进入交互式选择菜单（先选工具看当前模型，支持随时返回）
  crapi use claude                        为 Claude Code 交互式选择并切换模型
  crapi use claude claude-opus-5-5        直接把 Claude Code 切换为 claude-opus-5-5
  crapi use claude-opus-5-5               一键把全部已接入的工具统一切换为 claude-opus-5-5
  crapi use codex gpt-5.6-sol             直接把 OpenAI Codex 切换为 gpt-5.6-sol

参数：
  --harness <id>  指定要切换的目标 harness（也可作为第一个位置参数直接传入）
  --dry-run       仅预览变更，不实际修改配置文件`

func cmdUse(a *App, args []string) error {
	fs := newFlags("use", useHelp)
	harnessFlag := fs.String("harness", "", "")
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

	// 动态实时获取本机所有已接入或已配置的 harness 及其最新状态
	getConfigured := func() []harness.Harness {
		var list []harness.Harness
		for _, h := range harness.All() {
			_, managed := a.Cfg.Managed[h.ID()]
			det := h.Detect(env)
			if managed || (det.Installed && h.Status(env, a.Cfg.Base()).Configured) {
				list = append(list, h)
			}
		}
		return list
	}

	configured := getConfigured()
	if len(configured) == 0 {
		ui.Warn("未检测到已接入 crosery 的 agent harness，请先运行 crapi setup 或 crapi reload。")
		return nil
	}

	// 解析命令行直接传入的参数：支持 crapi use <harness> <model> 或 crapi use <model>
	var targetHarness harness.Harness
	targetModel := ""

	if *harnessFlag != "" {
		h, ok := harness.Get(*harnessFlag)
		if !ok {
			return fmt.Errorf("不认识的 harness 标识：%s（可用 crapi status 查看）", *harnessFlag)
		}
		targetHarness = h
	}

	if len(positional) >= 2 {
		// crapi use <harness> <model>
		h, ok := harness.Get(positional[0])
		if !ok {
			return fmt.Errorf("不认识的 harness 标识：%s", positional[0])
		}
		targetHarness = h
		targetModel = positional[1]
	} else if len(positional) == 1 {
		// 可能是 harness 也可能是 model
		if h, ok := harness.Get(positional[0]); ok {
			targetHarness = h
		} else {
			targetModel = positional[0]
		}
	}

	// 如果指定了具体 model，先校验有效性
	if targetModel != "" {
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
	}

	// 非交互模式必须有确定的 model
	if !term.Interactive() && targetModel == "" {
		return errors.New("缺少目标模型名称，例如：crapi use claude-opus-5-5")
	}

	// 交互式主循环：动态刷新，随时支持返回上一级
	for {
		var selectedHarnesses []harness.Harness
		currentModelOfTarget := ""

		if targetHarness != nil {
			selectedHarnesses = []harness.Harness{targetHarness}
			currentModelOfTarget = targetHarness.Status(env, a.Cfg.Base()).Model
		} else if targetModel == "" {
			// 第一级：展示每个 harness 当前使用的模型（动态读取最新配置）
			configured = getConfigured()
			var scopeOpts []ui.Option
			scopeOpts = append(scopeOpts, ui.Option{
				Label: ui.Pad("【全部已接入工具】", 20) + ui.Dim.Render(fmt.Sprintf("统一切换全部 %d 个工具的默认模型", len(configured))),
				Value: "ALL",
			})

			for _, h := range configured {
				st := h.Status(env, a.Cfg.Base())
				modelDisplay := "未设置"
				if st.Model != "" {
					modelDisplay = st.Model
				}
				label := ui.Pad(h.Name(), 18) + "  " + ui.Dim.Render("当前: ") + ui.AccentS.Render(modelDisplay)
				scopeOpts = append(scopeOpts, ui.Option{
					Label: label,
					Value: h.ID(),
				})
			}

			scopeOpts = append(scopeOpts, ui.Option{
				Label: ui.Dim.Render("← 返回上一级 / 退出"),
				Value: "__EXIT__",
			})

			ui.Println()
			pickHarness, err := ui.AskSelect("请选择要查看与切换模型的 Agent Harness：", "ALL", scopeOpts)
			if err != nil || pickHarness == "__EXIT__" {
				return nil
			}

			if pickHarness == "ALL" {
				selectedHarnesses = configured
			} else {
				h, _ := harness.Get(pickHarness)
				selectedHarnesses = []harness.Harness{h}
				currentModelOfTarget = h.Status(env, a.Cfg.Base()).Model
			}
		} else {
			selectedHarnesses = configured
		}

		// 第二级：选择目标模型（支持「返回上一级」）
		chosenModel := targetModel
		if chosenModel == "" {
			var err error
			hName := "全部工具"
			if len(selectedHarnesses) == 1 {
				hName = selectedHarnesses[0].Name()
			}
			chosenModel, err = promptSelectModelForHarness(hName, currentModelOfTarget, chat)
			if err != nil {
				return err
			}

			// 如果用户选择了返回上一级，重置当前目标 harness，回到第一级菜单
			if chosenModel == "__BACK__" {
				targetHarness = nil
				targetModel = ""
				continue
			}
		}

		// 第三级：执行配置写入
		ui.Println()
		ui.Section("切换默认模型", fmt.Sprintf("目标：%s · 影响 %d 个工具", ui.AccentS.Render(chosenModel), len(selectedHarnesses)))
		plan := harness.Plan{
			BaseURL:    a.Cfg.Base(),
			Key:        c.Key,
			Models:     chat,
			Model:      chosenModel,
			WebSearch:  false,
			UpdateOnly: false,
		}

		ok, _ := applyAll(a, env, plan, selectedHarnesses, *dry)
		if !*dry {
			if len(selectedHarnesses) > 1 {
				a.Cfg.DefaultModel = chosenModel
			}
			if err := a.Cfg.Save(); err != nil {
				return err
			}
		}

		ui.Println()
		ui.Success("默认模型已切换为 %s（已更新 %d / %d 个工具配置）。",
			ui.AccentS.Render(chosenModel), ok, len(selectedHarnesses))

		// 如果是通过命令行指定参数运行的，执行完毕后直接退出
		if len(positional) > 0 || *harnessFlag != "" || !term.Interactive() {
			return nil
		}

		// 交互模式下：清空当前选定工具，自动返回工具列表，方便继续查看与切换
		targetHarness = nil
		targetModel = ""
		continueChoice, err := ui.AskSelect("后续操作：", "continue", []ui.Option{
			{Label: "继续查看 / 切换其他 Agent Harness", Value: "continue"},
			{Label: "完成并退出", Value: "exit"},
		})
		if err != nil || continueChoice == "exit" {
			break
		}
	}

	return nil
}

// promptSelectModelForHarness 呈现带有「返回上一级」和当前模型高亮标记的选择菜单。
func promptSelectModelForHarness(harnessName, currentModel string, chat []api.Model) (string, error) {
	var opts []ui.Option

	// 顶部固定提供「返回上一级」选项
	opts = append(opts, ui.Option{
		Label: ui.Dim.Render("← 返回上一级（重新选择工具）"),
		Value: "__BACK__",
	})

	// 常用主流模型置顶推荐
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

	formatOpt := func(m api.Model) ui.Option {
		ctxStr := ""
		if m.ContextLength > 0 {
			ctxStr = fmt.Sprintf(" (%s ctx)", ui.Ctx(m.ContextLength))
		}
		statusTag := ""
		if currentModel != "" && (m.ID == currentModel || strings.HasSuffix(currentModel, "/"+m.ID)) {
			statusTag = " " + ui.OKS.Render("[当前使用]")
		}
		label := ui.Pad(m.ID, 28) + " " + ui.Dim.Render(m.OwnedBy+ctxStr) + statusTag
		return ui.Option{Label: label, Value: m.ID}
	}

	// 1. 如果当前正在使用的模型不在推荐列表中，且属于可用模型，优先放到最前面展示
	if currentModel != "" && !recSet[currentModel] {
		rawID := currentModel
		if idx := strings.LastIndex(rawID, "/"); idx >= 0 {
			rawID = rawID[idx+1:]
		}
		if m, ok := api.Find(chat, rawID); ok {
			opts = append(opts, formatOpt(m))
			recSet[rawID] = true
		}
	}

	// 2. 推荐主流模型列表
	for _, id := range recommended {
		if m, ok := api.Find(chat, id); ok {
			opts = append(opts, formatOpt(m))
		}
	}

	// 3. 其余可用模型按名称字母序追加
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
		opts = append(opts, formatOpt(m))
	}

	title := fmt.Sprintf("【%s】请选择要切换的默认主流模型：", harnessName)
	if currentModel != "" {
		title = fmt.Sprintf("【%s】请选择要切换的默认主流模型（当前: %s）：", harnessName, currentModel)
	}

	ui.Println()
	choice, err := ui.AskSelect(title, "", opts)
	if err != nil {
		return "", err
	}
	return choice, nil
}
