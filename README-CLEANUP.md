# Master 本地 Xray 代码清理指南

## 执行步骤

### 阶段 1: Git 工作流（合并代码并创建清理分支）

```bash
cd /home/graypaul/Projects/3x-ui-new
chmod +x git-workflow.sh
./git-workflow.sh
```

**该脚本将执行：**
1. 提交当前所有修改（Slave 流量统计功能完成）
2. 切换到 main 分支
3. 合并当前分支到 main
4. 创建新分支 `cleanup-master-xray`

### 阶段 2: 执行清理工作

```bash
chmod +x cleanup-execute.sh
./cleanup-execute.sh
```

**该脚本将：**
1. 删除 `web/job/xray_traffic_job.go` 文件
2. 检查需要修改的文件列表

### 阶段 3: 手动修改代码

由于代码修改需要精确操作，建议使用 VS Code 或其他编辑工具按照以下顺序修改：

#### 3.1 删除 XrayTrafficJob 启动代码

**文件**: `web/web.go` (第 308 行)

删除：
```go
s.cron.AddJob("@every 10s", job.NewXrayTrafficJob())
```

#### 3.2 删除 InboundService 中的废弃方法

**文件**: `web/service/inbound.go`

删除以下方法（约 970-1160 行）：
- `AddTraffic()`
- `addInboundTraffic()`
- `addClientTraffic()`
- `adjustTraffics()`

**保留方法**：
- `autoRenewClients()`
- `disableInvalidClients()`
- `disableInvalidInbounds()`

#### 3.3 删除 OutboundService 中的废弃方法

**文件**: `web/service/outbound.go`

删除方法（约 16-66 行）：
- `AddTraffic()`
- `addOutboundTraffic()`

#### 3.4 修改 XrayService 方法为废弃状态

**文件**: `web/service/xray.go`

修改 `IsXrayRunning()`:
```go
// IsXrayRunning is deprecated - Master node no longer runs Xray locally.
// Always returns false. Check Slave status instead.
func (s *XrayService) IsXrayRunning() bool {
	logger.Debug("IsXrayRunning called on Master - always returns false")
	return false
}
```

修改 `GetXrayTraffic()`:
```go
// GetXrayTraffic is deprecated - Master node no longer runs Xray locally.
// Returns empty data. Use Slave traffic stats instead.
func (s *XrayService) GetXrayTraffic() ([]*xray.Traffic, []*xray.ClientTraffic, error) {
	logger.Debug("GetXrayTraffic called on Master - returning empty data")
	return []*xray.Traffic{}, []*xray.ClientTraffic{}, nil
}
```

#### 3.5 修改 StatsNotifyJob

**文件**: `web/job/stats_notify_job.go` (第 28-30 行)

修改：
```go
// 旧代码
if !j.xrayService.IsXrayRunning() {
    return
}

// 新代码
// Master no longer runs Xray - get stats from database
db := database.GetDB()
var inbounds []*model.Inbound
if err := db.Model(model.Inbound{}).Find(&inbounds).Error; err != nil {
    logger.Warning("Failed to get inbounds for stats notification:", err)
    return
}

var totalTraffic int64
for _, inbound := range inbounds {
    totalTraffic += inbound.Up + inbound.Down
}

if totalTraffic == 0 {
    return
}
```

#### 3.6 修改 ServerService

**文件**: `web/service/server.go` (第 392 行)

修改：
```go
// 旧代码
if s.xrayService.IsXrayRunning() {
    return "running"
}

// 新代码
// Master doesn't run Xray - check Slave status
slaveService := &SlaveService{}
slaves, _ := slaveService.GetAllSlaves()
onlineCount := 0
for _, slave := range slaves {
    if slave.Status == "online" {
        onlineCount++
    }
}
if onlineCount > 0 {
    return fmt.Sprintf("running (%d slaves online)", onlineCount)
}
return "stopped"
```

#### 3.7 修改 TelegramBot

**文件**: `web/service/tgbot.go` (第 649 行及其他相关位置)

