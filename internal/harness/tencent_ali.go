package harness

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/crosery/crapi/internal/api"
)

// buddyModel 是 CodeBuddy / WorkBuddy 共用 models.json 加载器的条目格式（仅支持 OpenAI Chat Completions）。
func buddyModel(p Plan, m api.Model) *Obj {
	return O(
		"id", m.ID,
		"name", m.Name(),
		"vendor", ProviderName,
		"url", p.OpenAIBase()+"/chat/completions",
		"apiKey", p.Key,
		"maxInputTokens", ctxOr(m, 128000),
		"maxOutputTokens", maxOutOr(m, 16384),
		"supportsToolCall", true,
		"supportsImages", m.Vision(),
		"supportsReasoning", m.SupportsReasoning,
	)
}

// mergeBuddyModels 用本渠道的新条目替换旧条目（按 url 识别），保留用户自己的其他模型。
func mergeBuddyModels(existing []any, p Plan) []any {
	var out []any
	for _, e := range existing {
		if o, ok := e.(*Obj); ok && baseMatches(o.Str("url"), p.BaseURL) {
			continue
		}
		out = append(out, e)
	}
	for _, m := range agentModels(p) {
		out = append(out, buddyModel(p, m))
	}
	return out
}

// codebuddy（腾讯 CodeBuddy Code CLI）：~/.codebuddy/models.json + settings.json 的 model。
type codebuddy struct{}

func (codebuddy) ID() string         { return "codebuddy" }
func (codebuddy) Name() string       { return "CodeBuddy Code" }
func (codebuddy) Category() Category { return CLI }

func (codebuddy) dir(e Env) string {
	if d := os.Getenv("CODEBUDDY_CONFIG_DIR"); d != "" && e.Home == defaultHome() {
		return d
	}
	return e.P(".codebuddy")
}

func (x codebuddy) Detect(e Env) Detection {
	return detectAny(which(e, "codebuddy", "cbc"), x.dir(e))
}

func (x codebuddy) Status(e Env, base string) Status {
	mf, _, _ := readJSONFile(filepath.Join(x.dir(e), "models.json"))
	sf, _, _ := readJSONFile(filepath.Join(x.dir(e), "settings.json"))
	st := Status{}
	if mf != nil {
		arr, _ := mf.Get("models")
		list, _ := arr.([]any)
		for _, e := range list {
			if o, ok := e.(*Obj); ok && baseMatches(o.Str("url"), base) {
				st.Configured = true
				break
			}
		}
	}
	if sf != nil {
		st.Model = sf.Str("model")
	}
	return st
}

func (x codebuddy) Apply(e Env, p Plan, w *Writer) (Result, error) {
	mPath := filepath.Join(x.dir(e), "models.json")
	sPath := filepath.Join(x.dir(e), "settings.json")
	mf, _, err := readJSONFile(mPath)
	if err != nil {
		return Result{}, err
	}
	existing, _ := anyOr(mf, "models").([]any)
	merged := mergeBuddyModels(existing, p)
	mf.Set("models", merged)
	// availableModels 与产品内置列表合并，不会隐藏内置模型。
	var avail []any
	seen := map[string]bool{}
	for _, v := range Strs(anyOr(mf, "availableModels")) {
		if !seen[v] {
			avail = append(avail, v)
			seen[v] = true
		}
	}
	for _, m := range agentModels(p) {
		if !seen[m.ID] {
			avail = append(avail, m.ID)
			seen[m.ID] = true
		}
	}
	mf.Set("availableModels", avail)

	sf, _, err := readJSONFile(sPath)
	if err != nil {
		return Result{}, err
	}
	prev := sf.Str("model")
	model := p.Choose(prev, api.PrefAgent)
	if !p.UpdateOnly || prev != model {
		sf.Set("model", model)
	}
	if err := w.Write(mPath, MarshalJSON(mf), 0o600); err != nil {
		return Result{}, err
	}
	if err := w.Write(sPath, MarshalJSON(sf), 0o644); err != nil {
		return Result{}, err
	}
	return Result{Files: w.Written(), Model: model,
		Notes: []string{"修改默认模型需重启 CodeBuddy 或执行 /clear"}}, nil
}

// workbuddy（腾讯 WorkBuddy 桌面端）：~/.workbuddy/models.json（WorkBuddy AI 为 ~/.workbuddy-ai）。
// 应用自身总是写「裸数组」形态，这里保持一致；默认模型只能在应用内选择。
type workbuddy struct{ variant, dir string }

func (x workbuddy) ID() string {
	return strings.ToLower(strings.ReplaceAll(x.variant, " ", "-"))
}
func (x workbuddy) Name() string       { return x.variant }
func (x workbuddy) Category() Category { return Desktop }

