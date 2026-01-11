package client

import (
	"fmt"
	"image/color"
	"time"

	gamev1 "bomberman/api/gen/bomberman/v1"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"
)

var lobbyFont = text.NewGoXFace(basicfont.Face7x13)

// UI Color Palette
var (
	uiBackground      = color.RGBA{12, 16, 24, 255}
	uiPanelBackground = color.RGBA{24, 28, 36, 255}
	uiPanelBorder     = color.RGBA{60, 70, 85, 255}
	uiTextPrimary     = color.RGBA{230, 235, 245, 255}
	uiTextSecondary   = color.RGBA{150, 160, 175, 255}
	uiTextMuted       = color.RGBA{100, 110, 125, 255}
	uiAccent          = color.RGBA{255, 200, 80, 255}
	uiAccentDim       = color.RGBA{180, 140, 55, 255}
	uiSuccess         = color.RGBA{80, 200, 120, 255}
	uiWarning         = color.RGBA{230, 180, 80, 255}
	uiError           = color.RGBA{230, 90, 90, 255}
	uiRoomWaiting     = color.RGBA{80, 180, 220, 255}
	uiRoomPlaying     = color.RGBA{220, 100, 100, 255}
	uiRoomFull        = color.RGBA{140, 140, 160, 255}
)

// UI Layout Constants
const (
	uiPanelMargin  = 12
	uiPanelPadding = 16
	uiTitleMargin  = 20
	uiRowHeight    = 20
	uiColumnGap    = 16
	uiBorderRadius = 4
	uiInputBorder  = 2
)

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
	// Room creation input state
	inputMode        bool
	inputBuffer      string
	inputCursorBlink float32
	// Toast notification
	toastMessage string
	toastTimer   float32

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
	// Update toast timer
	if lc.toastTimer > 0 {
		lc.toastTimer -= 1 / 60.0
		if lc.toastTimer <= 0 {
			lc.toastMessage = ""
		}
	}

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
	// Update cursor blink
	lc.inputCursorBlink += 0.05

	// Handle input mode for room creation
	if lc.inputMode {
		lc.handleInputMode()
		return
	}

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
		lc.inputMode = true
		lc.inputBuffer = ""
		lc.inputCursorBlink = 0
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

func (lc *LobbyClient) handleInputMode() {
	// Handle text input
	if ebiten.IsKeyPressed(ebiten.KeyBackspace) {
		if lc.input.JustPressed(ebiten.KeyBackspace) && len(lc.inputBuffer) > 0 {
			lc.inputBuffer = lc.inputBuffer[:len(lc.inputBuffer)-1]
		}
	}

	// Get input runes
	inputChars := ebiten.AppendInputChars(nil)
	for _, r := range inputChars {
		if len(lc.inputBuffer) < 32 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			lc.inputBuffer += string(r)
		}
	}

	// Confirm with Enter
	if lc.input.JustPressed(ebiten.KeyEnter) {
		if lc.inputBuffer == "" {
			lc.startJoin("CREATE")
		} else {
			lc.startJoin("CREATE:" + lc.inputBuffer)
		}
		lc.inputMode = false
		lc.inputBuffer = ""
	}

	// Cancel with Escape
	if lc.input.JustPressed(ebiten.KeyEscape) {
		lc.inputMode = false
		lc.inputBuffer = ""
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
			lc.showToast(resp.ErrorMessage, uiError)
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
		if lc.input.JustPressed(ebiten.KeySpace) || lc.input.JustPressed(ebiten.KeyEnter) {
			lc.game = nil
			lc.screen = screenRoom
		}
		return
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

// showToast displays a toast notification message
func (lc *LobbyClient) showToast(message string, msgColor color.Color) {
	lc.toastMessage = message
	lc.toastTimer = 3.0 // Show for 3 seconds
}

// drawToast draws the toast notification if active
func (lc *LobbyClient) drawToast(screen *ebiten.Image) {
	if lc.toastMessage == "" || lc.toastTimer <= 0 {
		return
	}

	// Calculate opacity based on remaining time (fade out in last 0.5 seconds)
	alpha := uint8(255)
	if lc.toastTimer < 0.5 {
		alpha = uint8(lc.toastTimer * 2 * 255)
	}

	// Toast panel
	toastWidth := 300
	toastHeight := 32
	toastX := (ScreenWidth - toastWidth) / 2
	toastY := ScreenHeight - 60

	// Background
	toastImg := ebiten.NewImage(toastWidth, toastHeight)
	toastImg.Fill(color.RGBA{40, 45, 55, alpha})

	// Border
	vector.StrokeLine(toastImg, 0, 0, float32(toastWidth), 0, 1, color.RGBA{100, 110, 130, alpha}, false)
	vector.StrokeLine(toastImg, 0, float32(toastHeight), float32(toastWidth), float32(toastHeight), 1, color.RGBA{100, 110, 130, alpha}, false)
	vector.StrokeLine(toastImg, 0, 0, 0, float32(toastHeight), 1, color.RGBA{100, 110, 130, alpha}, false)
	vector.StrokeLine(toastImg, float32(toastWidth), 0, float32(toastWidth), float32(toastHeight), 1, color.RGBA{100, 110, 130, alpha}, false)

	// Draw toast to screen
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(toastX), float64(toastY))
	screen.DrawImage(toastImg, op)

	// Draw message centered
	drawText(screen, toastX+16, toastY+10, lc.toastMessage, color.RGBA{220, 230, 240, alpha})
}

