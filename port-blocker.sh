#!/bin/bash

# macOS 端口屏蔽脚本
# 使用方法: sudo ./port_blocker.sh <up|down> <port>
# 示例: sudo ./port_blocker.sh up 8080
#       sudo ./port_blocker.sh down 8080

set -e  # 遇到错误退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 配置
RULE_FILE="/tmp/block_port_rule.pf"
ANCHOR_NAME="block_port_anchor"
TEMP_MAIN_CONF="/tmp/pf.conf.blocker"

# 显示用法
show_usage() {
    echo "用法: $0 <up|down> <端口号>"
    echo "  up    - 屏蔽指定端口 (仅 TCP)"
    echo "  down  - 取消屏蔽指定端口"
    echo ""
    echo "示例:"
    echo "  sudo $0 up 8080      # 屏蔽 8080 端口"
    echo "  sudo $0 down 8080    # 取消屏蔽 8080 端口"
    exit 1
}

# 检查是否以 root 运行
check_root() {
    if [[ $EUID -ne 0 ]]; then
        echo -e "${RED}错误: 此脚本需要以 root 权限运行${NC}"
        echo -e "请使用: sudo $0 $@"
        exit 1
    fi
}

# 检查参数
check_args() {
    if [[ $# -ne 2 ]]; then
        show_usage
    fi
    
    ACTION="$1"
    PORT="$2"
    
    # 验证端口号
    if ! [[ "$PORT" =~ ^[0-9]+$ ]] || [[ "$PORT" -lt 1 ]] || [[ "$PORT" -gt 65535 ]]; then
        echo -e "${RED}错误: 端口号必须为 1-65535 之间的数字${NC}"
        exit 1
    fi
    
    # 验证动作
    if [[ "$ACTION" != "up" && "$ACTION" != "down" ]]; then
        echo -e "${RED}错误: 动作必须是 'up' 或 'down'${NC}"
        show_usage
    fi
}

# 获取端口对应的服务名
get_service_name() {
    local port="$1"
    # 从 /etc/services 查找 TCP 端口对应的服务名
    # 匹配格式: name  port/tcp
    if [ -f "/etc/services" ]; then
        grep -E "^[a-zA-Z0-9_-]+\s+$port/tcp" /etc/services | head -n 1 | awk '{print $1}'
    fi
}

# 创建规则文件
create_rule_file() {
    local port="$1"
    cat > "$RULE_FILE" << EOF
# 自动生成的端口屏蔽规则
block drop in proto tcp from any to any port $port
EOF
    echo -e "${GREEN}✓ 已创建规则文件: $RULE_FILE${NC}"
}

# 启用端口屏蔽
enable_port_block() {
    local port="$1"
    
    echo -e "${YELLOW}正在屏蔽端口 $port (TCP) ...${NC}"
    
    # 创建规则文件
    create_rule_file "$port"
    
    # 检查 pf 是否启用，如果未启用则启用
    if ! pfctl -s info 2>/dev/null | grep -q "Status: Enabled"; then
        echo -e "${YELLOW}启用 pf 防火墙...${NC}"
        pfctl -e 2>/dev/null || {
            echo -e "${RED}✗ 无法启用 pf 防火墙${NC}"
            return 1
        }
    fi
    
    # --- 关键修复: 将锚点注入到主配置 ---
    echo -e "${YELLOW}配置防火墙锚点...${NC}"
    
    # 复制系统默认配置
    if [ -f "/etc/pf.conf" ]; then
        cat /etc/pf.conf > "$TEMP_MAIN_CONF"
    else
        touch "$TEMP_MAIN_CONF"
    fi
    
    # 追加锚点引用 (如果尚未存在)
    if ! grep -q "anchor \"$ANCHOR_NAME\"" "$TEMP_MAIN_CONF"; then
        echo "" >> "$TEMP_MAIN_CONF"
        echo "# Added by port-blocker.sh" >> "$TEMP_MAIN_CONF"
        echo "anchor \"$ANCHOR_NAME\"" >> "$TEMP_MAIN_CONF"
    fi
    
    # 加载包含锚点引用的主配置
    pfctl -f "$TEMP_MAIN_CONF" 2>/dev/null || {
        echo -e "${RED}✗ 无法更新主防火墙配置 (锚点注入失败)${NC}"
        return 1
    }
    # ----------------------------------
    
    # 将规则加载到锚点
    echo -e "${YELLOW}加载屏蔽规则...${NC}"
    pfctl -a "$ANCHOR_NAME" -f "$RULE_FILE" 2>/dev/null || {
        echo -e "${RED}✗ 无法加载规则${NC}"
        return 1
    }
    
    # 验证规则是否加载成功
    # 这里我们只检查锚点内是否有内容，不纠结具体的 grep 匹配，详细匹配交给 show_status
    if ! pfctl -a "$ANCHOR_NAME" -s rules 2>/dev/null | grep -q "block drop"; then
        echo -e "${RED}✗ 规则加载失败${NC}"
        return 1
    fi
    
    echo -e "${GREEN}✓ 端口 $port 已被屏蔽${NC}"
    
    # 显示当前状态
    show_status "$port"
}

# 禁用端口屏蔽
disable_port_block() {
    local port="$1"
    
    echo -e "${YELLOW}正在取消屏蔽端口 $port ...${NC}"
    
    # 清除锚点规则
    pfctl -a "$ANCHOR_NAME" -F rules 2>/dev/null
    
    # 恢复系统默认配置 (移除我们的锚点引用)
    if [ -f "/etc/pf.conf" ]; then
        echo -e "${YELLOW}恢复系统默认配置...${NC}"
        pfctl -f /etc/pf.conf 2>/dev/null
    fi
    
    # 删除临时文件
    rm -f "$RULE_FILE" "$TEMP_MAIN_CONF"
    
    echo -e "${GREEN}✓ 端口 $port 已恢复 (系统配置已重置)${NC}"
    
    # 显示状态
    echo -e "\n当前屏蔽状态:"
    
    # 验证是否真的恢复了 (稍微复用一下 show_status 的逻辑)
    local service_name=$(get_service_name "$port")
    local grep_pattern="port (= )?$port"
    if [[ -n "$service_name" ]]; then
        grep_pattern="port (= )?($port|$service_name)"
    fi
    
    if pfctl -a "$ANCHOR_NAME" -s rules 2>/dev/null | grep -E -q "$grep_pattern"; then
         echo -e "端口 $port: ${RED}仍被屏蔽 (异常)${NC}"
    else
         echo -e "端口 $port: ${GREEN}未屏蔽${NC}"
    fi
}

# 显示状态
show_status() {
    local port="$1"
    echo -e "\n${YELLOW}=== 当前状态 ===${NC}"
    
    # 检查 pf 状态
    if pfctl -s info 2>/dev/null | grep -q "Status: Enabled"; then
        echo -e "PF 防火墙: ${GREEN}已启用${NC}"
    else
        echo -e "PF 防火墙: ${RED}未启用${NC}"
    fi
    
    # 获取服务名用于匹配 (例如 8080 -> http-alt)
    local service_name=$(get_service_name "$port")
    local display_info="端口 $port"
    if [[ -n "$service_name" ]]; then
        display_info="端口 $port ($service_name)"
    fi
    
    echo -e "\n检查 $display_info 的规则:"
    
    # 构建 grep 模式: 匹配 "port 8080", "port = 8080", "port http-alt", "port = http-alt"
    local grep_pattern="port (= )?$port"
    if [[ -n "$service_name" ]]; then
        grep_pattern="port (= )?($port|$service_name)"
    fi
    
    # 检查规则
    if pfctl -a "$ANCHOR_NAME" -s rules 2>/dev/null | grep -E -q "$grep_pattern"; then
        echo -e "$display_info: ${RED}已屏蔽${NC}"
        echo "生效规则:"
        pfctl -a "$ANCHOR_NAME" -s rules 2>/dev/null | grep -E "$grep_pattern"
    else
        echo -e "$display_info: ${GREEN}未屏蔽${NC}"
        # 调试信息: 如果锚点里有东西但没匹配上，显示出来
        if pfctl -a "$ANCHOR_NAME" -s rules 2>/dev/null | grep -q "block"; then
            echo "提示: 锚点中存在其他规则:"
            pfctl -a "$ANCHOR_NAME" -s rules 2>/dev/null
        fi
    fi
    
    echo -e "\n${YELLOW}=== 测试连接 ===${NC}"
    echo "在本机测试: nc -zv localhost $port"
    echo "在其他机器测试: nc -zv <本机IP> $port"
}

# 主函数
main() {
    check_args "$@"
    check_root "$@"
    
    case "$ACTION" in
        "up")
            enable_port_block "$PORT"
            ;;
        "down")
            disable_port_block "$PORT"
            ;;
    esac
}

# 运行主函数
main "$@"
