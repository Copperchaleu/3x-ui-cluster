# Docker Slave 测试指南

本指南介绍如何使用 Docker 完全隔离地运行多个 3x-ui Slave 进行测试。

## 前提条件

- Docker 已安装
- Docker Compose 已安装（可选，用于批量管理）
- Master 服务正在运行

## 文件说明

- `Dockerfile.slave` - Slave 容器镜像定义
- `docker-compose.slaves.yml` - Docker Compose 配置文件
- `docker-slaves.sh` - Slave 管理脚本

## 快速开始

### 方法 1：使用管理脚本（推荐）

#### 1. 构建 Slave 镜像

```bash
./docker-slaves.sh build
```

#### 2. 在 Master 面板添加节点

访问 http://192.168.10.192:2053/panel/slaves：
- 添加 Slave 1，复制 secret
- 添加 Slave 2，复制 secret
- 添加 Slave 3，复制 secret

#### 3. 启动单个 Slave

```bash
# 启动 slave1
./docker-slaves.sh start slave1 <SECRET_FROM_PANEL>

# 启动 slave2
./docker-slaves.sh start slave2 <SECRET_FROM_PANEL>

# 启动 slave3
./docker-slaves.sh start slave3 <SECRET_FROM_PANEL>
```

#### 4. 查看状态和日志

```bash
# 查看所有 slave 状态
./docker-slaves.sh status

# 查看 slave1 的日志
./docker-slaves.sh logs slave1

# 查看所有日志
./docker-slaves.sh logs
```

#### 5. 管理 Slaves

```bash
# 重启 slave
./docker-slaves.sh restart slave1

# 停止 slave
./docker-slaves.sh stop slave1

# 进入容器 shell
./docker-slaves.sh shell slave1

# 清理所有 slaves
./docker-slaves.sh cleanup
```

### 方法 2：使用 Docker Compose

#### 1. 编辑配置文件

编辑 `docker-compose.slaves.yml`，替换 secret：

```yaml
slave1:
  command: ["ws://host.docker.internal:2053/panel/api/slave/connect", "YOUR_ACTUAL_SECRET_1"]

slave2:
  command: ["ws://host.docker.internal:2053/panel/api/slave/connect", "YOUR_ACTUAL_SECRET_2"]

slave3:
  command: ["ws://host.docker.internal:2053/panel/api/slave/connect", "YOUR_ACTUAL_SECRET_3"]
```

#### 2. 启动所有 Slaves

```bash
./docker-slaves.sh start-all
```

或直接使用 docker-compose：

```bash
docker-compose -f docker-compose.slaves.yml up -d
```

#### 3. 停止所有 Slaves

```bash
./docker-slaves.sh stop-all
```

或：

```bash
docker-compose -f docker-compose.slaves.yml down
```

### 方法 3：手动使用 Docker 命令

#### 1. 构建镜像

```bash
docker build -f Dockerfile.slave -t 3x-ui-slave:latest .
```

#### 2. 运行容器

```bash
# Slave 1
docker run -d \
  --name 3x-ui-slave1 \
  --hostname slave1 \
  --restart unless-stopped \
  -v 3x-ui-slave1-data:/app/db \
  -v 3x-ui-slave1-logs:/app/log \
  3x-ui-slave:latest \
  ws://host.docker.internal:2053/panel/api/slave/connect \
  YOUR_SECRET_1

# Slave 2
docker run -d \
  --name 3x-ui-slave2 \
  --hostname slave2 \
  --restart unless-stopped \
  -v 3x-ui-slave2-data:/app/db \
  -v 3x-ui-slave2-logs:/app/log \
  3x-ui-slave:latest \
  ws://host.docker.internal:2053/panel/api/slave/connect \
  YOUR_SECRET_2
```

#### 3. 查看日志

```bash
docker logs -f 3x-ui-slave1
docker logs -f 3x-ui-slave2
```

#### 4. 停止和删除

```bash
docker stop 3x-ui-slave1 3x-ui-slave2
docker rm 3x-ui-slave1 3x-ui-slave2
docker volume rm 3x-ui-slave1-data 3x-ui-slave1-logs
docker volume rm 3x-ui-slave2-data 3x-ui-slave2-logs
```

## 常见问题

### Q1: 容器无法连接到 Master

**问题**：Slave 日志显示连接失败

**解决方案**：
- Linux: 使用 `--network host` 或 `--add-host=host.docker.internal:172.17.0.1`
- Mac/Windows: `host.docker.internal` 应该可以直接使用

修改 Master URL：
```bash
# Linux 使用实际 IP
docker run ... 3x-ui-slave:latest ws://192.168.10.192:2053/panel/api/slave/connect SECRET

# 或使用 host 网络
docker run --network host ... 3x-ui-slave:latest ws://127.0.0.1:2053/panel/api/slave/connect SECRET
```

### Q2: 如何查看容器内的文件

```bash
# 进入容器
./docker-slaves.sh shell slave1

# 或直接执行命令
docker exec 3x-ui-slave1 ls -la /app/db
docker exec 3x-ui-slave1 cat /app/log/3xui.log
```

### Q3: 如何更新 Slave 代码

```bash
# 停止所有 slaves
./docker-slaves.sh stop-all

# 重新构建镜像
./docker-slaves.sh build

# 重新启动
./docker-slaves.sh start-all
```

### Q4: 如何限制容器资源

在 `docker-compose.slaves.yml` 中添加：

```yaml
slave1:
  # ... 其他配置
  deploy:
    resources:
      limits:
        cpus: '0.5'
        memory: 512M
      reservations:
        cpus: '0.25'
        memory: 256M
```

或在 docker run 命令中：

```bash
docker run -d \
  --cpus="0.5" \
  --memory="512m" \
  ...
```

## 优势

✅ **完全隔离**：每个 Slave 在独立的容器中运行
✅ **易于管理**：统一的管理脚本
✅ **可复现性**：相同的镜像保证环境一致
✅ **资源控制**：可以限制每个 Slave 的 CPU/内存使用
✅ **快速清理**：一键删除所有测试环境

## 测试流程示例

```bash
# 1. 构建镜像
./docker-slaves.sh build

# 2. 在 Master 面板添加 3 个节点，获取 secrets

# 3. 启动 3 个 slaves
./docker-slaves.sh start slave1 secret_1_from_panel
./docker-slaves.sh start slave2 secret_2_from_panel
./docker-slaves.sh start slave3 secret_3_from_panel

# 4. 查看状态
./docker-slaves.sh status

# 5. 监控日志
./docker-slaves.sh logs

# 6. 在 Master 面板创建 Inbound 并分配给不同节点

# 7. 验证配置同步
./docker-slaves.sh logs slave1

# 8. 测试完成后清理
./docker-slaves.sh cleanup
```

## 网络配置

如果需要 Slave 容器可以被外部访问（例如测试实际流量），可以映射端口：

```bash
docker run -d \
  --name 3x-ui-slave1 \
  -p 10443:443 \
  -p 10080:80 \
  ...
```

或在 docker-compose.yml 中：

```yaml
slave1:
  ports:
    - "10443:443"
    - "10080:80"
```
