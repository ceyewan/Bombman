# Makefile for Bomberman

.PHONY: gen clean lint format help install-tools local server client clients

# 默认目标
help:
	@echo "========================================"
	@echo "  Bomberman - 炸弹人联机游戏"
	@echo "========================================"
	@echo ""
	@echo "游戏运行:"
	@echo "  make local       - 启动单机版游戏"
	@echo "  make server      - 启动联机服务器"
	@echo "  make client      - 启动联机客户端"
	@echo "  make clients     - 启动两个联机客户端（WASD + 方向键）"
	@echo ""
	@echo "开发工具:"
	@echo "  make gen         - 生成 Protobuf 代码"
	@echo "  make clean       - 清理生成的文件"
	@echo "  make install-tools - 安装开发工具"
	@echo ""
	@echo "更多选项: make help-dev"

# ========== 游戏运行 ==========

# 启动单机版游戏
local:
	@echo "启动单机版游戏..."
	go run cmd/client/main.go

# 启动联机服务器
server:
	@echo "启动联机服务器..."
	go run cmd/server/main.go

# 启动联机客户端
client:
	@echo "启动联机客户端..."
	go run cmd/client/main.go -server=localhost:8080

# 启动两个联机客户端
clients:
	@echo "启动两个联机客户端..."
	@go run cmd/client/main.go -server=localhost:8080 -character=0 -control=wasd & \
	go run cmd/client/main.go -server=localhost:8080 -character=1 -control=arrow & \
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
	@echo "  开发命令"
	@echo "========================================"
	@echo ""
	@echo "Protobuf 操作:"
	@echo "  make gen         - 生成 Protobuf 代码"
	@echo "  make clean       - 清理生成的文件"
	@echo "  make lint        - 检查 Protobuf 文件"
	@echo "  make format      - 格式化 Protobuf 文件"
	@echo "  make check       - 检查破坏性变更"
	@echo "  make install-tools - 安装 Buf CLI"
	@echo ""
	@echo "游戏运行（自定义参数）:"
	@echo "  单机版：go run cmd/client/main.go -character=1 -control=arrow"
	@echo "  联机服务器：go run cmd/server/main.go -addr=:9000"
	@echo "  联机客户端：go run cmd/client/main.go -server=localhost:8080"
	@echo ""
	@echo "测试:"
	@echo "  go test ./pkg/core/..."
	@echo "  go test ./pkg/protocol/..."

# 一次性完整工作流
all: clean gen lint
	@echo "✓ 所有操作完成"