func (lc *LobbyClient) drawLobby(screen *ebiten.Image) {
	// Background
	screen.Fill(uiBackground)

	// Header panel
	drawPanel(screen, 0, 0, ScreenWidth, 64)
	drawText(screen, uiPanelPadding, 18, "LOBBY", uiTextPrimary)
	drawText(screen, uiPanelPadding, 38, "Q:Quick  C:Create  R:Refresh  Enter:Join  W/S:Navigate", uiTextSecondary)

	// Room list panel
	panelX := uiPanelMargin
	panelY := 64 + uiPanelMargin
	panelWidth := ScreenWidth - 2*uiPanelMargin
	panelHeight := ScreenHeight - 64 - 2*uiPanelMargin - 28
	drawPanel(screen, panelX, panelY, panelWidth, panelHeight)

	// Column headers
	headerY := panelY + uiPanelPadding + 4
	drawText(screen, panelX+uiPanelPadding+12, headerY, "ROOM", uiTextMuted)
	drawText(screen, panelX+uiPanelPadding+200, headerY, "PLAYERS", uiTextMuted)
	drawText(screen, panelX+uiPanelPadding+300, headerY, "STATUS", uiTextMuted)

	// Room list rows
	y := headerY + uiRowHeight + 4
	for i, room := range lc.roomList {
		if room == nil {
			continue
		}

		rowY := y + i*uiRowHeight
		if rowY > panelY+panelHeight-uiRowHeight {
			break
		}

		// Draw selection background
		if i == lc.selectedIndex {
			drawSelectionRect(screen, panelX+uiPanelPadding, rowY-2, panelWidth-2*uiPanelPadding, uiRowHeight)
		}

		// Row indicator
		indicator := "  "
		indicatorColor := uiTextSecondary
		if i == lc.selectedIndex {
			indicator = "> "
			indicatorColor = uiAccent
		}

		// Room name with ID
		name := room.Name
		if name == "" {
			name = room.Id
		}
		roomName := fmt.Sprintf("%s[%d] %s (%s)", indicator, i+1, name, room.Id)
		drawText(screen, panelX+uiPanelPadding, rowY+5, roomName, indicatorColor)

		// Player count
		playerCount := fmt.Sprintf("%d/%d", room.CurrentPlayers, room.MaxPlayers)
		drawText(screen, panelX+uiPanelPadding+200, rowY+5, playerCount, uiTextPrimary)

		// Status with color
		statusText, statusColor := roomStatusWithColor(room.Status)
		drawText(screen, panelX+uiPanelPadding+300, rowY+5, statusText, statusColor)
	}

	// Footer status
	footerY := ScreenHeight - 24
	if lc.joinInFlight {
		drawText(screen, panelX+uiPanelPadding, footerY, "JOINING...", uiAccent)
	}
	if lc.lastError != "" {
		drawText(screen, panelX+uiPanelPadding, footerY+12, lc.lastError, uiError)
	}

	// Draw input mode dialog
	if lc.inputMode {
		lc.drawInputDialog(screen)
	}

	// Draw toast notification
	lc.drawToast(screen)
}

