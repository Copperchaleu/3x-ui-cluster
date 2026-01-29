# Slave 测试指南

本文档说明如何使用 Docker 在本地运行多个隔离的 Slave 节点进行测试。

## 前置条件

1. Docker 已安装并运行
2. Master 面板已启动（运行在 `192.168.10.192:2053`）
3. 已编译 `3x-ui` 二进制文件

## 快速开始

### 1. 构建 Slave Docker 镜像

```bash
./docker-slaves.sh build
```

### 2. 在 Master 面板添加节点

访问 Master 面板的节点管理页面：
- URL: `http://192.168.10.192:2053/panel/nodes`
- 点击"添加节点"，输入节点名称（如 slave1）
- 复制生成的 Secret 密钥

### 3. 启动 Slave 容器

使用复制的 Secret 启动 Slave：

```bash
MASTER_IP=192.168.10.192 ./docker-slaves.sh start slave1 <YOUR_SECRET>
```

**重要**: 必须设置 `MASTER_IP` 环境变量为 Master 的实际 IP 地址。

示例：
```bash
MASTER_IP=192.168.10.192 ./docker-slaves.sh start slave1 iVeD1pMbyWjH5tQeCZnL8wUiF3qOcAXl
```

### 4. 查看 Slave 状态

查看所有容器状态：
```bash
./docker-slaves.sh status
```

查看 Slave 日志：
```bash
./docker-slaves.sh logs slave1
```

### 5. 启动多个 Slave

重复步骤 2-3 为每个新 Slave：

```bash
# Slave 2
MASTER_IP=192.168.10.192 ./docker-slaves.sh start slave2 <SECRET_2>

# Slave 3
MASTER_IP=192.168.10.192 ./docker-slaves.sh start slave3 <SECRET_3>
```

## 管理命令

### 构建镜像
```bash
./docker-slaves.sh build
```

### 启动单个 Slave
```bash
MASTER_IP=192.168.10.192 ./docker-slaves.sh start <name> <secret>
```

### 停止 Slave
```bash
./docker-slaves.sh stop <name>
# 或停止所有
./docker-slaves.sh stop
```

### 查看日志
```bash
./docker-slaves.sh logs <name>
```

### 查看状态
```bash
./docker-slaves.sh status
```

### 清理所有容器
```bash
./docker-slaves.sh cleanup
```

## 验证连接

成功连接的日志应该包含：

```
INFO - Starting Slave...
INFO - Connecting to ws://192.168.10.192:2053/panel/api/slave/connect?secret=...
INFO - Connected to Master
INFO - Applying new configuration...
INFO - Xray started successfully
```

在 Master 面板的节点列表中，对应节点的状态应显示为"在线"（绿色）。

## 常见问题

### 1. 连接被拒绝 (connection refused)

检查：
- Master 是否在运行
- `MASTER_IP` 是否正确
- 防火墙是否允许 2053 端口

### 2. DNS 解析失败 (no such host)

Linux 系统不支持 `host.docker.internal`，必须使用 `MASTER_IP` 环境变量指定实际 IP。

### 3. 容器反复重启

查看日志：
```bash
./docker-slaves.sh logs <name>
```

常见原因：
- Secret 不正确
- Master URL 不可达
- Logger 未初始化（已在 v2 中修复）

### 4. 如何更改 Master IP

设置 `MASTER_IP` 环境变量：

```bash
export MASTER_IP=192.168.10.192
./docker-slaves.sh start slave1 <secret>
```

或在命令中直接指定：
```bash
MASTER_IP=192.168.10.192 ./docker-slaves.sh start slave1 <secret>
```

## 数据持久化

每个 Slave 容器使用独立的 Docker volumes：
- 数据库: `3x-ui-{name}-data` -> `/app/db`
- 日志: `3x-ui-{name}-logs` -> `/app/log`

查看 volumes：
```bash
docker volume ls | grep 3x-ui
```

清理 volumes：
```bash
docker volume rm 3x-ui-slave1-data 3x-ui-slave1-logs
```

## 架构说明

```
┌─────────────────┐
│  Master Panel   │  (192.168.10.192:2053)
│   WebSocket     │
└────────┬────────┘
         │
    ┌────┴────┬────────┬────────┐
    │         │        │        │
┌───▼───┐ ┌──▼───┐ ┌──▼───┐ ┌──▼───┐
│Slave 1│ │Slave2│ │Slave3│ │Slave4│
│Docker │ │Docker│ │Docker│ │Docker│
└───────┘ └──────┘ └──────┘ └──────┘
```

- Master 运行 Web 面板和 WebSocket 服务
- 每个 Slave 运行在独立的 Docker 容器中
- Slave 通过 WebSocket 连接到 Master
- 支持动态配置下发和状态监控

## 下一步

1. ✅ Slave 容器正常运行
2. ✅ 成功连接到 Master
3. ✅ Xray 启动成功
4. 🔄 测试配置下发
5. 🔄 测试多 Slave 并发
6. 🔄 测试节点断线重连
