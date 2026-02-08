#!/bin/bash

# Docker Slave 管理脚本

set -e

COMPOSE_FILE="docker-compose.slaves.yml"

# 检测宿主机 IP
# 支持通过环境变量 MASTER_IP 指定 Master IP
# Linux 使用实际 IP，Mac/Windows 使用 host.docker.internal
detect_host_ip() {
    # 优先使用环境变量
    if [ ! -z "$MASTER_IP" ]; then
        echo "$MASTER_IP"
        return
    fi
    
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        # Linux 系统，获取宿主机 IP
        # 尝试多种方法获取宿主机 IP
        HOST_IP=$(ip route show default | awk '/default/ {print $3}' | head -n1)
        if [ -z "$HOST_IP" ]; then
            HOST_IP=$(hostname -I | awk '{print $1}')
        fi
        if [ -z "$HOST_IP" ]; then
            HOST_IP="172.17.0.1"  # Docker 默认网关
        fi
        echo "$HOST_IP"
    else
        # Mac/Windows 使用 host.docker.internal
        echo "host.docker.internal"
    fi
}

MASTER_HOST=$(detect_host_ip)
MASTER_URL="ws://${MASTER_HOST}:2053/panel/api/slave/connect"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 构建 Slave 镜像
build() {
    print_info "Building 3x-ui binary (static compilation for Alpine)..."
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o 3x-ui main.go
    if [ ! -f "3x-ui" ]; then
        print_error "Failed to build 3x-ui binary"
        exit 1
    fi
    print_info "Binary built successfully"
    
    print_info "Building slave Docker image..."
    docker build -f Dockerfile.slave -t 3x-ui-slave:latest .
    print_info "Build completed!"
}

# 启动指定的 slave
start_slave() {
    local slave_name=$1
    local secret=$2
    
    if [ -z "$slave_name" ] || [ -z "$secret" ]; then
        print_error "Usage: $0 start <slave_name> <secret>"
        echo "Example: $0 start slave1 abc123xyz"
        exit 1
    fi
    
    print_info "Starting $slave_name with secret: ${secret:0:8}..."
    print_info "Master URL: ${MASTER_URL}"
    
    docker run -d \
        --name "3x-ui-${slave_name}" \
        --hostname "${slave_name}" \
        --restart unless-stopped \
        -v "3x-ui-${slave_name}-data:/app/db" \
        -v "3x-ui-${slave_name}-logs:/app/log" \
        3x-ui-slave:latest \
        "${MASTER_URL}" "${secret}"
    
    print_info "${slave_name} started successfully!"
}

# 使用 docker-compose 启动所有 slaves
start_all() {
    print_warn "Before starting, update secrets in ${COMPOSE_FILE}!"
    print_warn "Replace REPLACE_WITH_SECRET_X with actual secrets from Master panel."
    echo ""
    read -p "Have you updated the secrets? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_error "Please update secrets first!"
        exit 1
    fi
    
    print_info "Starting all slaves..."
    docker compose -f "${COMPOSE_FILE}" up -d
    print_info "All slaves started!"
}

# 停止指定的 slave
stop_slave() {
    local slave_name=$1
    
    if [ -z "$slave_name" ]; then
        print_error "Usage: $0 stop <slave_name>"
        echo "Example: $0 stop slave1"
        exit 1
    fi
    
    print_info "Stopping ${slave_name}..."
    docker stop "3x-ui-${slave_name}" 2>/dev/null || true
    print_info "${slave_name} stopped!"
}

# 停止所有 slaves
stop_all() {
    print_info "Stopping all slaves..."
    docker compose -f "${COMPOSE_FILE}" down
    print_info "All slaves stopped!"
}

# 查看日志
logs() {
    local slave_name=${1:-""}
    
    if [ -z "$slave_name" ]; then
        print_info "Showing logs for all slaves..."
        docker compose -f "${COMPOSE_FILE}" logs -f
    else
        print_info "Showing logs for ${slave_name}..."
        docker logs -f "3x-ui-${slave_name}"
    fi
}

# 查看状态
status() {
    print_info "Slave Status:"
    docker ps -a --filter "name=3x-ui-slave" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
}

# 进入容器 shell
shell() {
    local slave_name=$1
    
    if [ -z "$slave_name" ]; then
        print_error "Usage: $0 shell <slave_name>"
        echo "Example: $0 shell slave1"
        exit 1
    fi
    
    print_info "Entering ${slave_name} shell..."
    docker exec -it "3x-ui-${slave_name}" /bin/sh
}

# 清理所有资源
cleanup() {
    print_warn "This will remove all slave containers and volumes!"
    read -p "Are you sure? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_info "Cleanup cancelled."
        exit 0
    fi
    
    print_info "Stopping and removing all slaves..."
    docker compose -f "${COMPOSE_FILE}" down -v
    
    print_info "Removing standalone containers..."
    docker ps -a --filter "name=3x-ui-slave" -q | xargs -r docker rm -f
    
    print_info "Removing volumes..."
    docker volume ls --filter "name=3x-ui-slave" -q | xargs -r docker volume rm
    
    print_info "Cleanup completed!"
}

# 重启指定的 slave
restart_slave() {
    local slave_name=$1
    
    if [ -z "$slave_name" ]; then
        print_error "Usage: $0 restart <slave_name>"
        echo "Example: $0 restart slave1"
        exit 1
    fi
    
    print_info "Restarting ${slave_name}..."
    docker restart "3x-ui-${slave_name}"
    print_info "${slave_name} restarted!"
}

# 显示帮助信息
show_help() {
    cat << EOF
3x-ui Docker Slave Manager

Usage: $0 <command> [options]

Commands:
  build                    Build the slave Docker image
  start <name> <secret>    Start a single slave container
  start-all                Start all slaves using docker-compose
  stop <name>              Stop a specific slave
  stop-all                 Stop all slaves
  restart <name>           Restart a specific slave
  logs [name]              Show logs (all slaves if name not specified)
  status                   Show status of all slaves
  shell <name>             Enter slave container shell
  cleanup                  Remove all slaves and volumes

Examples:
  $0 build
  $0 start slave1 abc123xyz789
  $0 start-all
  $0 logs slave1
  $0 status
  $0 cleanup

Note:
  - For start-all, edit ${COMPOSE_FILE} and replace secrets first
  - Master URL is set to: ${MASTER_URL}
  - Use 'host.docker.internal' to access host machine from container

EOF
}

# 主程序
case "${1:-}" in
    build)
        build
        ;;
    start)
        if [ "$2" == "all" ]; then
            start_all
        else
            start_slave "$2" "$3"
        fi
        ;;
    start-all)
        start_all
        ;;
    stop)
        if [ "$2" == "all" ]; then
            stop_all
        else
            stop_slave "$2"
        fi
        ;;
    stop-all)
        stop_all
        ;;
    restart)
        restart_slave "$2"
        ;;
    logs)
        logs "$2"
        ;;
    status)
        status
        ;;
    shell)
        shell "$2"
        ;;
    cleanup)
        cleanup
        ;;
    *)
        show_help
        exit 1
        ;;
esac
