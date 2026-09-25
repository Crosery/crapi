# crapi 🐣

> **小鸡云 CPA 网关一站式 Agent Harness 配置与管理工具**  
> 一行命令将 [小鸡云 CPA](https://ai.crosery.com/) 的上游模型与 API Key 自动注入本机所有主流 AI 编程代理环境，提供模型热更新、用量与号池监控、全量生图等功能。

[![Go Version](https://img.shields.io/badge/Go-1.26-blue.svg)](https://golang.org)
[![Platform](https://img.shields.io/badge/Platform-macOS%20|%20Linux%20|%20Windows-green.svg)](#一键安装)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

---

## 目录

- [核心特性](#核心特性)
- [支持的环境与工具矩阵](#支持的环境与工具矩阵)
- [一键安装](#一键安装)
- [常用命令与工作流](#常用命令与工作流)
  - [主菜单与一键配置 (setup)](#主菜单与一键配置-setup)
  - [上游模型热更新 (update)](#上游模型热更新-update)
  - [权限与可用模型查询 (models / channels)](#权限与可用模型查询-models--channels)
  - [额度用量与近期请求监控 (usage)](#额度用量与近期请求监控-usage)
  - [上游号池健康状态 (pool)](#上游号池健康状态-pool)
  - [全量生图与图像编辑 (image)](#全量生图与图像编辑-image)
  - [配置回滚与安全审计 (restore / doctor)](#配置回滚与安全审计-restore--doctor)
- [关键技术与工程实现](#关键技术与工程实现)
- [源码构建](#源码构建)
- [开源许可](#开源许可)

---

## 核心特性

- 🚀 **一行命令跨平台配置**：原生支持 Linux、macOS 与 Windows（PowerShell 5.1/7+、cmd.exe），单静态二进制、零 CGO 依赖，开箱即用。
- 🤖 **全量 Harness 深度集成**：覆盖 Claude Code、Codex CLI、Pi、oh-my-pi、Cursor、WorkBuddy、Qoder、Hermes、OpenCode、Kilo、Kimi、Zed、Cherry Studio 等 20+ 款主流 Agent。
- 🔄 **模型热更新与保全设计**：自动拉取网关模型目录增量，清晰输出 `+ 新增` 与 `- 下线` 变化；同时保留用户自定义的手动默认模型。
- 🌐 **Claude Code 联网搜索一键配齐**：自动匹配并注入 Haiku 模型档位作为辅助检索模型（`ANTHROPIC_DEFAULT_HAIKU_MODEL`），开启 `WebSearch` 权限并消除双 Key 冲突。
- 🎨 **全量双通路生图引擎**：自动识别并桥接 OpenAI 图像接口（`/v1/images/*`，如 `gpt-image-*`）与多模态 Chat 生图（如 `gemini-3.1-flash-image`），支持文生图与图生图局部重绘。
- 📊 **可视化用量与号池监控**：基于终端 UI 库精心打磨的小鸡黄品牌色高对比度 TUI，实时展示今日/本周/总额度、Token 消耗、延迟统计与上游渠道健康度。
- 🛡️ **严格零破坏与原子备份**：外科式 TOML/JSONC/YAML 按行或节点级修改，绝不破坏用户原本的手写注释；每次修改前自动留存时间戳备份，支持随时回滚。
- 🪟 **极致的 Windows 体验**：动态切换代码页为 UTF-8（65001）、开启 VT 虚拟终端，遇传统 conhost 终端自动平滑降级 ASCII 边框，规避中文字符错位；针对 PowerShell 5.1 采用纯 ASCII Unicode 逃逸安装脚本，彻底避免乱码。

---

## 支持的环境与工具矩阵

| 分类 | 工具 / Harness | 配置文件位置 | 自动化能力说明 |
| :--- | :--- | :--- | :--- |
| **CLI 代理** | **Claude Code** | `~/.claude/settings.json`<br>`~/.claude.json` | 自动配置 `ANTHROPIC_BASE_URL` 与 Token；映射 Haiku 辅助检索模型以支持 **网络搜索**；补全 `~/.claude.json` 跳过 onboarding 流程；消除双 Key 冲突。 |
| **CLI 代理** | **OpenAI Codex CLI** | `~/.codex/config.toml` | 行级注入 `[model_providers.crosery]`，配置 `wire_api = "responses"` 与默认模型，保留原有设置。 |
| **CLI 代理** | **Pi** | `~/.pi/agent/models.json` | 自动配置小鸡云模型供应商与 `default_model`。 |
| **CLI 代理** | **oh-my-pi (omp)** | `~/.omp/agent/models.yml`<br>`config.yml` | 保持 YAML 注释与顺序，配置模型列表与 `modelRoles.default`。 |
| **CLI 代理** | **Nous Hermes** | `~/.hermes/config.yaml` | 注入 `providers.crosery`（`chat_completions` 协议）及 `model.provider`。 |
| **CLI 代理** | **OpenCode** | `~/.config/opencode/opencode.json` | 适配 `@ai-sdk/openai-compatible` 协议，写入全量上下文限制与 Tool 调用标记。 |
| **CLI 代理** | **Kilo Code** | `~/.config/kilo/config.json` | 写入 Kilo 规范的多模型供应商列表及 `active_provider`。 |
| **CLI 代理** | **Kimi Code** | `~/.kimi-code/config.toml` | 自动适配 Kimi 特有的 `max_output_size = 128000` 限制，规避 400 溢出。 |
| **CLI 代理** | **Qwen Code** | `~/.qwen/settings.json` | 配置通义千问 Agent 上游端点与 Key。 |
| **CLI 代理** | **OpenClaw** | `~/.openclaw/config.json` | 注入模型映射与网关认证信息。 |
| **CLI 代理** | **Crush** | `~/.crush/config.json` | 写入终端 AI 代理端点与模型规格。 |
| **CLI 代理** | **GitHub Copilot CLI** | `~/.copilot-cli/config.json` | 代理自定义模型端点覆盖。 |
| **CLI 代理** | **Factory Droid** | `~/.factory/droid.json` | 配置 Droid 代理引擎。 |
| **CLI 代理** | **Aider** | `~/.aider.conf.yml` | 写入 `openai-api-base` 与 `openai-api-key`。 |
| **CLI 代理** | **Continue** | `~/.continue/config.json` | 增量追加小鸡云模型列表至 Continue 配置。 |
| **CLI 代理** | **Gemini CLI** | `~/.gemini/.env`<br>`settings.json` | 提取当前 Key 可用的 Gemini 系列模型并设置 `GOOGLE_GEMINI_BASE_URL`。 |
| **CLI 代理** | **CodeBuddy CLI** | `~/.codebuddy/settings.json` | 腾讯云 CodeBuddy 命令行端点与模型配置。 |
| **CLI 代理** | **Qoder CLI** | `~/.qoder/settings.json` | 注入小鸡云模型供应商并设为活跃。 |
| **桌面 GUI** | **WorkBuddy / AI** | `~/.workbuddy/models.json` | 解析并安全重写 WorkBuddy 桌面端模型列表数组，设为默认选择。 |
| **桌面 GUI** | **Qoder CN (桌面)** | `~/.qoder/settings.json` | 同步配置桌面端设置文件。 |
| **桌面 GUI** | **Zed 编辑器** | `~/.config/zed/settings.json` | 配置 `language_models.openai_compatible` 模型列表，终端自动拷贝 Key 方便粘贴。 |
| **桌面 GUI** | **Cherry Studio** | 深度链接唤起 | 构造 `cherrystudio://` 专有链接一键唤起并导入服务商配置与模型。 |
| **桌面 GUI** | **Cursor** | 应用内指引 | 终端自动将 Key 拷入剪贴板，输出 Override Base URL 及推荐的模型名称清单。 |

---

## 一键安装

只需打开终端，执行对应平台的一行命令即可完成下载、校验与初始化。

> 💡 **国内加速说明**：安装脚本内部内置了**智能测速与多节点容灾回退**机制（自动在官方源与国内高速节点间测速切换）。在国内无代理或网络受限环境下，推荐使用下方的**国内高速加速命令**；海外或已有全局代理环境使用默认命令即可。

### 1. macOS & Linux

#### 国内高速安装（推荐）
```bash
# 交互式引导安装
curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/crosery/crapi/main/install.sh | sh

# 带 Key 免交互全自动安装（适合脚本与无头环境）
curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/crosery/crapi/main/install.sh | sh -s -- --key sk-your-key-here
```

#### 海外 / 全局代理环境
```bash
curl -fsSL https://raw.githubusercontent.com/crosery/crapi/main/install.sh | sh
```

---

### 2. Windows (PowerShell 5.1+ / PowerShell 7+)

以普通用户权限打开 PowerShell 窗口：

#### 国内高速安装（推荐）
```powershell
# 交互式引导安装
irm https://ghfast.top/https://raw.githubusercontent.com/crosery/crapi/main/install.ps1 | iex

# 带 Key 免交互全自动安装
$env:CRAPI_KEY='sk-your-key-here'; irm https://ghfast.top/https://raw.githubusercontent.com/crosery/crapi/main/install.ps1 | iex
```

#### 海外 / 全局代理环境
```powershell
irm https://raw.githubusercontent.com/crosery/crapi/main/install.ps1 | iex
```

---

### 3. Windows (传统命令提示符 cmd.exe)

```cmd
powershell -ExecutionPolicy Bypass -c "irm https://ghfast.top/https://raw.githubusercontent.com/crosery/crapi/main/install.ps1 | iex"
```

> **Windows 环境变量说明**：安装脚本会自动将 `crapi.exe` 所在目录注册到 Windows 用户注册表 `HKCU\Environment\Path`，并即时向系统广播环境刷新消息，通常在当前会话或新开窗口中即可直接键入 `crapi`。

---

## 常用命令与工作流

### 主菜单与一键配置 (setup)

直接运行 `crapi` 进入美化交互式控制台，支持键盘上下箭头与回车直观操作：

```bash
crapi
```

或者直接执行批量配置：

```bash
# 检测本机已安装的 harness 并列出选项
crapi setup

# 一键为本机已安装的全部工具完成配置
crapi setup --all

# 只针对指定工具进行配置（例如 Claude Code 和 Codex）
crapi setup claude codex

# 指定默认首选模型与开启 Claude Code 联网搜索
crapi setup claude --model claude-opus-5-5 --websearch

# 预览变更文件与内容，不实际落盘
crapi setup --dry-run
```

---

### 上游模型热更新 (update)

当小鸡云 CPA 网关上线新模型或调整渠道时，运行 `crapi update` 即可无缝同步：

```bash
crapi update
```

- **增量比对**：控制台会直观以 `+ 新增`（绿色）与 `- 下线`（红色）显示模型变动。
- **配置刷新**：自动更新所有已纳管的 Harness 本地模型目录，无需重新逐个设置。
- **保留偏好**：用户此前手动指定过的默认模型（只要依然有效）会继续保留，绝不随意覆盖。

---

### 权限与可用模型查询 (models / channels)

查询你持有的 API Key 拥有的上游渠道与模型权限列表：

```bash
# 查看所有支持的模型（按对话模型、多模态生图模型、音频模型分类）
crapi models

# 仅查看包含 "opus" 关键词的模型
crapi models --filter opus

# 查看当前 Key 拥有的上游渠道（如 anthropic, openai, openrouter, antigravity 等）
crapi channels
```

---

### 额度用量与近期请求监控 (usage)

通过精美设计的终端彩色进度条与表格，直观掌握账户消耗情况：

```bash
crapi usage
```

- **额度窗口**：今日已用/额度上限、本周额度、账号总额度进度条与重置倒计时。
- **统计指标**：累计消耗 USD、Token 消耗统计（Prompt / Completion）、平均延迟。
- **请求流水**：展示最近的请求时间、模型、状态码、输入/输出 Token 数及耗时。

---

### 上游号池健康状态 (pool)

实时了解 CPA 网关背后的号池健康度与负载情况：

```bash
crapi pool
```

- 查看各渠道活跃节点数、冷却中节点数、异常节点与禁用节点数。
- 查看渠道的当前速率限制（RPM/TPM）及窗口水位。

---

### 全量生图与图像编辑 (image)

内置强大的双通路生图引擎，自动适配网关所有生图模型：

```bash
# 文本生图（自动匹配当前 Key 下最佳生图模型，如 gpt-image 或 gemini-image）
crapi image "一只正在写代码的赛博小鸡，霓虹风格，8K高清"

# 指定模型、尺寸与输出路径
crapi image "太空宇航员在火星日落下的剪影" \
  --model gemini-3.1-flash-image \
  --size 1024x1024 \
  --out ./mars.png

# 图像编辑 / 局部重绘（Image-to-Image / Inpainting）
crapi image "将背景更换为繁星满天的夜空" \
  --image ./photo.png \
  --mask ./mask.png \
  --out ./edited.png
```

---

### 配置回滚与安全审计 (restore / doctor)

- **配置回滚**：每次 `crapi` 修改任何文件前，均会在 `~/.config/crapi/backups/` 目录下生成带时间戳的完整镜像与索引。
  ```bash
  # 列出所有可用的备份记录
  crapi restore --list

  # 将 Claude Code 的配置回滚至最近一次修改前的状态
  crapi restore claude
  ```

- **健康诊断**：
  ```bash
  crapi doctor
  ```
  一键检查本机网络连通性、网关 API 连通性、各 Harness 文件的合法性与模型匹配状态。

---

## 关键技术与工程实现

1. **零外部运行时依赖**：
   采用 Go 1.26 纯静态编译，无 CGO 依赖。二进制体积仅 ~9MB，跨架构支持 `darwin/amd64`、`darwin/arm64`、`linux/amd64`、`linux/arm64`、`windows/amd64`、`windows/arm64`、`windows/386`。
2. **非侵入式配置解析引擎**：
   - **JSON / JSONC**：自研 `internal/harness/ojson.go` 保序解码器，能容忍注释与尾逗号，修改字段时严格保持用户原本的字段书写顺序。
   - **TOML**：行级外科手术式更新器（`internal/harness/textedit.go`），通过 AST 与行范围定位，只替换目标表，绝不擦除用户的任何注释与空行。
   - **YAML**：基于 `yaml.v3` Node 树，原样保留标量行内注释与节点位置。
3. **Windows 终端自适应适配层**：
   - Windows 平台（`internal/term/term_windows.go`）在进程启动时通过 Win32 API 自动开启 UTF-8 代码页并激活 VT 处理；并在进程退出或捕获中断信号时可靠复原控制台属性。
   - 自动探测是否运行在传统 `conhost.exe` 下，若为非 Windows Terminal 环境，自动将边框渲染降级为单字节 ASCII，根治中文字符双宽造成的表格错位。
   - `install.ps1` 采用全 ASCII 编码并在运行时通过 `[regex]::Unescape` 动态还原中文提示，完美规避 PowerShell 5.1 在默认 GBK 编码下将网络下载脚本误认为 Latin-1 导致的中文乱码。

---

## 源码构建

如需从源码编译：

```bash
git clone https://github.com/crosery/crapi.git
cd crapi

# 本机编译并运行
go build -o crapi .
./crapi --version

# 运行完整单元测试
go test -v ./...

# 交叉编译全平台发布产物（产物输出至 dist/ 目录）
chmod +x scripts/build.sh
./scripts/build.sh 0.1.0
```

---

## 开源许可

本项目采用 [MIT 许可证](./LICENSE) 开源。欢迎提交 Issue 与 Pull Request！
