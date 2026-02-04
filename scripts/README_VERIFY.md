# Slave 配置验证工具

用于验证 Slave 节点上的实际运行配置是否与 Master 数据库中的设置一致。

## 功能特性

- ✅ 验证入站 (Inbounds) 配置同步
- ✅ 验证出站 (Outbounds) 配置同步
- ✅ 验证路由规则 (Routing Rules) 同步
- ✅ 检测缺失的配置
- ✅ 检测多余的配置
- ✅ 检测配置不匹配

## 使用方法

### 第一步：查看可用的 Slave

```bash
# 列出所有 Slave
./scripts/verify_slave_config.sh --list
```

输出示例：
```
📋 Available Slaves:
================================================================================
  ID: 4   | Name: slave1               | Address: 192.168.1.100:2053   | Status: online
  ID: 5   | Name: slave2               | Address: 192.168.1.101:2053   | Status: online
================================================================================

Total: 2 slave(s)
```

### 方式一：通过 Slave ID 验证（推荐）

```bash
# 验证 ID 为 4 的 Slave
./scripts/verify_slave_config.sh --slave-id 4

# 详细模式
./scripts/verify_slave_config.sh --slave-id 4 --verbose
```

### 方式二：通过 URL 和 Token 验证

当 Slave 地址未配置或想验证特定 URL 时使用：

```bash
./scripts/verify_slave_config.sh \
    --url http://192.168.1.100:2053 \
    --token your-secret-token
```

### 方式三：自定义数据库路径

```bash
./scripts/verify_slave_config.sh \
    --slave-id 4 \
    --db /path/to/x-ui.db \
    --verbose
```

## 参数说明

| 参数 | 简写 | 说明 |
|------|------|------|
| `--list` | `-l` | 列出数据库中所有 Slave |
| `--slave-id ID` | `-s` | Slave ID（从数据库获取信息） |
| `--url URL` | `-u` | Slave API URL |
| `--token TOKEN` | `-t` | Slave 认证令牌 |
| `--db PATH` | `-d` | Master 数据库路径（默认：./db/x-ui.db） |
| `--verbose` | `-v` | 详细输出模式 |
| `--help` | `-h` | 显示帮助信息 |

## 输出说明

### 成功示例

```
🔍 Starting configuration verification...

📋 Verifying Slave: MySlaveServer (ID: 1)
   Address: http://192.168.1.100:2053
   Status: online

================================================================================
  VERIFICATION RESULTS
================================================================================

📥 INBOUNDS:
  ✅ All inbounds match

📤 OUTBOUNDS:
  ✅ All outbounds match

📝 DETAILS:
  ℹ️  Found 5 routing rules on Slave (expected 5 from Master)

✅ All configurations match!
```

### 发现问题示例

```
📥 INBOUNDS:
  ❌ Missing on Slave: 2
  ⚠️  Extra on Slave: 1
  ⚠️  Mismatched: 1

📝 DETAILS:
  ❌ Inbound 'vmess-443' (Port: 443, Protocol: vmess) not found on Slave
  ❌ Inbound 'vless-80' (Port: 80, Protocol: vless) not found on Slave
  ⚠️  Inbound 'trojan-8443' port mismatch: expected 8443, got 8444
  ⚠️  Extra inbound 'old-vmess' found on Slave (not in Master config)

❌ Verification failed. Please check the details above.
```

## 验证原理

1. **从 Master 读取配置**：
   - 连接 Master 数据库（SQLite）
   - 查询指定 Slave 的所有入站、出站、路由规则配置
   - 只获取 `enable=true` 的配置

2. **从 Slave 获取配置**：
   - 通过 HTTP API 连接到 Slave 节点
   - 获取 Xray 当前运行的配置
   - 解析 JSON 配置文件

3. **比较配置**：
   - 按 Tag 匹配入站和出站
   - 比较端口、协议、设置等关键字段
   - 生成差异报告

## 前置要求

### Slave 节点要求

