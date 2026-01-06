# Makefile for Protocol Buffers operations

.PHONY: gen clean lint format help install-tools

# 默认目标
help:
	@echo "Available targets:"
	@echo "  make gen         - Generate protobuf code"
	@echo "  make clean       - Remove generated files"
	@echo "  make lint        - Lint protobuf files"
	@echo "  make format      - Format protobuf files"
	@echo "  make check       - Run breaking change detection"
	@echo "  make install-tools- Install required tools"

# 生成代码
gen:
	@echo "Generating protobuf code..."
	cd api && buf generate
	@echo "✓ Code generation complete"

# 清理生成的文件
clean:
	@echo "Cleaning generated files..."
	rm -rf api/gen/bomberman/**/*.go
	@echo "✓ Clean complete"

# 代码检查
lint:
	@echo "Linting protobuf files..."
	cd api && buf lint
	@echo "✓ Lint complete"

# 格式化代码
format:
	@echo "Formatting protobuf files..."
	cd api && buf format -w
	@echo "✓ Format complete"

# 破坏性变更检查 (需要先创建一个 .git 之前的版本对比)
check:
	@echo "Checking for breaking changes..."
	cd api && buf breaking --against '.git#branch=main'
	@echo "✓ Breaking change check complete"

# 安装工具
install-tools:
	@echo "Installing Buf CLI..."
	go install github.com/bufbuild/buf/cmd/buf@latest
	@echo "✓ Tools installed"

# 一次性完整工作流
all: clean gen lint
	@echo "✓ All operations complete"
