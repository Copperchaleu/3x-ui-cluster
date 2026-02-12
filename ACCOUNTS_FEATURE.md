# 多入站用户管理系统 (Multi-Inbound Account System)

## 功能简介

本系统实现了跨入站的用户账户管理功能，允许：
- 一个账户关联多个入站的多个客户端
- 账户级别的流量聚合统计和限制
- 账户级别的过期时间管理
- 统一的订阅链接（一个账户的所有节点）

## 核心组件

### 1. 数据库模型

#### Account（账户表）
- `id`: 主键
- `username`: 账户唯一标识
- `remark`: 备注
- `enable`: 是否启用
- `totalGB`: 总流量限制（GB）
- `expiryTime`: 过期时间
- `up/down`: 已使用的上传/下载流量
- `subId`: 订阅ID（UUID）
- `tgId`: Telegram ID
- `reset`: 流量重置周期（天）

#### AccountClient（账户-客户端关联表）
- `id`: 主键
- `accountId`: 关联的账户ID
- `inboundId`: 关联的入站ID
- `clientEmail`: 客户端Email（唯一）

#### ClientTraffic（新增字段）
- `accountId`: 关联的账户ID（新增）

### 2. 服务层

#### AccountService
位置：`web/service/account.go`

主要方法：
- `GetAccounts()`: 获取所有账户
- `AddAccount()`: 创建账户
- `UpdateAccount()`: 更新账户
- `DelAccount()`: 删除账户
- `AddClientToAccount()`: 关联客户端到账户
- `RemoveClientFromAccount()`: 移除客户端关联
- `GetAccountTraffic()`: 获取账户聚合流量
- `CheckAccountTrafficLimit()`: 检查流量限制
- `ResetAccountTraffic()`: 重置账户流量
- `SyncAccountTraffic()`: 同步账户流量
- `DisableClientsExceedingAccountLimit()`: 禁用超限客户端
- `DisableExpiredAccountClients()`: 禁用过期账户的客户端

### 3. 控制器

#### AccountController
位置：`web/controller/account.go`

API 路由：
- `GET /panel/api/account/list`: 获取账户列表
- `POST /panel/api/account/add`: 添加账户
- `POST /panel/api/account/update/:id`: 更新账户
- `POST /panel/api/account/del/:id`: 删除账户
- `GET /panel/api/account/:id/clients`: 获取账户的客户端
- `POST /panel/api/account/:id/clients/add`: 添加客户端到账户
- `POST /panel/api/account/:id/clients/remove/:email`: 移除客户端
- `GET /panel/api/account/:id/traffic`: 获取账户流量
- `POST /panel/api/account/:id/traffic/reset`: 重置账户流量

### 4. 订阅功能

#### 订阅路由
- `GET /sub/account/:subId`: 按账户生成订阅链接
- `GET /sub/account/json/:subId`: 按账户生成JSON订阅（TODO）

#### SubService 新增方法
- `GetSubsByAccountId()`: 为账户生成所有关联节点的订阅链接
- `getInboundsByAccountId()`: 获取账户关联的所有入站

### 5. 定时任务

#### CheckAccountLimitJob
位置：`web/job/check_account_limit_job.go`

执行频率：每2分钟

功能：
- 检查账户流量限制，禁用超限客户端
- 检查账户过期时间，禁用过期客户端

### 6. 前端页面

#### 账户管理页面
位置：`web/html/accounts.html`

访问路径：`/panel/accounts`

功能：
- 账户列表展示
- 添加/编辑账户
- 流量统计显示
- 客户端管理
- 订阅链接复制
- 流量重置

## 使用流程

### 1. 创建账户
1. 访问 `/panel/accounts`
2. 点击"添加账户"
3. 填写账户信息：
   - 用户名（必填，唯一）
   - 备注
   - 流量限制（GB，0表示无限制）
   - 过期时间（可选）
4. 保存

### 2. 关联客户端
1. 在账户列表中点击"管理客户端"
2. 点击"添加客户端"
3. 选择入站
4. 输入客户端Email（必须是已存在于该入站的客户端）
5. 确认

### 3. 获取订阅链接
1. 在账户列表中点击"复制订阅链接"
2. 订阅链接格式：`https://your-domain/sub/account/{subId}`
3. 该链接包含该账户的所有节点

### 4. 监控账户
- 流量使用情况实时显示在账户列表
- 流量超限或过期的账户会自动禁用其所有客户端
- 每2分钟检查一次

## 数据流

### 流量统计流程
1. Xray 收集客户端流量数据（按Email）
2. `InboundService.addClientTraffic()` 更新客户端流量
3. 同步更新账户流量（按AccountId聚合）
4. 定时任务检查账户限制

### 流量聚合逻辑
```sql
SELECT SUM(up), SUM(down) 
FROM client_traffics 
WHERE account_id = ?
```

账户的流量 = 所有关联客户端的流量之和

## 注意事项

### 1. 客户端Email唯一性
- 每个客户端Email只能关联一个账户
- 如果客户端已关联账户，无法再关联到其他账户

### 2. 流量限制优先级
- 账户级流量限制优先于客户端级限制
- 账户超限时，所有关联客户端都会被禁用
- 即使客户端自身未超限

### 3. 入站删除
- 删除入站会自动删除相关的AccountClient关联
- 但不会删除账户本身

### 4. 账户删除
- 删除账户会：
  - 删除所有AccountClient关联
  - 将ClientTraffic的accountId重置为0
  - 不会删除客户端本身

### 5. 订阅更新
- 订阅链接中只包含启用的入站和客户端
- 账户禁用或过期时订阅会返回错误

## 数据库迁移

系统启动时自动执行：
1. 创建`accounts`表
2. 创建`account_clients`表
3. 在`client_traffics`表添加`account_id`列（如果不存在）
4. 创建索引

## API 测试示例

### 创建账户
```bash
curl -X POST http://localhost:2053/panel/api/account/add \
  -H "Content-Type: application/json" \
  -d '{
    "username": "user001",
    "remark": "测试账户",
    "enable": true,
    "totalGB": 100,
    "expiryTime": 0
  }'
```

### 关联客户端
```bash
curl -X POST http://localhost:2053/panel/api/account/1/clients/add \
  -H "Content-Type: application/json" \
  -d '{
    "inboundId": 1,
    "clientEmail": "client@example.com"
  }'
```

### 获取账户流量
```bash
curl http://localhost:2053/panel/api/account/1/traffic
```

### 获取订阅
```bash
curl http://localhost:2053/sub/account/{subId}
```

## 扩展建议

### 短期优化
1. 实现账户级JSON订阅
2. 添加账户流量图表
3. 支持批量导入账户
4. 账户流量报警（Telegram通知）

### 长期规划
1. 账户组管理
2. 账户余额和套餐系统
3. 流量包购买
4. 多管理员权限（不同管理员管理不同账户）

## 故障排查

### 流量未同步
检查定时任务是否正常运行：
```bash
# 查看日志
tail -f /var/log/3x-ui/access.log
```

### 订阅链接无法访问
1. 检查账户状态
2. 检查客户端是否已添加到入站
3. 检查客户端Email是否正确

### 数据库迁移失败
手动执行：
```sql
ALTER TABLE client_traffics ADD COLUMN account_id INTEGER DEFAULT 0;
CREATE INDEX idx_client_traffics_account_id ON client_traffics(account_id);
```

## 版本信息

- 功能版本：v1.0.0
- 兼容版本：3x-ui v2.x
- 开发日期：2026-02-11