Slave 需要实现 API 端点：`GET /api/xray/config`

返回格式：
```json
{
  "inbounds": [...],
  "outbounds": [...],
  "routing": {
    "rules": [...]
  }
}
```

### 网络要求

- Master 能够访问 Slave 的 API 端点
- Slave 的防火墙允许 Master 的连接
- 正确的认证令牌

## 故障排查

### 错误：Slave ID not found

```bash
# 先列出所有可用的 Slave
./scripts/verify_slave_config.sh --list

# 然后使用正确的 ID
./scripts/verify_slave_config.sh --slave-id <正确的ID>
```

### 错误：Database not found

```bash
# 检查数据库路径
ls -l ./db/x-ui.db

# 或检查生产环境路径
ls -l /etc/x-ui/x-ui.db

# 使用自定义路径
./scripts/verify_slave_config.sh --slave-id 4 --db /path/to/x-ui.db
```

### 错误：Failed to connect to slave

1. 检查 Slave 地址是否正确配置：`./scripts/verify_slave_config.sh --list`
2. 检查 Slave 是否在运行
3. 检查网络连接：`curl http://slave-ip:port/api/xray/config`
4. 检查防火墙设置
5. 如果地址未配置，使用 `--url` 参数直接指定

### 错误：Slave returned status 401

- 检查认证令牌是否正确
- 确认 Slave 的 API 认证配置

## 自动化脚本

### 定期验证所有 Slave

```bash
#!/bin/bash
# verify_all_slaves.sh

DB_PATH="/etc/x-ui/x-ui.db"

# 获取所有 Slave ID
SLAVE_IDS=$(sqlite3 "$DB_PATH" "SELECT id FROM slaves WHERE status='online';")

for SLAVE_ID in $SLAVE_IDS; do
    echo "Verifying Slave ID: $SLAVE_ID"
    ./scripts/verify_slave_config.sh --slave-id "$SLAVE_ID"
    echo "---"
done
```

### 使用 Cron 定期检查

```bash
# 每小时验证一次
0 * * * * /path/to/3x-ui/scripts/verify_all_slaves.sh >> /var/log/slave-verify.log 2>&1
```

## 集成到监控系统

### Prometheus Exporter

可以将验证结果导出为 Prometheus 指标：

```bash
# 添加到 exporter 脚本
SLAVE_CONFIG_MATCH=$(./scripts/verify_slave_config.sh --slave-id 1 && echo 1 || echo 0)
echo "slave_config_match{slave_id=\"1\"} $SLAVE_CONFIG_MATCH"
```

### 告警规则

```yaml
- alert: SlaveConfigMismatch
  expr: slave_config_match == 0
  for: 5m
  annotations:
    summary: "Slave {{ $labels.slave_id }} configuration mismatch"
    description: "Slave configuration does not match Master database"
```

## 开发说明

### Go 代码结构

```
scripts/verify_slave_config.go
├── Database Models (Slave, Inbound, Outbound, RoutingRule)
├── Xray Config Models (XrayConfig)
├── getExpectedInbounds() - 从数据库读取预期配置
├── getSlaveConfig() - 从 Slave API 获取实际配置
├── compareConfigs() - 比较配置差异
└── printResults() - 格式化输出结果
```

### 添加新的验证项

在 `compareConfigs()` 函数中添加：

```go
// 验证新配置项
if expected.NewField != actual.NewField {
    diff.Details = append(diff.Details, 
        fmt.Sprintf("⚠️ Field mismatch: expected %v, got %v", 
            expected.NewField, actual.NewField))
}
```

## 相关工具

- `cleanup_master_configs.sh` - 清理 Master 节点的旧配置
- Master/Slave 同步机制文档：见 `MIGRATION_GUIDE.md`

## 支持

如遇问题，请提供：
1. 完整的错误输出
2. Master 和 Slave 的版本信息
3. 网络拓扑结构
4. 数据库查询结果（不含敏感信息）
