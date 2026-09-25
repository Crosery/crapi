#!/bin/sh
# crapi 一键安装（macOS / Linux）
#   curl -fsSL https://crapi.crosery.com/install.sh | sh
#   curl -fsSL https://crapi.crosery.com/install.sh | sh -s -- --key sk-xxx   # 带 Key 全自动
# 可选环境变量：
#   CRAPI_DOWNLOAD_BASE  发布产物下载地址（需包含 VERSION / SHA256SUMS / crapi-<os>-<arch>）
#   CRAPI_INSTALL_DIR    安装目录，默认 ~/.local/bin
#   CRAPI_NO_SETUP=1     只安装，不进入配置流程
set -eu

BASE="${CRAPI_DOWNLOAD_BASE:-https://github.com/crosery/crapi/releases/latest/download}"
BASE="${BASE%/}"
DIR="${CRAPI_INSTALL_DIR:-$HOME/.local/bin}"

if [ -t 1 ]; then
  Y="$(printf '\033[33m')"; G="$(printf '\033[32m')"; R="$(printf '\033[31m')"; D="$(printf '\033[2m')"; N="$(printf '\033[0m')"
else
  Y=""; G=""; R=""; D=""; N=""
fi
say()  { printf '%s◆%s %s\n' "$Y" "$N" "$1"; }
ok()   { printf '%s✓%s %s\n' "$G" "$N" "$1"; }
die()  { printf '%s✗ %s%s\n' "$R" "$1" "$N" >&2; exit 1; }

case "$(uname -s)" in
  Darwin) OS=darwin ;;
  Linux)  OS=linux ;;
  MINGW*|MSYS*|CYGWIN*) die "Windows 请在 PowerShell 中运行：irm ${BASE%/releases/*}/install.ps1 | iex" ;;
  *) die "暂不支持的系统：$(uname -s)" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) die "暂不支持的 CPU 架构：$(uname -m)" ;;
esac

fetch() { # fetch <url> <out>
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --retry 2 --connect-timeout 15 -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -T 15 -O "$2" "$1"
  else
    die "需要 curl 或 wget"
  fi
}

ASSET="crapi-$OS-$ARCH"
TMP="$(mktemp -d 2>/dev/null || mktemp -d -t crapi)"
trap 'rm -rf "$TMP"' EXIT INT TERM

printf '\n%s  crapi · 小鸡云 CPA 一键接入%s\n\n' "$Y" "$N"
say "下载 $ASSET"
fetch "$BASE/$ASSET" "$TMP/crapi" || die "下载失败：$BASE/$ASSET（检查网络，或设置 CRAPI_DOWNLOAD_BASE 使用镜像）"

if fetch "$BASE/SHA256SUMS" "$TMP/SHA256SUMS" 2>/dev/null; then
  WANT="$(awk -v a="$ASSET" '$2==a || $2=="*"a {print $1}' "$TMP/SHA256SUMS")"
  if [ -n "$WANT" ]; then
    if command -v sha256sum >/dev/null 2>&1; then GOT="$(sha256sum "$TMP/crapi" | awk '{print $1}')"
    else GOT="$(shasum -a 256 "$TMP/crapi" | awk '{print $1}')"; fi
    [ "$WANT" = "$GOT" ] || die "校验和不一致，已中止安装"
    ok "校验通过"
  fi
fi

mkdir -p "$DIR"
chmod +x "$TMP/crapi"
mv -f "$TMP/crapi" "$DIR/crapi"
ok "已安装到 $DIR/crapi"

# 把安装目录加入 PATH（只追加一次）
case ":$PATH:" in
  *":$DIR:"*) ;;
  *)
    LINE="export PATH=\"$DIR:\$PATH\"  # crapi"
    case "${SHELL:-}" in
      */zsh)  RCS="$HOME/.zshrc" ;;
      */bash) RCS="$HOME/.bashrc $HOME/.bash_profile" ;;
      */fish) RCS="" ; mkdir -p "$HOME/.config/fish/conf.d"
              echo "fish_add_path $DIR  # crapi" > "$HOME/.config/fish/conf.d/crapi.fish" ;;
      *)      RCS="$HOME/.profile" ;;
    esac
    for rc in $RCS; do
      [ -f "$rc" ] || [ "$rc" = "$HOME/.zshrc" ] || [ "$rc" = "$HOME/.bashrc" ] || [ "$rc" = "$HOME/.profile" ] || continue
      grep -qs "# crapi" "$rc" || printf '\n%s\n' "$LINE" >> "$rc"
    done
    ok "已把 $DIR 加入 PATH（新开终端生效）"
    ;;
esac

if [ "${CRAPI_NO_SETUP:-}" = "1" ]; then
  printf '\n运行 %scrapi setup%s 开始配置。\n' "$Y" "$N"
  exit 0
fi

printf '\n'
# 通过 curl | sh 运行时标准输入是管道，交互界面需要接到终端上。
if [ -r /dev/tty ] && [ -w /dev/tty ] && ( : </dev/tty ) 2>/dev/null; then
  "$DIR/crapi" setup "$@" </dev/tty
else
  "$DIR/crapi" setup --yes "$@"
fi
