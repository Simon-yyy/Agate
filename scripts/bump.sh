#!/usr/bin/env bash
# scripts/bump.sh - Agate 语义化版本升级与发布脚本
set -e

BUMP_TYPE="${1:-patch}"
DO_RELEASE=false

for arg in "$@"; do
    if [ "$arg" == "--release" ] || [ "$arg" == "-r" ]; then
        DO_RELEASE=true
    fi
done

ROOT_FILE="cmd/root.go"
if [ ! -f "$ROOT_FILE" ]; then
    echo "[FAIL] 未找到 $ROOT_FILE，请在项目根目录下执行"
    exit 1
fi

# 1. 提取当前版本号
CURRENT_VERSION=$(grep -E 'version\s*=\s*"[^"]+"' "$ROOT_FILE" | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$CURRENT_VERSION" ]; then
    echo "[FAIL] 无法从 $ROOT_FILE 解析当前版本号"
    exit 1
fi

# 2. 计算新版本号
IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT_VERSION"
MAJOR=${MAJOR:-0}
MINOR=${MINOR:-0}
PATCH=${PATCH:-0}

case "$BUMP_TYPE" in
    patch)
        PATCH=$((PATCH + 1))
        NEW_VERSION="${MAJOR}.${MINOR}.${PATCH}"
        ;;
    minor)
        MINOR=$((MINOR + 1))
        PATCH=0
        NEW_VERSION="${MAJOR}.${MINOR}.${PATCH}"
        ;;
    major)
        MAJOR=$((MAJOR + 1))
        MINOR=0
        PATCH=0
        NEW_VERSION="${MAJOR}.${MINOR}.${PATCH}"
        ;;
    *)
        # 直接指定具体版本号，如 0.2.0 或 v0.2.0
        CLEAN_VER="${BUMP_TYPE#v}"
        if [[ "$CLEAN_VER" =~ ^[0-9]+\.[0-9]+\.[0-9]+ ]]; then
            NEW_VERSION="$CLEAN_VER"
        else
            echo "[FAIL] 无效的版本升级类型: $BUMP_TYPE (支持: patch, minor, major 或指定版本号如 0.2.0)"
            exit 1
        fi
        ;;
esac

echo "=== [Agate 版本升级] ==="
echo "当前版本: v$CURRENT_VERSION"
echo "升级目标: v$NEW_VERSION"

# 3. 回填更新 cmd/root.go
if [[ "$OSTYPE" == "darwin"* ]]; then
    sed -i '' "s/version = \"$CURRENT_VERSION\"/version = \"$NEW_VERSION\"/" "$ROOT_FILE"
else
    sed -i "s/version = \"$CURRENT_VERSION\"/version = \"$NEW_VERSION\"/" "$ROOT_FILE"
fi
echo "[+] 已更新 $ROOT_FILE 版本号至 $NEW_VERSION"

# 4. 执行本地自检与全平台构建归档
echo "[+] 正在触发全平台编译与归档..."
./scripts/archive.sh

# 5. 更新本地当前环境可执行文件
if [ -f "bin/v$NEW_VERSION/linux/agate" ]; then
    mkdir -p "$HOME/.local/bin"
    cp -f "bin/v$NEW_VERSION/linux/agate" "$HOME/.local/bin/agate"
    chmod +x "$HOME/.local/bin/agate"
    echo "[+] 已同步刷新本地 ~/.local/bin/agate"
fi

echo -e "\n\033[92m[✓] 本地版本升级完毕: v$NEW_VERSION\033[0m"

# 6. 若指定了 --release 或在交互模式下提示
if [ "$DO_RELEASE" = true ]; then
    echo "[+] 正在提交并推送 Release 标签 (v$NEW_VERSION)..."
    git add "$ROOT_FILE" scripts/
    git commit -m "chore(release): bump version to v$NEW_VERSION"
    git tag -a "v$NEW_VERSION" -m "release: v$NEW_VERSION"
    ALLOW_AUTOMATED_PUSH=1 git push origin main
    ALLOW_AUTOMATED_PUSH=1 git push origin "v$NEW_VERSION"
    echo -e "\033[92m[✓] Release 标签 v$NEW_VERSION 已推送，GitHub Actions 已自动触发云端发布！\033[0m"
else
    echo -e "💡 提示：如需将此版本自动发布至 GitHub Releases，可执行："
    echo -e "   \033[96mgit add cmd/root.go && git commit -m \"chore(release): bump version to v$NEW_VERSION\"\033[0m"
    echo -e "   \033[96mgit tag -a \"v$NEW_VERSION\" -m \"release: v$NEW_VERSION\"\033[0m"
    echo -e "   \033[96mALLOW_AUTOMATED_PUSH=1 git push origin main && ALLOW_AUTOMATED_PUSH=1 git push origin v$NEW_VERSION\033[0m"
    echo -e "   或下次直接运行: \033[93m./scripts/bump.sh --release\033[0m 自动完成全流程发布。"
fi
