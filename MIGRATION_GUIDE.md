# 迁移指南 - Master/Slave 架构重构

## 概述

在新版本中，**Master 节点不再运行 Xray 代理程序**。Master 节点仅作为 Web 管理面板使用，所有代理功能必须在 Slave 节点上运行。

## 架构变化

### 旧架构
```
Master (Web Panel + Xray Proxy)
  ↓
Slave 1 (Xray Proxy)
Slave 2 (Xray Proxy)
```

### 新架构
```
Master (Web Panel Only)
  ↓
Slave 1 (Xray Proxy)
Slave 2 (Xray Proxy)
```

## 影响

1. **数据库变化**：
   - `inbounds`、`xray_outbounds`、`xray_routing_rules` 表中的 `slave_id` 字段不再接受 0（Master）作为值
   - 所有新配置必须指定一个实际存在的 Slave 服务器

2. **前端变化**：
   - 所有服务器选择下拉框中移除了 "Master" 选项
   - 添加/编辑 Inbound、Outbound、Routing Rule 时必须选择 Slave 服务器

3. **API 变化**：
   - Master 节点的 Xray 相关 API 返回错误或无操作
   - 配置更改会自动推送到对应的 Slave 节点

## 迁移步骤

### 1. 检查现有配置

启动新版本时，系统会自动检查是否存在 `slave_id=0` 的配置：

```
⚠️  WARNING: Found configurations assigned to Master node (SlaveId=0)
   - Inbounds: X
   - Outbounds: X
   - Routing Rules: X
```

### 2. 添加 Slave 服务器

如果 Master 节点所在的主机需要运行代理，请在该主机上安装并配置一个 Slave：

1. 进入 Web 面板的 "Slaves" 页面
2. 点击 "Add Slave" 获取连接命令
3. 在 Master 所在的主机上运行该命令（或在其他服务器上运行）

```bash
# 示例连接命令
./3x-ui-slave --master=https://your-master-ip:port --token=YOUR_TOKEN
```

### 3. 迁移现有配置

有两种方式处理旧的 Master 配置：

#### 方式 A：通过 Web 面板迁移（推荐）

1. 进入对应的配置页面（Inbounds / Xray Settings）
2. 编辑每个配置，将服务器从无效的 Master 改为新添加的 Slave
3. 保存配置

#### 方式 B：批量删除旧配置

如果旧的 Master 配置不再需要，可以直接删除：

```sql
-- 删除所有 Master 节点的配置
DELETE FROM inbounds WHERE slave_id=0;
DELETE FROM xray_outbounds WHERE slave_id=0;
DELETE FROM xray_routing_rules WHERE slave_id=0;
```

**注意**：此操作不可逆，请提前备份数据库！

### 4. 验证配置

1. 确认所有配置都已分配到 Slave 节点
2. 在 Slave 服务器上验证 Xray 进程正在运行
3. 测试代理连接是否正常

## 常见问题

### Q: 为什么要移除 Master 的代理功能？

A: 这样做有以下优点：
- **职责分离**：Master 专注于管理，Slave 专注于代理
- **更好的扩展性**：所有节点平等，易于水平扩展
- **更简单的部署**：Master 节点可以部署在任何地方，不需要开放代理端口

### Q: 我必须在 Master 主机上运行 Slave 吗？

A: 不是必须的。如果您不需要 Master 主机运行代理，可以将所有代理流量分配到其他 Slave 服务器。

### Q: 旧版本的数据会丢失吗？

A: 不会。数据库中的旧配置仍然保留，只是无法使用。您需要手动将它们迁移到 Slave 节点或删除。

### Q: 如何备份数据库？

A: 复制 `x-ui.db` 文件：

```bash
cp /etc/x-ui/x-ui.db /etc/x-ui/x-ui.db.backup
```

## 技术细节

### 代码变更

1. **web/service/xray.go**:
   - `GetXrayConfig()`: 返回错误，不再生成配置
   - `RestartXray()`, `StopXray()`: 改为 no-op
   - `SetToNeedRestart()`: 改为 no-op

2. **database/model/xray.go**:
   - `XrayOutbound.SlaveId`: 添加 `not null` 约束
   - `XrayRoutingRule.SlaveId`: 添加 `not null` 约束

3. **前端文件**:
   - `web/html/form/inbound.html`: 移除 Master 选项
   - `web/html/inbounds.html`: 移除 Master 标签显示
   - `web/html/xray.html`: 移除 Master 相关逻辑
   - `web/assets/js/model/outbound.js`: slaveId 默认值从 0 改为 null

## 联系支持

如果在迁移过程中遇到问题，请：
1. 查看日志文件：`/var/log/3x-ui.log`
2. 在 GitHub Issues 中报告问题
3. 提供详细的错误信息和日志