查找所有 `IsXrayRunning()` 调用，修改为检查 Slave 状态：
```go
// 旧代码
if t.xrayService.IsXrayRunning() {
    // ...
}

// 新代码
slaveService := &SlaveService{}
slaves, _ := slaveService.GetAllSlaves()
hasOnlineSlave := false
for _, slave := range slaves {
    if slave.Status == "online" {
        hasOnlineSlave = true
        break
    }
}
if hasOnlineSlave {
    // ...
}
```

### 阶段 4: 编译测试

每次修改后编译测试：

```bash
go build -o 3x-ui main.go
```

如果编译成功，进行功能测试：

```bash
# 启动 Master
sudo ./3x-ui

# 在另一个终端检查日志
tail -f /var/log/3x-ui/access.log

# 测试前端访问
curl http://localhost:2053/panel
```

### 阶段 5: 提交代码

```bash
# 查看修改
git diff

# 提交修改
git add -A
git commit -m "refactor: 删除 Master 本地 Xray 冗余代码

- 删除 XrayTrafficJob 及相关启动代码
- 删除 InboundService.AddTraffic() 等废弃方法
- 删除 OutboundService.AddTraffic() 废弃方法
- 修改 XrayService 方法为废弃状态（始终返回 false/空数据）
- 修改 StatsNotifyJob 从数据库获取统计
- 修改 ServerService 和 TgBot 检查 Slave 状态
- 保留客户端管理功能（续期、禁用等）"

# 推送到远程（如果需要）
git push origin cleanup-master-xray
```

### 阶段 6: 合并到 main（可选）

```bash
# 切换到 main
git checkout main

# 合并清理分支
git merge cleanup-master-xray --no-ff -m "Merge cleanup-master-xray: 删除 Master 本地 Xray 冗余代码"

# 推送到远程
git push origin main
```

## 验证清单

清理完成后验证以下功能：

- [ ] Master 服务启动无错误
- [ ] Slave 可以正常连接 Master
- [ ] Slave 流量统计正常收集
- [ ] 前端 Slaves 页面显示流量数据
- [ ] 前端 Inbounds 页面显示正常
- [ ] 客户端自动续期功能正常
- [ ] 禁用无效客户端功能正常
- [ ] Telegram Bot 状态检查正常
- [ ] 统计通知功能正常

## 回滚方案

如果出现问题，可以快速回滚：

```bash
# 回到清理前的状态
git checkout main

# 或者撤销清理分支的修改
git revert <commit-hash>
```

## 注意事项

1. **不要删除配置生成相关代码** - Master 仍需生成 Xray 配置发送给 Slave
2. **保留客户端管理功能** - 续期、禁用等功能仍然需要
3. **测试完整流程** - 确保 Slave 模式下所有功能正常
4. **备份数据库** - 清理前备份 `db/x-ui.db`
5. **分步提交** - 可以分多个小提交，便于回滚

## 文件清单

### 已创建的辅助文件

- `git-workflow.sh` - Git 工作流自动化脚本
- `cleanup-execute.sh` - 清理执行脚本
- `cleanup-plan.md` - 详细清理计划
- `README-CLEANUP.md` - 本文件（清理指南）

### 需要修改的文件

1. ~~`web/job/xray_traffic_job.go`~~ - 删除
2. `web/web.go` - 删除 1 行
3. `web/service/inbound.go` - 删除 4 个方法
4. `web/service/outbound.go` - 删除 2 个方法
5. `web/service/xray.go` - 修改 2 个方法为废弃状态
6. `web/job/stats_notify_job.go` - 修改统计获取逻辑
7. `web/service/server.go` - 修改状态检查逻辑
8. `web/service/tgbot.go` - 修改状态检查逻辑

## 预估时间

- 阶段 1: 5 分钟
- 阶段 2: 5 分钟
- 阶段 3: 30-60 分钟（手动修改代码）
- 阶段 4: 10-20 分钟（编译测试）
- 阶段 5: 5 分钟（提交）

**总计**: 约 1-2 小时

## 支持

如有问题，参考：
- `cleanup-plan.md` - 详细技术方案
- Git commit history - 查看修改历史
- 原有代码备份 - main 分支或之前的 commit