// drawInputDialog draws the room ID input dialog
func (lc *LobbyClient) drawInputDialog(screen *ebiten.Image) {
	// Dim background
	dimImg := ebiten.NewImage(ScreenWidth, ScreenHeight)
	dimImg.Fill(color.RGBA{0, 0, 0, 150})
	op := &ebiten.DrawImageOptions{}
	screen.DrawImage(dimImg, op)

	// Dialog panel
	dialogWidth := 360
	dialogHeight := 100
	dialogX := (ScreenWidth - dialogWidth) / 2
	dialogY := (ScreenHeight - dialogHeight) / 2
	drawPanel(screen, dialogX, dialogY, dialogWidth, dialogHeight)

	// Dialog title
	drawText(screen, dialogX+uiPanelPadding, dialogY+16, "CREATE ROOM", uiAccent)

	// Input prompt
	drawText(screen, dialogX+uiPanelPadding, dialogY+38, "Enter Room ID (optional):", uiTextSecondary)

	// Input field with cursor
	inputY := dialogY + 60
	inputText := lc.inputBuffer
	if lc.inputBuffer == "" {
		inputText = "(random)"
	}

	// Draw input text
	drawText(screen, dialogX+uiPanelPadding, inputY, inputText, uiTextPrimary)

	// Draw blinking cursor
	cursorX := dialogX + uiPanelPadding + len(inputText)*8
	if int(lc.inputCursorBlink)%2 == 0 {
		cursorImg := ebiten.NewImage(2, 14)
		cursorImg.Fill(uiAccent)
		cursorOp := &ebiten.DrawImageOptions{}
		cursorOp.GeoM.Translate(float64(cursorX), float64(inputY))
		screen.DrawImage(cursorImg, cursorOp)
	}

	// Instructions
	drawText(screen, dialogX+uiPanelPadding, dialogY+84, "Enter:Confirm  Esc:Cancel", uiTextMuted)
}

