# 3x-ui Master-Slave 安装指南

## 架构说明

此版本的 3x-ui 支持 Master-Slave 架构，其中：
- **Master**: 管理面板，用于配置和管理所有节点
- **Slave/Agent**: 从服务器，自动从 Master 获取配置并运行 Xray

## 安装步骤

### 1. 安装 Master 节点

在主服务器上运行标准安装命令：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/mhsanaei/3x-ui/master/install.sh)
```

安装完成后，访问 Web 面板（默认端口 2053）。

### 2. 在 Master 面板中添加 Slave 节点

1. 登录 Master 面板
2. 访问 "Nodes" 页面
3. 点击 "Add Node" 按钮
4. 输入节点名称（例如：US-Server-1）
5. 点击 OK，系统将自动生成安装命令

### 3. 复制安装命令

添加节点后，将自动弹出安装命令对话框，命令格式如下：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/mhsanaei/3x-ui/master/install.sh) agent http://MASTER-IP:2053 SECRET-KEY
```

点击 "Copy" 按钮复制命令。

### 4. 在 Slave 服务器上执行安装命令

将复制的命令粘贴到 Slave 服务器上执行：

```bash
bash <(curl -Ls https://raw.githubusercontent.com/mhsanaei/3x-ui/master/install.sh) agent http://192.168.1.100:2053 abc123xyz789...
```

安装完成后，Agent 将自动：
- 连接到 Master
- 接收配置
- 启动 Xray 服务

## Agent 管理命令

在 Slave 服务器上，可以使用以下命令管理 Agent：

```bash
# 启动 Agent
systemctl start x-ui-agent

# 停止 Agent
systemctl stop x-ui-agent

# 重启 Agent
systemctl restart x-ui-agent

# 查看状态
systemctl status x-ui-agent

# 查看日志
journalctl -u x-ui-agent -f
```

## 配置 Inbound

1. 在 Master 面板中创建或编辑 Inbound
2. 在 "Node" 字段中选择目标节点
3. 保存后，配置将自动推送到选定的 Slave 节点

## 工作流程

```
Master Panel (Web UI)
    ↓
  添加节点
    ↓
生成安装命令 (包含 Master URL + Secret)
    ↓
在 Slave 上执行安装命令
    ↓
Slave 自动连接到 Master (WebSocket)
    ↓
Master 推送配置到 Slave
    ↓
Slave 应用配置并启动 Xray
    ↓
Slave 定期上报状态 (CPU, 内存)
```

## 故障排查

### Agent 无法连接到 Master

1. 检查 Master 服务是否运行：
   ```bash
   systemctl status x-ui
   ```

2. 检查防火墙是否开放端口：
   ```bash
   # Master 端口默认 2053
   ufw allow 2053/tcp
   ```

3. 验证 Secret 是否正确：
   - 在 Master 面板的 Nodes 页面查看节点信息
   - 点击 "Install Cmd" 查看正确的安装命令

### 查看 Agent 日志

```bash
# 实时查看日志
journalctl -u x-ui-agent -f

# 查看最近 100 行日志
journalctl -u x-ui-agent -n 100

# 查看启动错误
journalctl -u x-ui-agent -b
```

### 重新安装 Agent

如果需要重新安装 Agent：

1. 停止并删除现有服务：
   ```bash
   systemctl stop x-ui-agent
   systemctl disable x-ui-agent
   rm /etc/systemd/system/x-ui-agent.service
   systemctl daemon-reload
   ```

2. 从 Master 面板获取新的安装命令并重新执行

## 安全建议

1. **使用 HTTPS**: 在生产环境中，建议配置 SSL 证书并使用 HTTPS
2. **防火墙**: 只开放必要的端口，限制 Master 面板访问
3. **Secret 保护**: 安装命令中的 Secret 请妥善保管，不要泄露
4. **定期更新**: 及时更新 Master 和 Agent 到最新版本

## 技术细节

- **通信协议**: WebSocket (ws:// 或 wss://)
- **认证方式**: 基于 Secret 的令牌认证
- **配置同步**: Master 主动推送配置到 Agent
- **状态上报**: Agent 每 5 秒上报一次系统状态
- **自动重连**: Agent 断线后每 5 秒自动尝试重连

## 命令参考

### 安装命令格式

```bash
# Master 安装
bash <(curl -Ls https://raw.githubusercontent.com/mhsanaei/3x-ui/master/install.sh)

# Agent 安装
bash <(curl -Ls https://raw.githubusercontent.com/mhsanaei/3x-ui/master/install.sh) agent <MASTER_URL> <SECRET>
```

### 参数说明

- `<MASTER_URL>`: Master 服务器地址，格式为 `http://ip:port` 或 `https://domain:port`
- `<SECRET>`: 32 字符的随机密钥，由 Master 面板自动生成
