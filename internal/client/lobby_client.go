package client

import (
	"fmt"
	"image/color"
	"time"

	gamev1 "bomberman/api/gen/bomberman/v1"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/basicfont"
)

var lobbyFont = text.NewGoXFace(basicfont.Face7x13)

type lobbyScreen int

const (
	screenLobby lobbyScreen = iota
	screenRoom
	screenGame
)

type joinResult struct {
	resp *gamev1.JoinResponse
	err  error
}

type keyTracker struct {
	prev map[ebiten.Key]bool
}

func (k *keyTracker) JustPressed(key ebiten.Key) bool {
	if k.prev == nil {
		k.prev = make(map[ebiten.Key]bool)
	}
	now := ebiten.IsKeyPressed(key)
	prev := k.prev[key]
	k.prev[key] = now
	return now && !prev
}

// LobbyClient handles lobby/room/game transitions.
type LobbyClient struct {
	network        *NetworkClient
	controlScheme  ControlScheme
	screen         lobbyScreen
	roomList       []*gamev1.RoomInfo
	roomState      *gamev1.RoomStateUpdate
	selectedIndex  int
	lastListFetch  time.Time
	lastError      string
	joinInFlight   bool
	joinResultChan chan joinResult
	input          keyTracker

	game *NetworkGameClient
}

func NewLobbyClient(network *NetworkClient, controlScheme ControlScheme) *LobbyClient {
	return &LobbyClient{
		network:        network,
		controlScheme:  controlScheme,
		screen:         screenLobby,
		joinResultChan: make(chan joinResult, 1),
	}
}

func (lc *LobbyClient) Update() error {
	switch lc.screen {
	case screenLobby:
		lc.updateLobby()
	case screenRoom:
		lc.updateRoom()
	case screenGame:
		lc.updateGame()
	}
	return nil
}

func (lc *LobbyClient) Draw(screen *ebiten.Image) {
	switch lc.screen {
	case screenLobby:
		lc.drawLobby(screen)
	case screenRoom:
		lc.drawRoom(screen)
	case screenGame:
		if lc.game != nil {
			lc.game.Draw(screen)
		}
	}
}

func (lc *LobbyClient) Layout(outsideWidth, outsideHeight int) (int, int) {
	return ScreenWidth, ScreenHeight
}

func (lc *LobbyClient) updateLobby() {
	if time.Since(lc.lastListFetch) > time.Second {
		_ = lc.network.RequestRoomList(1, 20)
		lc.lastListFetch = time.Now()
	}

	for {
		resp := lc.network.ReceiveRoomList()
		if resp == nil {
			break
		}
		lc.roomList = resp.Rooms
		if lc.selectedIndex >= len(lc.roomList) {
			lc.selectedIndex = 0
		}
	}

	select {
	case res := <-lc.joinResultChan:
		lc.joinInFlight = false
		if res.err != nil {
			lc.lastError = res.err.Error()
			break
		}
		lc.lastError = ""
		if res.resp != nil {
			lc.roomState = res.resp.RoomState
			lc.screen = screenRoom
		}
	default:
	}

	if lc.input.JustPressed(ebiten.KeyR) {
		_ = lc.network.RequestRoomList(1, 20)
		lc.lastListFetch = time.Now()
	}

	if lc.input.JustPressed(ebiten.KeyQ) {
		lc.startJoin("")
	}
	if lc.input.JustPressed(ebiten.KeyC) {
		lc.startJoin("CREATE")
	}
	if lc.input.JustPressed(ebiten.KeyArrowUp) || lc.input.JustPressed(ebiten.KeyW) {
		if lc.selectedIndex > 0 {
			lc.selectedIndex--
		}
	}
	if lc.input.JustPressed(ebiten.KeyArrowDown) || lc.input.JustPressed(ebiten.KeyS) {
		if lc.selectedIndex < len(lc.roomList)-1 {
			lc.selectedIndex++
		}
	}
	if lc.input.JustPressed(ebiten.KeyEnter) && lc.selectedIndex >= 0 && lc.selectedIndex < len(lc.roomList) {
		room := lc.roomList[lc.selectedIndex]
		if room != nil && room.Status == gamev1.RoomStatus_ROOM_STATUS_WAITING {
			lc.startJoin(room.Id)
		}
	}
}

func (lc *LobbyClient) updateRoom() {
	for {
		update := lc.network.ReceiveRoomState()
		if update == nil {
			break
		}
		lc.roomState = update
	}

	for {
		resp := lc.network.ReceiveRoomActionResponse()
		if resp == nil {
			break
		}
		if !resp.Success {
			lc.lastError = resp.ErrorMessage
			continue
		}
		lc.lastError = ""
		if resp.RoomId == "" {
			lc.roomState = nil
			lc.screen = screenLobby
			lc.lastListFetch = time.Time{}
		}
	}

	for {
		event := lc.network.ReceiveEvent()
		if event == nil {
			break
		}
		if _, ok := event.Event.(*gamev1.GameEvent_GameStart); ok {
			gameClient, err := NewNetworkGameClient(lc.network, lc.controlScheme)
			if err == nil {
				lc.game = gameClient
				lc.screen = screenGame
			} else {
				lc.lastError = err.Error()
			}
		}
	}

	if lc.input.JustPressed(ebiten.KeySpace) {
		lc.toggleReady()
	}
	if lc.input.JustPressed(ebiten.KeyEnter) {
		lc.startGame()
	}
	if lc.input.JustPressed(ebiten.KeyA) {
		lc.addAI(1)
	}
	if lc.input.JustPressed(ebiten.KeyL) || lc.input.JustPressed(ebiten.KeyEscape) {
		_ = lc.network.LeaveRoom()
	}
}

