package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/crosery/crapi/internal/api"
	"github.com/crosery/crapi/internal/term"
	"github.com/crosery/crapi/internal/ui"
)

const imageHelp = `crapi image "<描述>" [参数]

用小鸡云网关生图，支持网关上全部生图模型（gpt-image 系列、Gemini Image 等），
可附带参考图做图生图 / 编辑。结果保存为本地图片文件。

参数：
  -m, --model <id>     生图模型（缺省自动选择；--list 查看全部）
  -n <N>               生成张数（默认 1）
  -s, --size <WxH>     尺寸，如 1024x1024 / 1536x1024 / 1024x1536 / auto
  -q, --quality <档>   low / medium / high / auto（gpt-image 系列）
  -i, --image <文件>   参考图，可重复传多张
  -o, --out <目录>     保存目录（默认当前目录）
  --open               生成后用系统看图工具打开
  --list               列出当前 Key 可用的生图模型`

func cmdImage(a *App, args []string) error {
	fs := newFlags("image", imageHelp)
	model := fs.String("model", "", "")
	fs.StringVar(model, "m", "", "")
	n := fs.Int("n", 1, "")
	size := fs.String("size", "", "")
	fs.StringVar(size, "s", "", "")
	quality := fs.String("quality", "", "")
	fs.StringVar(quality, "q", "", "")
	var refs multiFlag
	fs.Var(&refs, "image", "")
	fs.Var(&refs, "i", "")
	out := fs.String("out", ".", "")
	fs.StringVar(out, "o", ".", "")
	open := fs.Bool("open", false, "")
	list := fs.Bool("list", false, "")
	pos, err := parse(fs, args, imageHelp)
	if err != nil {
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
	imgModels := api.ImageModels(models)
	if len(imgModels) == 0 {
		return errors.New("当前 Key 没有可用的生图模型")
	}
	if *list {
		ui.Section("生图模型", fmt.Sprintf("%d 个", len(imgModels)))
		var rows [][]string
		for _, m := range imgModels {
			via := "chat/completions"
			if strings.HasPrefix(m.ID, "gpt-image") || strings.Contains(m.ID, "imagine") {
				via = "images/generations"
			}
			rows = append(rows, []string{m.ID, ui.Truncate(m.Name(), 28), api.ChannelLabel(m.OwnedBy), via, price(m)})
		}
		ui.Println(indent(ui.Table([]string{"模型 ID", "名称", "渠道", "接口", "价格 $/M"}, rows), "  "))
		return nil
	}

	prompt := strings.TrimSpace(strings.Join(pos, " "))
	if prompt == "" {
		if !term.Interactive() {
			return errors.New("请提供图片描述：crapi image \"一只橘猫\"")
		}
		prompt, err = ui.AskText("描述你想要的画面", "中英文都可以，越具体越好", "例如：赛博朋克风格的上海外滩夜景，雨夜，霓虹倒影", func(s string) error {
			if strings.TrimSpace(s) == "" {
				return errors.New("描述不能为空")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if *model == "" {
		*model = api.Pick(imgModels, api.PrefImage, nil)
		if term.Interactive() && len(pos) == 0 {
			var opts []ui.Option
			for _, m := range imgModels {
				opts = append(opts, ui.Option{Label: ui.Pad(m.ID, 26) + ui.Dim.Render(api.ChannelLabel(m.OwnedBy)), Value: m.ID, Selected: m.ID == *model})
			}
			if *model, err = ui.AskSelect("选择生图模型", "", opts); err != nil {
				return err
			}
		}
	} else if _, ok := api.Find(models, *model); !ok {
		return fmt.Errorf("当前 Key 看不到模型 %s，用 crapi image --list 查看可用生图模型", *model)
	}
	for _, r := range refs {
		if _, err := os.Stat(r); err != nil {
			return fmt.Errorf("参考图不存在：%s", r)
		}
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}

	ui.Section("生图", *model)
	ui.KV("描述", ui.Truncate(prompt, 70))
	if len(refs) > 0 {
		ui.KV("参考图", strings.Join(refs, ", "))
	}
	req := api.ImageRequest{Model: *model, Prompt: prompt, Size: *size, Quality: *quality, N: *n, RefImages: refs}
	var imgs []api.Image
	start := time.Now()
	err = ui.Spin(fmt.Sprintf("正在生成 %d 张图片（通常 10~60 秒）…", *n), func(ctx context.Context) error {
		var err error
		imgs, err = c.GenerateImages(a.ctx, req)
		return err
	})
	if err != nil && len(imgs) == 0 {
		return err
	}
	stamp := time.Now().Format("20060102-150405")
	var saved []string
	for i, im := range imgs {
		name := fmt.Sprintf("crapi-%s-%d.%s", stamp, i+1, im.Ext)
		path := filepath.Join(*out, name)
		if err := os.WriteFile(path, im.Data, 0o644); err != nil {
			return err
		}
		abs, _ := filepath.Abs(path)
		saved = append(saved, abs)
		ui.Success("%s %s", abs, ui.Dim.Render(fmt.Sprintf("(%d KB)", len(im.Data)/1024)))
	}
	ui.Note("耗时 %.1f 秒", time.Since(start).Seconds())
	if err != nil {
		ui.Warn("部分图片生成失败：%v", err)
	}
	if *open {
		for _, p := range saved {
			openFile(p)
		}
	}
	return nil
}

func openFile(p string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", p)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", p)
	default:
		cmd = exec.Command("xdg-open", p)
	}
	_ = cmd.Start()
}
