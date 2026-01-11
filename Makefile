# Makefile for Bomberman

.PHONY: gen clean lint format help install-tools build local server client clients

# 默认配置
PROTO ?= tcp
ADDR ?= localhost:8080
CHARACTER ?= 0
CONTROL ?= wasd

# 默认目标
help:
	@echo "========================================"
	@echo "  Bomberman - 炸弹人联机游戏"
	@echo "========================================"
	@echo ""
	@echo "常用命令:"
	@echo "  make build       - 编译服务器和客户端可执行文件"
	@echo "  make local       - 启动单机版游戏"
	@echo "  make server      - 启动联机服务器 (TCP + AI)"
	@echo "  make client      - 启动联机客户端（带大厅）"
	@echo "  make clients     - 启动两个联机客户端（测试用）"
	@echo ""
	@echo "参数配置:"
	@echo "  PROTO            网络协议 (tcp/kcp)，默认: tcp"
	@echo "  ADDR             服务器地址，默认: localhost:8080"
	@echo "  CHARACTER        角色类型 (0-3)，默认: 0"
	@echo "  CONTROL          控制方案 (wasd/arrow)，默认: wasd"
	@echo ""
	@echo "示例:"
	@echo "  make server PROTO=kcp                - 启动 KCP 服务器"
	@echo "  make client ADDR=localhost:9000      - 连接到指定服务器"
	@echo "  make client CHARACTER=1 CONTROL=arrow - 使用黑色角色+方向键"
	@echo ""
	@echo "开发工具:"
	@echo "  make gen         - 生成 Protobuf 代码"
	@echo "  make clean       - 清理生成的文件"
	@echo "  make install-tools - 安装开发工具"
	@echo ""
	@echo "更多帮助: make help-dev"

# ========== 编译 ==========

# 编译所有可执行文件
build: build-server build-client
	@echo "✓ 编译完成"
	@echo "  服务器: bin/server"
	@echo "  客户端: bin/client"

# 编译服务器
build-server:
	@echo "编译服务器..."
	@mkdir -p bin
	go build -o bin/server cmd/server/main.go
	@echo "✓ 服务器编译完成: bin/server"

# 编译客户端
build-client:
	@echo "编译客户端..."
	@mkdir -p bin
	go build -o bin/client cmd/client/main.go
	@echo "✓ 客户端编译完成: bin/client"

# ========== 游戏运行 ==========

# 启动单机版游戏
local:
	@echo "启动单机版游戏..."
	go run cmd/client/main.go

# 启动联机服务器（默认 TCP + AI）
server:
	@echo "启动联机服务器..."
	@echo "  协议: $(PROTO)"
	@echo "  地址: $(ADDR)"
	@echo "  AI: 已启用"
	go run cmd/server/main.go -addr=$(ADDR) -proto=$(PROTO) -enable-ai

# 启动联机客户端（默认大厅模式）
client:
	@echo "启动联机客户端（大厅模式）..."
	@echo "  服务器: $(ADDR)"
	@echo "  协议: $(PROTO)"
	@echo "  角色: $(CHARACTER)"
	@echo "  控制: $(CONTROL)"
	go run cmd/client/main.go -server=$(ADDR) -proto=$(PROTO) -character=$(CHARACTER) -control=$(CONTROL)

# 启动两个联机客户端（测试用）
clients:
	@echo "启动两个联机客户端..."
	@echo "  服务器: $(ADDR)"
	@echo "  协议: $(PROTO)"
	@go run cmd/client/main.go -server=$(ADDR) -proto=$(PROTO) -character=0 -control=wasd & \
	go run cmd/client/main.go -server=$(ADDR) -proto=$(PROTO) -character=1 -control=arrow & \
	wait

# ========== 开发工具 ==========

# 生成代码
gen:
	@echo "生成 Protobuf 代码..."
	cd api && buf generate
	@echo "✓ 代码生成完成"

# 清理生成的文件
clean:
	@echo "清理生成的文件..."
	rm -rf api/gen/bomberman/**/*.go
	@echo "✓ 清理完成"

# 代码检查
lint:
	@echo "检查 Protobuf 文件..."
	cd api && buf lint
	@echo "✓ 检查完成"

# 格式化代码
format:
	@echo "格式化 Protobuf 文件..."
	cd api && buf format -w
	@echo "✓ 格式化完成"

# 破坏性变更检查
check:
	@echo "检查破坏性变更..."
	cd api && buf breaking --against '.git#branch=main'
	@echo "✓ 检查完成"

# 安装工具
install-tools:
	@echo "安装开发工具..."
	go install github.com/bufbuild/buf/cmd/buf@latest
	@echo "✓ 工具安装完成"

# 开发帮助
help-dev:
	@echo "========================================"
	@echo "  开发命令详细说明"
	@echo "========================================"
	@echo ""
	@echo "编译:"
	@echo "  make build       - 编译服务器和客户端"
	@echo "  make build-server - 仅编译服务器"
	@echo "  make build-client  - 仅编译客户端"
	@echo ""
	@echo "Protobuf 操作:"
	@echo "  make gen         - 生成 Protobuf 代码"
	@echo "  make clean       - 清理生成的文件"
	@echo "  make lint        - 检查 Protobuf 文件"
	@echo "  make format      - 格式化 Protobuf 文件"
	@echo "  make check       - 检查破坏性变更"
	@echo "  make install-tools - 安装 Buf CLI"
	@echo ""
	@echo "游戏运行（直接使用 go run）:"
	@echo "  单机版：go run cmd/client/main.go"
	@echo "  服务器：go run cmd/server/main.go -addr=:8080 -proto=tcp -enable-ai"
	@echo "  客户端：go run cmd/client/main.go -server=localhost:8080"
	@echo ""
	@echo "服务器参数:"
	@echo "  -addr          监听地址，默认 :8080"
	@echo "  -proto         协议类型 (tcp/kcp)，默认 tcp"
	@echo "  -enable-ai     是否启用 AI 填充空位，默认 false"
	@echo ""
	@echo "客户端参数:"
	@echo "  -server        服务器地址（留空=单机模式）"
	@echo "  -proto         协议类型 (tcp/kcp)，默认 tcp"
	@echo "  -character     角色类型 (0=白, 1=黑, 2=红, 3=蓝)，默认 0"
	@echo "  -control       控制方案 (wasd/arrow)，默认 wasd"
	@echo "  -quick         跳过大厅直接加入默认房间，默认 false"
	@echo ""
	@echo "测试:"
	@echo "  go test ./pkg/core/..."
	@echo "  go test ./pkg/protocol/..."

# 一次性完整工作流
all: clean gen lint
	@echo "✓ 所有操作完成"
