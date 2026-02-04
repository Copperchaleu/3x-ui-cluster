# 快速开始指南

## 验证 Slave 配置同步（开发测试工具）

本工具用于开发阶段验证 Web 界面的配置是否成功推送到 Slave 节点。通过 Docker 直接读取容器内的配置文件并与 Master 数据库对比。

### 1️⃣ 列出所有 Slave

```bash
./scripts/verify_slave_config.sh --list
```

**示例输出：**
```
📋 Available Slaves:
========================================================================================
  ID   | Name                 | Address                        | Status    
========================================================================================
  4    | slave1               | 192.168.1.100                  | online    
  5    | slave2               | 192.168.1.101                  | online    
========================================================================================

Total: 2 slave(s)

Usage:
  ./verify_slave_config.sh --slave-id <ID>
```

### 2️⃣ 验证指定 Slave

```bash
# 基本用法（需要明确指定容器名）
./scripts/verify_slave_config.sh --slave-id 4 --container 3x-ui-slave1

# 更简洁的方式（如果容器名与 Slave Name 一致）
./scripts/verify_slave_config.sh --slave-id 4 --container slave1

# 自定义配置文件路径
./scripts/verify_slave_config.sh --slave-id 4 --container 3x-ui-slave1 --config-path /custom/path/config.json
```

### 3️⃣ 查看详细信息

```bash
./scripts/verify_slave_config.sh --slave-id 4 --container 3x-ui-slave1 --verbose
```

---

## 工作原理

```
Master 数据库               Slave 服务器
┌─────────────┐            ┌──────────────────┐
│  inbounds   │            │  config.json     │
│  outbounds  │  SSH读取   │  (Xray配置)      │
│  routing    │  ───────>  │                  │
└─────────────┘            └──────────────────┘
       │                            │
       └────────── 对比配置 ─────────┘
                    │
                    ▼
              验证结果报告
```

**验证步骤：**
1. 从 Master 数据库读取该 Slave 的预期配置
2. 通过 `docker exec` 读取容器内的配置文件 (`/app/bin/config.json`)
3. 对比 Inbounds、Outbounds、Routing 等配置项
4. 生成差异报告

---

## 常见情况处理

### 情况 1：标准容器命名

如果使用 docker-compose 启动，容器名通常为 `3x-ui-slave1`, `3x-ui-slave2` 等：

```bash
# 查看容器名
docker ps --format "{{.Names}}"

# 使用实际容器名
./scripts/verify_slave_config.sh --slave-id 4 --container 3x-ui-slave1
```

### 情况 2：自动检测容器名
脚本默认使用数据库中的 Slave Name 作为容器名：

```bash
# Slave Name = "slave1" → 自动使用容器 "slave1"
./scripts/verify_slave_config.sh --slave-id 4
```

注意：如果容器名为 `3x-ui-slave1` 而数据库中只有 `slave1`，则需要明确指定。

### 情况 3：明确指定容器名
如果容器名与数据库不一致：

```bash
./scripts/verify_slave_config.sh --slave-id 4 --container my-slave-container
```

### 情况 4：自定义配置路径
如果 Slave 的配置文件不在默认位置：

```bash
./scripts/verify_slave_config.sh \
    --slave-id 4 \
    --container 3x-ui-slave1 \
    --config-path /custom/path/config.json
```

### 情况 5：开发环境验证
开发环境数据库默认在 `./db/x-ui.db`：

```bash
# 直接运行（默认使用 ./db/x-ui.db）
./scripts/verify_slave_config.sh --list
./scripts/verify_slave_config.sh --slave-id 4 --container 3x-ui-slave1
```

### 情况 6：生产环境验证

生产环境数据库在 `/etc/x-ui/x-ui.db`：

```bash
./scripts/verify_slave_config.sh \
    --slave-id 1 \
    --db /etc/x-ui/x-ui.db
```

---

## 预期结果

### ✅ 配置同步成功

```
================================================================================
  VERIFICATION RESULTS
================================================================================

📥 INBOUNDS:
  ✅ All inbounds match

📤 OUTBOUNDS:
  ✅ All outbounds match

✅ All configurations match!
```

### ⚠️ 发现配置不一致

```
📥 INBOUNDS:
  ❌ Missing on Slave: 2
  ⚠️  Extra on Slave: 1

📝 DETAILS:
  ❌ Inbound 'vmess-443' (Port: 443, Protocol: vmess) not found on Slave
  ⚠️  Extra inbound 'old-vmess' found on Slave (not in Master config)
```

**解决方案：**
1. 检查 Master 的 Web 面板配置是否已保存
2. 检查 Slave 与 Master 的 WebSocket 连接状态
3. 手动重启 Slave 的 Xray 服务
4. 在 Master Web 面板重新保存配置触发推送

---

## 故障排查

### 问题：找不到 Slave

```
❌ Slave ID 1 not found in database

💡 Available slaves:
   - ID: 4, Name: slave1, Status: online
   - ID: 5, Name: slave2, Status: online
```

**解决：** 使用 `--list` 查看正确的 ID，然后使用正确的 ID 进行验证。

### 问题：容器不存在或未运行

```
❌ Failed to read Slave config: docker exec error: No such container: slave1
```

**解决步骤：**
1. 检查容器是否运行：`docker ps | grep slave`
2. 启动容器：`docker start slave1`
3. 或明确指定容器名：`--container actual-container-name`

### 问题：配置文件不存在

```
❌ Failed to read Slave config: cat: /opt/3x-ui/bin/config.json: No such file or directory
```

**解决步骤：**
1. 进入容器确认路径：`docker exec -it slave1 ls -l /opt/3x-ui/bin/`
2. 使用 `--config-path` 参数指定正确路径
3. 确认 Slave 的 Xray 服务已启动

---

## 自动化验证

### 验证所有在线的 Slave

```bash
#!/bin/bash
# 保存为 verify_all.sh

DB_PATH="./db/x-ui.db"

echo "🔍 Verifying all online slaves..."

# 获取所有在线 Slave 的 ID
SLAVE_IDS=$(sqlite3 "$DB_PATH" "SELECT id FROM slaves WHERE status='online';")

for SLAVE_ID in $SLAVE_IDS; do
    echo ""
    echo "========================================="
    echo "Verifying Slave ID: $SLAVE_ID"
    echo "========================================="
    ./scripts/verify_slave_config.sh --slave-id "$SLAVE_ID" --db "$DB_PATH"
    echo ""
done

echo "✅ All slaves verified!"
```

### 定时验证（Cron）

```bash
# 每小时验证一次所有 Slave
0 * * * * /path/to/3x-ui-new/verify_all.sh >> /var/log/slave-verify.log 2>&1
```

---

## 注意事项

1. **Docker 访问**：需要在运行脚本的机器上有 Docker 访问权限
2. **容器运行状态**：Slave 容器必须处于运行状态
3. **容器命名**：通常为 `3x-ui-slave1`, `3x-ui-slave2` 等
4. **配置文件路径**：默认为 `/app/bin/config.json`
5. **仅用于开发测试**：此工具主要用于开发阶段验证配置推送是否成功

## 更多帮助

查看完整文档：[README_VERIFY.md](README_VERIFY.md)

查看帮助信息：
```bash
./scripts/verify_slave_config.sh --help
```
