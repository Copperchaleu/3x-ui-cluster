#!/bin/bash
# 同步上游 3x-ui 项目更新的脚本

set -e

echo "========================================="
echo "  3x-ui Cluster - 同步上游更新"
echo "========================================="
echo ""

# 检查当前分支
CURRENT_BRANCH=$(git branch --show-current)
echo "当前分支: $CURRENT_BRANCH"
echo ""

# 获取上游更新
echo "📥 正在获取上游更新..."
git fetch upstream

# 检查是否有新提交
UPSTREAM_COMMITS=$(git log --oneline HEAD..upstream/main 2>/dev/null | wc -l)
if [ "$UPSTREAM_COMMITS" -eq 0 ]; then
    echo "✅ 已是最新版本，无需同步"
    exit 0
fi

echo ""
echo "📋 上游有 $UPSTREAM_COMMITS 个新提交:"
echo "----------------------------------------"
git log --oneline --graph HEAD..upstream/main | head -20
echo "----------------------------------------"
echo ""

# 询问是否合并
read -p "是否合并这些更新到当前分支 ($CURRENT_BRANCH)? (y/n): " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "❌ 已取消同步"
    exit 0
fi

# 如果不在 main 分支，先切换
if [ "$CURRENT_BRANCH" != "main" ]; then
    echo ""
    echo "⚠️  当前不在 main 分支"
    read -p "是否先切换到 main 分支进行同步? (y/n): " -n 1 -r
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "🔄 切换到 main 分支..."
        git checkout main
        CURRENT_BRANCH="main"
    fi
fi

# 执行合并
echo ""
echo "🔄 正在合并上游更新..."
if git merge upstream/main --no-edit; then
    echo "✅ 合并成功！"
    
    # 询问是否推送
    echo ""
    read -p "是否推送到远程仓库? (y/n): " -n 1 -r
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "📤 正在推送..."
        git push origin $CURRENT_BRANCH
        echo "✅ 推送完成！"
    fi
    
    # 如果原来在其他分支，询问是否切换回去
    if [ "$CURRENT_BRANCH" = "main" ] && [ "$(git branch --show-current)" = "main" ]; then
        echo ""
        read -p "是否将更新合并到 experimental/advanced-features 分支? (y/n): " -n 1 -r
        echo ""
        
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            echo "🔄 切换到 experimental/advanced-features 分支..."
            git checkout experimental/advanced-features
            echo "🔄 合并 main 分支的更新..."
            if git merge main --no-edit; then
                echo "✅ 合并成功！"
                
                read -p "是否推送到远程仓库? (y/n): " -n 1 -r
                echo ""
                
                if [[ $REPLY =~ ^[Yy]$ ]]; then
                    echo "📤 正在推送..."
                    git push origin experimental/advanced-features
                    echo "✅ 推送完成！"
                fi
            else
                echo "⚠️  合并遇到冲突，请手动解决"
                echo "解决冲突后运行："
                echo "  git add <冲突文件>"
                echo "  git commit"
                echo "  git push origin experimental/advanced-features"
            fi
        fi
    fi
    
else
    echo "⚠️  合并遇到冲突！"
    echo ""
    echo "请手动解决冲突，然后运行："
    echo "  git status                  # 查看冲突文件"
    echo "  # 编辑冲突文件，解决冲突"
    echo "  git add <冲突文件>"
    echo "  git commit"
    echo "  git push origin $CURRENT_BRANCH"
    exit 1
fi

echo ""
echo "========================================="
echo "  ✅ 同步完成！"
echo "========================================="
