# Master 本地 Xray 冗余代码清理计划

## 背景
Master 节点不再直接运行 Xray，所有代理功能通过 Slave 节点实现。需要删除 Master 本地运行 Xray 的相关代码。

## 需要删除的文件

### 1. 流量收集任务
- `web/job/xray_traffic_job.go` - 完整删除

## 需要修改的文件

### 1. web/web.go
**删除代码（第 308 行）：**
```go
s.cron.AddJob("@every 10s", job.NewXrayTrafficJob())
```

### 2. web/service/inbound.go
**删除方法：**
- `AddTraffic()` (970-1013行) - Master 本地模式流量添加
- `addInboundTraffic()` (1015-1035行) - 添加 inbound 流量
- `addClientTraffic()` (1037-1095行) - 添加客户端流量
- `adjustTraffics()` (1097-1155行) - 调整流量数据

**保留方法：**
- `autoRenewClients()` - 自动续期功能仍需要
- `disableInvalidClients()` - 禁用无效客户端仍需要
- `disableInvalidInbounds()` - 禁用无效 inbound 仍需要

### 3. web/service/outbound.go
**删除方法：**
- `AddTraffic()` (16-35行) - Outbound 流量添加
- `addOutboundTraffic()` (37-66行) - 添加 outbound 流量

**保留方法：**
- `GetOutboundsTraffic()` - 获取流量统计
- `ResetOutboundTraffic()` - 重置流量

### 4. web/service/xray.go
**标记为废弃但保留的方法（向后兼容）：**
- `IsXrayRunning()` - 改为始终返回 false
- `GetXrayTraffic()` - 改为返回空数据
- `SetToNeedRestart()` - 已经是 no-op

**保留方法：**
- 其他 Xray 配置管理方法（用于生成 Slave 配置）

### 5. web/service/stats_notify_job.go
**修改代码（第 28-30 行）：**
```go
// 旧代码：
if !j.xrayService.IsXrayRunning() {
    return
}

// 新代码：
// Master 不运行 Xray，从数据库获取统计数据
```

### 6. web/service/server.go
**修改代码（第 392 行）：**
```go
// 旧代码：
if s.xrayService.IsXrayRunning() {
    return "running"
}

// 新代码：
// 检查 Slave 状态而不是本地 Xray
```

### 7. web/service/tgbot.go
**修改代码（第 649 行）：**
```go
// 旧代码：
if t.xrayService.IsXrayRunning() {
    // ...
}

// 新代码：
// 检查是否有在线 Slave
```

## 保留的功能

### 1. Xray 配置生成
- `web/service/xray.go` 中的配置生成方法（用于生成 Slave 配置）
- Inbound/Outbound 配置管理

### 2. 客户端管理
- 客户端自动续期 (`autoRenewClients`)
- 禁用无效客户端 (`disableInvalidClients`)
- 禁用无效 inbound (`disableInvalidInbounds`)

### 3. Slave 流量统计
- `web/service/slave.go` 中的 `ProcessTrafficStats()`
- `GetAllSlavesWithTraffic()`

## 清理优先级

### 🔴 高优先级
1. 删除 `web/job/xray_traffic_job.go`
2. 移除 `web/web.go` 中的 XrayTrafficJob 启动代码
3. 删除 `inbound.go` 中的 `AddTraffic()` 等方法

### 🟡 中优先级
4. 删除 `outbound.go` 中的 `AddTraffic()` 方法
5. 修改 `stats_notify_job.go` 从数据库获取统计

### 🟢 低优先级
6. 修改 `server.go` 和 `tgbot.go` 中的状态检查逻辑
7. 更新 `xray.go` 中废弃方法的实现

## 测试验证

清理后需要验证：
1. Master 启动无错误
2. Slave 连接正常
3. 流量统计正确收集和显示
4. 前端页面显示正常
5. 客户端管理功能正常（续期、禁用等）

## 实施步骤

1. 先执行 Git 工作流脚本（git-workflow.sh）
2. 在新分支 cleanup-master-xray 上执行删除操作
3. 逐个文件修改，每次修改后编译验证
4. 完成后提交并测试
