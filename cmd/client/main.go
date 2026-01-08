package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hajimehoshi/ebiten/v2"

	client "bomberman/internal/client"
	"bomberman/pkg/core"
)

func main() {
	// 命令行参数
	serverAddr := flag.String("server", "", "服务器地址（留空为单机模式）")
	proto := flag.String("proto", "tcp", "服务器协议: tcp 或 kcp")
	character := flag.Int("character", 0, "角色类型 (0=白, 1=黑, 2=红, 3=蓝)")
	control := flag.String("control", "wasd", "控制方案 (wasd 或 arrow)")
	flag.Parse()

	// 解析角色类型
	charType := core.CharacterType(*character)
	if charType < core.CharacterWhite || charType > core.CharacterBlue {
		log.Fatalf("无效的角色类型: %d", *character)
	}

	// 解析控制方案
	var controlScheme client.ControlScheme
	switch *control {
	case "wasd":
		controlScheme = client.ControlWASD
	case "arrow":
		controlScheme = client.ControlArrow
	default:
		log.Fatalf("无效的控制方案: %s (使用 'wasd' 或 'arrow')", *control)
	}

	// 设置窗口选项
	ebiten.SetWindowSize(client.ScreenWidth, client.ScreenHeight)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	ebiten.SetTPS(client.FPS)

	var game ebiten.Game
	var title string
	var networkClient *client.NetworkClient

	if *serverAddr == "" {
		// ========== 单机模式 ==========
		log.Println("========================================")
		log.Println("  Bomberman - 单机模式")
		log.Println("========================================")
		log.Printf("角色: %s", charType)
		log.Printf("控制: %s", controlScheme)
		log.Println("========================================")

		// 创建单机游戏
		game = createLocalGame(charType, controlScheme)
		title = "Bomberman - 单机模式 [" + charType.String() + "] [" + controlScheme.String() + "]"
	} else {
		// ========== 联机模式 ==========
		log.Println("========================================")
		log.Println("  Bomberman - 联机模式")
		log.Println("========================================")
		log.Printf("协议: %s", *proto)
		log.Printf("服务器: %s", *serverAddr)
		log.Printf("角色: %s", charType)
		log.Printf("控制: %s", controlScheme)
		log.Println("========================================")

		// 创建联机游戏
		networkClient = client.NewNetworkClient(*serverAddr, *proto, charType)

		if err := networkClient.Connect(); err != nil {
			log.Fatalf("连接服务器失败: %v", err)
		}
		defer networkClient.Close()
		setupSignalHandler(networkClient)

		var err error
		game, err = client.NewNetworkGameClient(networkClient, controlScheme)
		if err != nil {
			log.Fatalf("创建联机游戏失败: %v", err)
		}

		title = "Bomberman - 联机模式 [" + *proto + "] [" + *serverAddr + "] [" + charType.String() + "] [" + controlScheme.String() + "]"
	}

	ebiten.SetWindowTitle(title)

	// 运行游戏
	log.Println("游戏启动！")
	if err := ebiten.RunGame(game); err != nil {
		if networkClient != nil {
			networkClient.Close()
		}
		log.Fatalf("游戏运行错误: %v", err)
	}
}

func setupSignalHandler(networkClient *client.NetworkClient) {
	if networkClient == nil {
		return
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		networkClient.Close()
		os.Exit(0)
	}()
}

// createLocalGame 创建单机游戏
func createLocalGame(character core.CharacterType, controlScheme client.ControlScheme) *client.Game {
	game := client.NewGame()
	game.SetControlScheme(controlScheme)

	// 创建本地玩家
	x, y := client.GridToPlayerXY(0, 0)
	player := client.NewPlayer(game, 1, x, y, character, false)
	game.AddPlayer(player)

	// 添加 AI 玩家（可选）
	addAIPlayers(game, 3)

	return game
}

// addAIPlayers 添加 AI 玩家（用于测试）
func addAIPlayers(game *client.Game, count int) {
	spawns := []struct{ x, y int }{
		{core.MapWidth - 1, 0},                  // 右上角
		{0, core.MapHeight - 1},                 // 左下角
		{core.MapWidth - 1, core.MapHeight - 1}, // 右下角
	}

	chars := []core.CharacterType{
		core.CharacterWhite,
		core.CharacterBlack,
		core.CharacterBlue,
	}

	for i := 0; i < count && i < len(spawns); i++ {
		x, y := client.GridToPlayerXY(spawns[i].x, spawns[i].y)
		aiPlayer := client.NewPlayer(game, i+2, x, y, chars[i%len(chars)], true)
		game.AddPlayer(aiPlayer)
	}
}
