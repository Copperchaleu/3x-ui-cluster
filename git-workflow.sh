#!/bin/bash
set -e

echo "========== Git Workflow Script =========="
echo "Step 1: Check current branch and status"
git branch --show-current
git status --short

echo ""
echo "Step 2: Add all changes"
git add -A

echo ""
echo "Step 3: Commit changes"
git commit -m "feat: 完成 Slave 模式流量统计功能

- 删除 traffic_stats 表，使用 inbounds 和 client_traffics 表
- 修复 ProcessTrafficStats() 直接更新 inbounds 表
- 添加用户级别流量统计收集和更新
- 修复 Slave WebSocket 连接 URL 路径
- 添加 Xray API inbound 和路由规则
- GetAllSlavesWithTraffic() 从 inbounds 表聚合流量
- 前端显示 Slave 总流量"

echo ""
echo "Step 4: Get current branch name"
CURRENT_BRANCH=$(git branch --show-current)
echo "Current branch: $CURRENT_BRANCH"

echo ""
echo "Step 5: Checkout to main branch"
git checkout main

echo ""
echo "Step 6: Merge current branch to main"
git merge "$CURRENT_BRANCH" --no-ff -m "Merge branch '$CURRENT_BRANCH' - Slave traffic stats完成"

echo ""
echo "Step 7: Create new cleanup branch"
git checkout -b cleanup-master-xray

echo ""
echo "========== Workflow Complete =========="
echo "Current branch: $(git branch --show-current)"
echo "Ready for cleanup work"