func (lc *LobbyClient) updateGame() {
	if lc.game == nil {
		lc.screen = screenRoom
		return
	}
	_ = lc.game.Update()
	if lc.game.game.gameOver {
		lc.game = nil
		lc.screen = screenRoom
	}
}

func (lc *LobbyClient) startJoin(roomID string) {
	if lc.joinInFlight {
		return
	}
	lc.joinInFlight = true
	lc.lastError = ""
	go func() {
		resp, err := lc.network.JoinRoom(roomID)
		select {
		case lc.joinResultChan <- joinResult{resp: resp, err: err}:
		default:
		}
	}()
}

func (lc *LobbyClient) toggleReady() {
	if lc.roomState == nil {
		return
	}
	playerID := lc.network.GetPlayerID()
	ready := false
	for _, player := range lc.roomState.Players {
		if player.Id == playerID {
			ready = player.IsReady
			break
		}
	}
	action := &gamev1.RoomAction{
		Type:  gamev1.RoomActionType_ROOM_ACTION_READY,
		Ready: !ready,
	}
	_ = lc.network.SendRoomAction(action)
}

func (lc *LobbyClient) startGame() {
	if lc.roomState == nil {
		return
	}
	if lc.roomState.HostId != lc.network.GetPlayerID() {
		return
	}
	action := &gamev1.RoomAction{
		Type: gamev1.RoomActionType_ROOM_ACTION_START,
	}
	_ = lc.network.SendRoomAction(action)
}

func (lc *LobbyClient) addAI(count int32) {
	if lc.roomState == nil {
		return
	}
	if lc.roomState.HostId != lc.network.GetPlayerID() {
		return
	}
	action := &gamev1.RoomAction{
		Type:    gamev1.RoomActionType_ROOM_ACTION_ADD_AI,
		AiCount: count,
	}
	_ = lc.network.SendRoomAction(action)
}

func (lc *LobbyClient) drawLobby(screen *ebiten.Image) {
	screen.Fill(color.RGBA{18, 22, 30, 255})
	drawText(screen, 16, 24, "Lobby", color.White)
	drawText(screen, 16, 44, "Q: Quick Match  C: Create  R: Refresh  Enter: Join  W/S: Select", color.RGBA{180, 190, 200, 255})

	y := 70
	for i, room := range lc.roomList {
		if room == nil {
			continue
		}
		prefix := " "
		col := color.RGBA{210, 220, 230, 255}
		if i == lc.selectedIndex {
			prefix = ">"
			col = color.RGBA{255, 220, 120, 255}
		}
		status := roomStatusLabel(room.Status)
		line := fmt.Sprintf("%s [%d] %s  %d/%d  AI:%d  %s", prefix, i+1, room.Name, room.CurrentPlayers, room.MaxPlayers, room.AiCount, status)
		drawText(screen, 16, y, line, col)
		y += 16
	}

	if lc.joinInFlight {
		drawText(screen, 16, ScreenHeight-24, "Joining...", color.RGBA{200, 200, 120, 255})
	}
	if lc.lastError != "" {
		drawText(screen, 16, ScreenHeight-8, lc.lastError, color.RGBA{255, 120, 120, 255})
	}
}

func (lc *LobbyClient) drawRoom(screen *ebiten.Image) {
	screen.Fill(color.RGBA{16, 18, 24, 255})
	roomID := ""
	if lc.roomState != nil {
		roomID = lc.roomState.RoomId
	}
	drawText(screen, 16, 24, fmt.Sprintf("Room: %s", roomID), color.White)
	drawText(screen, 16, 44, "Space: Ready  Enter: Start  A: Add AI  L: Leave", color.RGBA{180, 190, 200, 255})

	y := 70
	if lc.roomState != nil {
		for _, player := range lc.roomState.Players {
			if player == nil {
				continue
			}
			flags := ""
			if player.IsHost {
				flags += "H"
			}
			if player.IsAi {
				flags += "A"
			}
			if player.IsReady {
				flags += "R"
			}
			line := fmt.Sprintf("[%s] %s (%s)", flags, player.Name, shortCharacter(player.Character))
			drawText(screen, 16, y, line, color.RGBA{220, 230, 240, 255})
			y += 16
		}
	}

	if lc.lastError != "" {
		drawText(screen, 16, ScreenHeight-8, lc.lastError, color.RGBA{255, 120, 120, 255})
	}
}

func drawText(screen *ebiten.Image, x, y int, msg string, clr color.Color) {
	options := &text.DrawOptions{}
	options.GeoM.Translate(float64(x), float64(y))
	options.ColorScale.ScaleWithColor(clr)
	text.Draw(screen, msg, lobbyFont, options)
}

func roomStatusLabel(status gamev1.RoomStatus) string {
	switch status {
	case gamev1.RoomStatus_ROOM_STATUS_WAITING:
		return "WAITING"
	case gamev1.RoomStatus_ROOM_STATUS_PLAYING:
		return "PLAYING"
	default:
		return "UNKNOWN"
	}
}

func shortCharacter(char gamev1.CharacterType) string {
	switch char {
	case gamev1.CharacterType_CHARACTER_TYPE_WHITE:
		return "WHITE"
	case gamev1.CharacterType_CHARACTER_TYPE_BLACK:
		return "BLACK"
	case gamev1.CharacterType_CHARACTER_TYPE_RED:
		return "RED"
	case gamev1.CharacterType_CHARACTER_TYPE_BLUE:
		return "BLUE"
	default:
		return "UNKNOWN"
	}
}
