package main

import (
	client "bomberman/internal/client"
	"github.com/hajimehoshi/ebiten/v2"
	"log"
)

func main() {
	game := client.NewGame()
	x, y := client.GridToPlayerXY(0, 0)
	player := client.NewPlayer(game, 1, x, y, client.CharacterRed, false)
	game.AddPlayer(player)
	// 设置窗口选项
	ebiten.SetWindowSize(client.ScreenWidth, client.ScreenHeight)
	ebiten.SetWindowTitle("Bomberman - 炸弹人 [" + player.Character().String() + "] [" + client.ControlWASD.String() + "]")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	ebiten.SetTPS(client.FPS)

	// 运行游戏
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