func (lc *LobbyClient) drawRoom(screen *ebiten.Image) {
	screen.Fill(uiBackground)

	// Header panel
	drawPanel(screen, 0, 0, ScreenWidth, 64)

	roomID := "UNKNOWN"
	if lc.roomState != nil {
		roomID = lc.roomState.RoomId
	}
	drawText(screen, uiPanelPadding, 18, "ROOM: "+roomID, uiTextPrimary)
	drawText(screen, uiPanelPadding, 38, "Space:Ready  Enter:Start  A:AddAI  L:Leave", uiTextSecondary)

	// Players panel
	panelX := uiPanelMargin
	panelY := 64 + uiPanelMargin
	panelWidth := 280
	panelHeight := ScreenHeight - 64 - 2*uiPanelMargin - 28
	drawPanel(screen, panelX, panelY, panelWidth, panelHeight)

	// Players header
	headerY := panelY + uiPanelPadding + 4
	drawText(screen, panelX+uiPanelPadding, headerY, "PLAYERS", uiTextMuted)

	// Player list
	y := headerY + uiRowHeight + 4
	if lc.roomState != nil {
		for i, player := range lc.roomState.Players {
			if player == nil {
				continue
			}

			rowY := y + i*uiRowHeight
			if rowY > panelY+panelHeight-uiRowHeight {
				break
			}

			// Player flags
			flags := playerFlags(player)
			flagColor := uiTextMuted
			if player.IsReady {
				flagColor = uiSuccess
			}

			// Player name and character
			playerText := fmt.Sprintf(" %s %s", player.Name, shortCharacter(player.Character))
			drawText(screen, panelX+uiPanelPadding, rowY+5, flags, flagColor)
			drawText(screen, panelX+uiPanelPadding+28, rowY+5, playerText, uiTextPrimary)
		}
	}

	// Room info panel (right side)
	infoPanelX := panelX + panelWidth + uiPanelMargin
	infoPanelWidth := ScreenWidth - infoPanelX - uiPanelMargin
	drawPanel(screen, infoPanelX, panelY, infoPanelWidth, panelHeight)

	infoHeaderY := panelY + uiPanelPadding + 4
	drawText(screen, infoPanelX+uiPanelPadding, infoHeaderY, "ROOM INFO", uiTextMuted)

	// Room info content
	infoY := infoHeaderY + uiRowHeight + 8
	if lc.roomState != nil {
		playerCount := fmt.Sprintf("Players: %d / 4", len(lc.roomState.Players))
		drawText(screen, infoPanelX+uiPanelPadding, infoY, playerCount, uiTextPrimary)

		// Host indicator
		isHost := lc.roomState.HostId == lc.network.GetPlayerID()
		hostText := "You are: Host"
		hostColor := uiAccent
		if !isHost {
			hostText = "You are: Guest"
			hostColor = uiTextSecondary
		}
		drawText(screen, infoPanelX+uiPanelPadding, infoY+uiRowHeight, hostText, hostColor)
	}

	// Footer status
	footerY := ScreenHeight - 24
	if lc.lastError != "" {
		drawText(screen, panelX+uiPanelPadding, footerY, lc.lastError, uiError)
	}

	// Draw toast notification
	lc.drawToast(screen)
}

// drawPanel draws a panel with background and border
func drawPanel(screen *ebiten.Image, x, y, width, height int) {
	// Panel background
	panelImg := ebiten.NewImage(width, height)
	panelImg.Fill(uiPanelBackground)

	// Draw border using vector strokes
	borders := []struct {
		x1, y1, x2, y2 float32
	}{
		{0, 0, float32(width), 0},                             // Top
		{0, float32(height), float32(width), float32(height)}, // Bottom
		{0, 0, 0, float32(height)},                            // Left
		{float32(width), 0, float32(width), float32(height)},  // Right
	}

	for _, b := range borders {
		vector.StrokeLine(panelImg, b.x1, b.y1, b.x2, b.y2, uiInputBorder, uiPanelBorder, false)
	}

	// Draw panel to screen
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(panelImg, op)
}

// drawSelectionRect draws a selection highlight rectangle
func drawSelectionRect(screen *ebiten.Image, x, y, width, height int) {
	selectionImg := ebiten.NewImage(width, height)
	selectionImg.Fill(color.RGBA{255, 200, 80, 20})

	// Draw left accent bar
	vector.DrawFilledRect(selectionImg, 0, 0, 3, float32(height), uiAccent, false)

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(selectionImg, op)
}

// roomStatusWithColor returns status text and its color
func roomStatusWithColor(status gamev1.RoomStatus) (string, color.Color) {
	switch status {
	case gamev1.RoomStatus_ROOM_STATUS_WAITING:
		return "WAITING", uiRoomWaiting
	case gamev1.RoomStatus_ROOM_STATUS_PLAYING:
		return "PLAYING", uiRoomPlaying
	default:
		return "UNKNOWN", uiTextMuted
	}
}

// playerFlags returns player status flags
func playerFlags(player *gamev1.RoomPlayer) string {
	flags := "["
	if player.IsHost {
		flags += "H"
	} else {
		flags += " "
	}
	if player.IsAi {
		flags += "A"
	} else {
		flags += " "
	}
	if player.IsReady {
		flags += "R"
	} else {
		flags += " "
	}
	flags += "]"
	return flags
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
