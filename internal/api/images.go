package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ImageRequest 描述一次生图。RefImages 为本地参考图路径（图生图 / 编辑）。
type ImageRequest struct {
	Model     string
	Prompt    string
	Size      string
	Quality   string
	N         int
	RefImages []string
}

// Image 是一张生成结果。
type Image struct {
	Data []byte
	Ext  string // png / jpg / webp
}

const imageTimeout = 10 * time.Minute

// usesImagesAPI：gpt-image / dall-e / grok-imagine 走 /v1/images/*，
// Gemini 等多模态模型走 chat/completions 的图片输出。
func usesImagesAPI(model string) bool {
	m := strings.ToLower(model)
	return strings.HasPrefix(m, "gpt-image") || strings.HasPrefix(m, "dall-e") || strings.Contains(m, "imagine")
}

// GenerateImages 按模型自动选择接口生成图片。
func (c *Client) GenerateImages(ctx context.Context, req ImageRequest) ([]Image, error) {
	if req.N <= 0 {
		req.N = 1
	}
	if usesImagesAPI(req.Model) {
		imgs, err := c.imagesAPI(ctx, req)
		var e *Error
		if err == nil || !(errors.As(err, &e) && e.Status == 400 && strings.Contains(e.Message, "not supported on /v1/images")) {
			return imgs, err
		}
	}
	return c.chatImages(ctx, req)
}

type imagesResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	OutputFormat string `json:"output_format"`
}

func (c *Client) imagesAPI(ctx context.Context, req ImageRequest) ([]Image, error) {
	var resp imagesResponse
	if len(req.RefImages) == 0 {
		payload := map[string]any{"model": req.Model, "prompt": req.Prompt, "n": req.N}
		if req.Size != "" {
			payload["size"] = req.Size
		}
		if req.Quality != "" {
			payload["quality"] = req.Quality
		}
		if err := c.postJSON(ctx, c.BaseURL+"/v1/images/generations", payload, imageTimeout, &resp); err != nil {
			return nil, err
		}
	} else {
		body, ctype, err := editForm(req)
		if err != nil {
			return nil, err
		}
		if err := c.do(ctx, http.MethodPost, c.BaseURL+"/v1/images/edits", body, ctype, imageTimeout, &resp); err != nil {
			return nil, err
		}
	}
	ext := resp.OutputFormat
	if ext == "" || ext == "jpeg" {
		ext = map[string]string{"": "png", "jpeg": "jpg"}[ext]
	}
	var out []Image
	for _, d := range resp.Data {
		if d.B64JSON == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(d.B64JSON)
		if err != nil {
			return nil, fmt.Errorf("图片 base64 解码失败：%w", err)
		}
		out = append(out, Image{Data: raw, Ext: sniffExt(raw, ext)})
	}
	if len(out) == 0 {
		return nil, errors.New("服务端没有返回图片")
	}
	return out, nil
}

func editForm(req ImageRequest) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("model", req.Model)
	_ = w.WriteField("prompt", req.Prompt)
	_ = w.WriteField("n", strconv.Itoa(req.N))
	if req.Size != "" {
		_ = w.WriteField("size", req.Size)
	}
	if req.Quality != "" {
		_ = w.WriteField("quality", req.Quality)
	}
	field := "image"
	if len(req.RefImages) > 1 {
		field = "image[]"
	}
	for _, p := range req.RefImages {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, "", fmt.Errorf("读取参考图 %s 失败：%w", p, err)
		}
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, filepath.Base(p)))
		h.Set("Content-Type", mimeOf(p, data))
		part, err := w.CreatePart(h)
		if err != nil {
			return nil, "", err
		}
		_, _ = part.Write(data)
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}

type chatImageResponse struct {
	Choices []struct {
		Message struct {
			Content any `json:"content"`
			Images  []struct {
				ImageURL struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"images"`
		} `json:"message"`
	} `json:"choices"`
}

// chatImages 通过 chat/completions 的图片输出生图；每次请求取一张，按 N 循环。
func (c *Client) chatImages(ctx context.Context, req ImageRequest) ([]Image, error) {
	content := []map[string]any{{"type": "text", "text": req.Prompt}}
	for _, p := range req.RefImages {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("读取参考图 %s 失败：%w", p, err)
		}
		url := "data:" + mimeOf(p, data) + ";base64," + base64.StdEncoding.EncodeToString(data)
		content = append(content, map[string]any{"type": "image_url", "image_url": map[string]string{"url": url}})
	}
	payload := map[string]any{
		"model":      req.Model,
		"messages":   []map[string]any{{"role": "user", "content": content}},
		"modalities": []string{"image", "text"},
	}
	if req.Size != "" {
		payload["size"] = req.Size
	}
	var out []Image
	for i := 0; i < req.N; i++ {
		var resp chatImageResponse
		if err := c.postJSON(ctx, c.BaseURL+"/v1/chat/completions", payload, imageTimeout, &resp); err != nil {
			return out, err
		}
		for _, ch := range resp.Choices {
			for _, im := range ch.Message.Images {
				if img, ok := decodeDataURL(im.ImageURL.URL); ok {
					out = append(out, img)
				}
			}
			if parts, ok := ch.Message.Content.([]any); ok {
				for _, part := range parts {
					pm, _ := part.(map[string]any)
					iu, _ := pm["image_url"].(map[string]any)
					if u, _ := iu["url"].(string); u != "" {
						if img, ok := decodeDataURL(u); ok {
							out = append(out, img)
						}
					}
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("模型没有返回图片，可以换个描述或换个模型再试")
	}
	return out, nil
}

func decodeDataURL(u string) (Image, bool) {
	if !strings.HasPrefix(u, "data:") {
		return Image{}, false
	}
	comma := strings.IndexByte(u, ',')
	if comma < 0 {
		return Image{}, false
	}
	meta := u[5:comma]
	raw, err := base64.StdEncoding.DecodeString(u[comma+1:])
	if err != nil {
		return Image{}, false
	}
	ext := "png"
	if exts, _ := mime.ExtensionsByType(strings.TrimSuffix(meta, ";base64")); len(exts) > 0 {
		ext = strings.TrimPrefix(exts[0], ".")
	}
	return Image{Data: raw, Ext: sniffExt(raw, ext)}, true
}

func sniffExt(data []byte, fallback string) string {
	switch http.DetectContentType(data) {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	}
	if fallback == "" || fallback == "jpeg" || fallback == "jpe" || fallback == "jfif" {
		if fallback == "" {
			return "png"
		}
		return "jpg"
	}
	return fallback
}

func mimeOf(path string, data []byte) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); strings.HasPrefix(t, "image/") {
		return t
	}
	return http.DetectContentType(data)
}
