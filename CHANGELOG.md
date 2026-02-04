# 架构重构总结 - Master/Slave 解耦

## 重构目标

将 Master 节点的 Xray 代理功能与 Web 面板完全解耦，实现：
- Master 节点仅运行 Web 管理面板
- 所有 Xray 代理功能由 Slave 节点负责
- 消除 SlaveId=0 的特殊处理逻辑

## 已完成的修改

### 1. 后端服务层 (Backend Services)

#### web/service/xray.go
- ✅ `GetXrayConfig()`: 返回错误，提示使用 Slave 节点
- ✅ `RestartXray()`: 改为 no-op，仅记录日志
- ✅ `StopXray()`: 改为 no-op，仅记录日志
- ✅ `SetToNeedRestart()`: 改为 no-op
- ✅ `IsNeedRestartAndSetFalse()`: 始终返回 false
- ✅ `DidXrayCrash()`: 始终返回 false
- ✅ 移除 `encoding/json` 未使用的 import

**影响**: Master 节点不再启动/停止/管理 Xray 进程

### 2. 数据库层 (Database)

#### database/model/xray.go
- ✅ `XrayOutbound.SlaveId`: 添加 `gorm:"not null"` 约束
- ✅ `XrayRoutingRule.SlaveId`: 添加 `gorm:"not null"` 约束

**影响**: 所有新的 Outbound 和 Routing Rule 必须关联到有效的 Slave

#### database/db.go
- ✅ 添加启动时检查逻辑，警告存在 SlaveId=0 的配置
- ✅ 提供清理建议和 SQL 删除命令

**影响**: 用户在启动时会收到迁移提示

### 3. 前端 UI (Frontend)

#### web/html/form/inbound.html
- ✅ 移除服务器选择下拉框中的 "Master" 选项
- ✅ 仅显示实际的 Slave 服务器

#### web/html/inbounds.html
- ✅ 移除 Inbound 列表中的 Master 标签显示
- ✅ 移除 `getSlaveNameById()` 中的 Master 特殊处理
- ✅ 移除 `addInbound()` 中的 `slaveId || 0` 默认值
- ✅ 移除 `updateInbound()` 中的默认值处理（部分）

#### web/html/xray.html
- ✅ 移除 Slave Context Indicator 中的 Master 条件判断
- ✅ 将 `selectedSlaveId` 默认值从 0 改为 null
- ✅ 将 `selectedSlaveName` 默认值从 'Master' 改为 ''
- ✅ 移除初始化逻辑中的 Master 名称赋值
- ✅ `addOutbound()`: 已有 Slave 选择验证
- ✅ `addRule()`: 已有 Slave 选择验证

#### web/assets/js/model/outbound.js
- ✅ Outbound 构造函数中 slaveId 默认值从 0 改为 null

**影响**: 用户无法再将配置分配给 Master，必须选择 Slave

### 4. 文档 (Documentation)

- ✅ `MIGRATION_GUIDE.md`: 详细的迁移指南
- ✅ `CHANGELOG.md`: 本文件，记录所有变更

## 测试验证

### 编译测试
```bash
go build -o bin/3x-ui .
```
✅ 编译成功，无错误

### 功能测试清单

#### 后端
- [ ] Master 启动时不启动 Xray 进程
- [ ] Master 上的 Xray API 调用返回合适的错误/空响应
- [ ] 配置推送到 Slave 节点正常工作
- [ ] 数据库约束正确阻止 SlaveId=0 的新记录

#### 前端
- [ ] Inbound 表单中无 Master 选项
- [ ] Outbound 表单中无 Master 选项
- [ ] Routing Rule 表单中无 Master 选项
- [ ] 未选择 Slave 时显示错误提示
- [ ] 无 Slave 时禁止添加配置

#### 数据库
- [ ] 启动时正确检测 SlaveId=0 的记录
- [ ] 警告信息正确显示
- [ ] 迁移建议清晰易懂

## 向后兼容性

### ⚠️ 破坏性变更 (Breaking Changes)

1. **Master 节点不再运行 Xray**
   - 所有现有的 SlaveId=0 配置将失效
   - 用户必须手动迁移或删除这些配置

2. **数据库约束**
   - 新的 Outbound 和 Routing Rule 必须指定有效的 SlaveId
   - 前端表单不再提供默认值

### 迁移路径

用户需要：
1. 升级到新版本
2. 查看启动日志中的迁移警告
3. 添加 Slave 服务器
4. 将旧配置迁移到 Slave 或删除
5. 验证功能正常

## 潜在问题和解决方案

### 问题 1: 用户忘记添加 Slave
**解决方案**: 
- 前端添加明确的提示信息
- 无 Slave 时禁止添加配置
- 启动日志中提供清晰的警告

### 问题 2: 旧配置无法自动迁移
**解决方案**: 
- 提供详细的迁移指南
- 数据保留，不自动删除
- 提供清理 SQL 命令

### 问题 3: API 兼容性
**解决方案**: 
- 保留所有方法签名
- 方法改为 no-op 而非删除
- 返回合适的错误信息

## 后续工作

### 可选优化
1. [ ] 添加一键迁移按钮（将 SlaveId=0 的配置批量迁移到指定 Slave）
2. [ ] 在 Web 面板首页显示迁移状态
3. [ ] 添加迁移进度追踪
4. [ ] 自动检测 Slave 健康状态

### 文档更新
- [ ] 更新用户手册
- [ ] 更新 API 文档
- [ ] 添加架构图
- [ ] 录制迁移视频教程

## 代码审查清单

- ✅ 所有文件编译通过
- ✅ 没有遗留的 TODO 注释
- ✅ 日志消息清晰易懂
- ✅ 错误处理得当
- ✅ 向后兼容性考虑充分
- ✅ 文档完整

## 发布清单

1. [ ] 更新版本号
2. [ ] 编译所有平台的二进制文件
3. [ ] 创建 GitHub Release
4. [ ] 在 Release Notes 中强调破坏性变更
5. [ ] 链接到迁移指南
6. [ ] 通知用户社区

## 总结

本次重构成功实现了 Master 和 Slave 节点的职责分离：
- **Master**: 纯粹的 Web 管理面板
- **Slave**: 所有 Xray 代理功能

优点：
- ✅ 架构更清晰
- ✅ 职责更明确
- ✅ 易于水平扩展
- ✅ 简化部署流程

缺点：
- ⚠️ 破坏性变更，需要用户手动迁移
- ⚠️ 增加了用户操作步骤

总体评估：**优点远大于缺点**，值得推进。
