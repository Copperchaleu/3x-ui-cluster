#!/bin/bash
set -e

echo "========== Master Xray 冗余代码清理脚本 =========="
echo "当前分支: $(git branch --show-current)"
echo ""

# 确认在正确的分支上
if [ "$(git branch --show-current)" != "cleanup-master-xray" ]; then
    echo "❌ 错误: 请先运行 git-workflow.sh 切换到 cleanup-master-xray 分支"
    exit 1
fi

echo "🔴 步骤 1: 删除 XrayTrafficJob 文件"
if [ -f "web/job/xray_traffic_job.go" ]; then
    rm web/job/xray_traffic_job.go
    echo "✅ 已删除 web/job/xray_traffic_job.go"
else
    echo "⚠️  文件不存在: web/job/xray_traffic_job.go"
fi

echo ""
echo "🔴 步骤 2: 检查需要修改的文件"
FILES_TO_MODIFY=(
    "web/web.go"
    "web/service/inbound.go"
    "web/service/outbound.go"
    "web/service/xray.go"
    "web/service/stats_notify_job.go"
    "web/service/server.go"
    "web/service/tgbot.go"
)

for file in "${FILES_TO_MODIFY[@]}"; do
    if [ -f "$file" ]; then
        echo "✅ $file - 存在"
    else
        echo "❌ $file - 不存在"
    fi
done

echo ""
echo "⚠️  注意: 文件修改需要手动完成或使用代码编辑工具"
echo "请参考 cleanup-plan.md 中的详细说明"
echo ""
echo "建议步骤:"
echo "1. 备份当前代码: git stash"
echo "2. 逐个修改文件"
echo "3. 每次修改后编译测试: go build -o 3x-ui main.go"
echo "4. 确认无误后提交: git commit -am 'refactor: 删除 Master 本地 Xray 冗余代码'"
echo ""
echo "========== 准备工作完成 =========="