func (x workbuddy) path(e Env) string { return e.P(x.dir, "models.json") }

func (x workbuddy) Detect(e Env) Detection {
	app := appInstalled(e, []string{x.variant}, []string{x.variant, strings.ReplaceAll(x.variant, " ", "")}, nil)
	return detectAny(app, e.P(x.dir))
}

func (x workbuddy) Status(e Env, base string) Status {
	data, err := os.ReadFile(x.path(e))
	if err != nil {
		return Status{}
	}
	list := buddyList(data)
	for _, v := range list {
		if o, ok := v.(*Obj); ok && baseMatches(o.Str("url"), base) {
			return Status{Configured: true, Detail: "在应用内「自定义模型」分组里选择"}
		}
	}
	return Status{}
}

func buddyList(data []byte) []any {
	if arr, err := ParseJSONArray(data); err == nil {
		return arr
	}
	if o, _, err := ParseJSON(data); err == nil {
		arr, _ := anyOr(o, "models").([]any)
		return arr
	}
	return nil
}

func (x workbuddy) Apply(e Env, p Plan, w *Writer) (Result, error) {
	data, _ := os.ReadFile(x.path(e))
	merged := mergeBuddyModels(buddyList(data), p)
	if err := w.Write(x.path(e), MarshalJSON(merged), 0o600); err != nil {
		return Result{}, err
	}
	model := p.Choose("", api.PrefAgent)
	return Result{Files: w.Written(), Model: model, Notes: []string{
		"应用会自动热加载；在模型选择器的「自定义模型」分组里选中 " + model,
	}}, nil
}

// qoder（Qoder CLI / Qoder CN）：settings.json 的 providers.crosery。
// 该功能由 Qoder 服务端按账号开关控制（custom_providers），未开通时会被静默忽略。
type qoder struct{ variant, dir string }

func (x qoder) ID() string {
	if x.dir == ".qoder" {
		return "qoder"
	}
	return "qoder-cn"
}
func (x qoder) Name() string       { return x.variant }
func (x qoder) Category() Category { return CLI }

func (x qoder) settings(e Env) string {
	if x.dir == ".qoder" {
		if d := os.Getenv("QODER_CONFIG_DIR"); d != "" && e.Home == defaultHome() {
			return filepath.Join(d, "settings.json")
		}
	}
	return e.P(x.dir, "settings.json")
}

func (x qoder) Detect(e Env) Detection {
	if x.dir == ".qoder" {
		return detectAny(which(e, "qodercli"), e.P(".qoder", "bin"))
	}
	return detectAny(appInstalled(e, []string{"Qoder CN"}, []string{"Qoder CN", "QoderCN"}, nil), e.P(x.dir))
}

func (x qoder) Status(e Env, base string) Status {
	s, _, _ := readJSONFile(x.settings(e))
	if s == nil {
		return Status{}
	}
	prov := s.Child("providers").Child(ProviderID)
	return Status{Configured: baseMatches(prov.Str("baseUrl"), base), Model: s.Child("model").Str("name")}
}

func (x qoder) Apply(e Env, p Plan, w *Writer) (Result, error) {
	path := x.settings(e)
	s, had, err := readJSONFile(path)
	if err != nil {
		return Result{}, err
	}
	prev := ""
	if cur := s.Child("model").Str("name"); strings.HasPrefix(cur, ProviderID+"/") {
		prev = strings.TrimPrefix(cur, ProviderID+"/")
	}
	model := p.Choose(prev, api.PrefAgent)
	var models []any
	for _, m := range agentModels(p) {
		models = append(models, O(
			"model", m.ID,
			"displayName", m.Name(),
			"contextWindow", ctxOr(m, 128000),
			"maxOutputTokens", maxOutOr(m, 16384),
			"capabilities", O("tools", true, "vision", m.Vision(), "thinking", m.SupportsReasoning),
		))
	}
	s.Child("providers").Set(ProviderID, O(
		"type", "openai-compatible",
		"protocol", "openai",
		"displayName", ProviderName,
		"baseUrl", p.OpenAIBase(),
		"apiKey", p.Key,
		"model", model,
		"models", models,
	))
	if !p.UpdateOnly || prev != model {
		s.Child("model").Set("name", ProviderID+"/"+model)
	}
	if err := w.Write(path, MarshalJSON(s), 0o600); err != nil {
		return Result{}, err
	}
	notes := []string{"需要 Qoder 账号已开通「自定义模型」（个人版 v1.1.50+）；未开通时该配置会被忽略", "重启后生效，也可用 -m " + ProviderID + "/" + model + " 指定"}
	if had {
		notes = append(notes, "settings.json 里的注释会在重写后丢失（原文件已备份）")
	}
	return Result{Files: w.Written(), Model: model, Notes: notes}, nil
}
