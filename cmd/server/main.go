package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"bomberman/internal/server"
)

func main() {
	// 命令行参数
	address := flag.String("addr", ":8080", "服务器监听地址")
	flag.Parse()

	// 创建服务器
	gameServer := server.NewGameServer(*address)

	// 启动服务器（在新的 goroutine 中）
	go func() {
		if err := gameServer.Start(); err != nil {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	log.Println("========================================")
	log.Println("  Bomberman 联机服务器")
	log.Println("========================================")
	log.Printf("监听地址: %s", *address)
	log.Printf("最大玩家数: %d", server.MaxPlayers)
	log.Printf("服务器 TPS: %d", server.ServerTPS)
	log.Println("========================================")
	log.Println("服务器正在运行...")
	log.Println("按 Ctrl+C 停止服务器")

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("\n正在关闭服务器...")
	gameServer.Shutdown()

	log.Println("服务器已关闭，再见！")
}
