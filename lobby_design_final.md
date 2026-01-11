# 极简大厅与匹配系统设计 (Final Design)

## 核心理念

**"一切皆房间"**：不引入独立的 Lobby 服务器实体或 Session 管理。
- **大厅** = 连接到服务器但未加入特定房间的状态（`roomID == ""`）
- **匹配** = 智能化的加入房间请求处理
- **准备** = 房间的等待状态 (WAITING)

### 兼容性保证

本设计完全兼容现有机制：

| 现有功能 | 兼容方式 |
|---------|---------|
| **TCP/KCP 双协议** | ✅ 不变。服务器同时监听两种协议，客户端可选择 |
| **重连机制 (JWT)** | ✅ 不变。Token 包含 `roomID`，重连时恢复到对应房间 |
| **会话令牌** | ✅ 扩展。大厅状态时 `roomID=""`, 加入房间后更新 Token |
| **单机模式** | ✅ 不变。不启动网络客户端时独立运行 |
| **直接联机** | ✅ 兼容。`room_id=""` 时自动匹配，行为与现在一致 |

---

## 1. 状态模型

### 1.1 客户端状态机

```
┌─────────────────────────────────────────────────────────────────────┐
│                         网络客户端状态                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌──────────┐    Connect     ┌──────────┐     Join      ┌────────┐ │
│  │ 未连接   │ ─────────────▶ │   大厅   │ ────────────▶ │  房间  │ │
│  │OFFLINE   │                │  LOBBY   │               │  ROOM  │ │
│  └──────────┘                └──────────┘               └────────┘ │
│       ▲                           ▲                          │     │
│       │                           │ LeaveRoom                │     │
│       │ Disconnect                ├──────────────────────────┘     │
│       │                           │                                │
│       │                           │ GameOver (自动返回)            │
│       │                      ┌────┴────┐                           │
│       └──────────────────────│  游戏中  │                           │
│                              │ PLAYING  │                           │
│                              └──────────┘                           │
│                                    ▲                                │
│                                    │ StartGame                      │
│                              ┌─────┴─────┐                          │
│                              │   房间    │                          │
│                              │ (WAITING) │                          │
│                              └───────────┘                          │
└─────────────────────────────────────────────────────────────────────┘
```

### 1.2 服务器端连接状态

```go
type ConnectionState int

const (
    StateDisconnected ConnectionState = iota // 未连接
    StateLobby                               // 大厅（已连接，未加入房间）
    StateInRoom                              // 在房间中（WAITING 或 PLAYING）
)
```

**核心变化**：连接后不再自动加入默认房间，而是进入"大厅"状态。

---

## 2. 协议扩展 (game.proto)

### 2.1 新增消息类型

```protobuf
// ========== 新增枚举 ==========

enum RoomStatus {
  ROOM_STATUS_UNSPECIFIED = 0;
  ROOM_STATUS_WAITING = 1;    // 等待中，可加入
  ROOM_STATUS_PLAYING = 2;    // 游戏中
}

enum RoomActionType {
  ROOM_ACTION_UNSPECIFIED = 0;
  ROOM_ACTION_LEAVE = 1;       // 离开房间 → 大厅
  ROOM_ACTION_READY = 2;       // 准备/取消准备
  ROOM_ACTION_START = 3;       // 开始游戏 (房主)
  ROOM_ACTION_ADD_AI = 4;      // 添加 AI (房主)
  ROOM_ACTION_KICK = 5;        // 踢人 (房主)
}

// ========== 客户端 → 服务器 ==========

// 获取房间列表
message RoomListRequest {
  int32 page = 1;       // 分页（可选）
  int32 page_size = 2;  // 每页数量（可选，默认 20）
}

// 房间操作（统一入口）
message RoomAction {
  RoomActionType type = 1;
  
  // 可选参数
  bool ready = 2;          // READY: true=准备, false=取消
  int32 ai_count = 3;      // ADD_AI: 添加几个 AI
  int32 target_player = 4; // KICK: 踢谁
}

// ========== 服务器 → 客户端 ==========

// 房间列表响应
message RoomListResponse {
  repeated RoomInfo rooms = 1;
  int32 total = 2;        // 总数
}

// 房间信息
message RoomInfo {
  string id = 1;
  string name = 2;             // 如 "玩家A的房间"
  int32 current_players = 3;   // 当前玩家数（不含 AI）
  int32 ai_count = 4;          // AI 数量
  int32 max_players = 5;       // 最大玩家数（默认 4）
  RoomStatus status = 6;       // 房间状态
  string host_name = 7;        // 房主名称
}

// 房间操作响应
message RoomActionResponse {
  bool success = 1;
  string error_message = 2;
}

// 房间状态更新（广播给房间内所有玩家）
message RoomStateUpdate {
  string room_id = 1;
  RoomStatus status = 2;
  repeated RoomPlayer players = 3;
  int32 host_id = 4;
}

// 房间内玩家信息
message RoomPlayer {
  int32 id = 1;
  string name = 2;
  CharacterType character = 3;
  bool is_ready = 4;
  bool is_host = 5;
  bool is_ai = 6;
}
```

### 2.2 修改现有消息

```protobuf
// JoinRequest - 扩展语义
message JoinRequest {
  string player_name = 1;
  CharacterType character = 2;
  
  // room_id 语义扩展：
  // - ""        : 快速匹配（找一个 WAITING 且未满的房间，或创建新房间）
  // - "CREATE"  : 强制创建新房间
  // - "room_xxx": 加入指定房间
  string room_id = 3;
}

// JoinResponse - 扩展返回
message JoinResponse {
  bool success = 1;
  int32 player_id = 2;
  string error_message = 3;
  
  int64 game_seed = 4;
  int32 tps = 5;
  string session_token = 10;
  
  // 新增：房间信息
  string room_id = 11;           // 实际加入的房间 ID
  RoomStateUpdate room_state = 12; // 房间当前状态
}

// Packet - 新增消息类型
enum MessageType {
  // ... 现有类型 ...
  
  // 新增
  MESSAGE_TYPE_ROOM_LIST_REQUEST = 20;
  MESSAGE_TYPE_ROOM_LIST_RESPONSE = 21;
  MESSAGE_TYPE_ROOM_ACTION = 22;
  MESSAGE_TYPE_ROOM_ACTION_RESPONSE = 23;
  MESSAGE_TYPE_ROOM_STATE_UPDATE = 24;
}

// GameEvent - 新增事件
message GameEvent {
  int32 frame_id = 1;
  
  oneof event {
    // ... 现有事件 ...
    
    // 新增
    RoomStateUpdate room_update = 10;  // 房间状态变更
  }
}
```

---

## 3. 服务器端实现

### 3.1 Connection 扩展

```go
// internal/server/connection.go

type Connection struct {
    // ... 现有字段 ...
    
    // 新增
    state      ConnectionState // 连接状态
    playerName string          // 玩家名称（大厅阶段设置）
}

// 连接建立后进入大厅状态
func (c *Connection) Handle(ctx context.Context, wg *sync.WaitGroup) {
    c.state = StateLobby  // 新：默认进入大厅
    // ...
}
```

### 3.2 Room 扩展

```go
// internal/server/room.go

type Room struct {
    // ... 现有字段 ...
    
    // 新增：大厅系统
    hostID      int32               // 房主 ID
    readyStatus map[int32]bool      // 玩家准备状态
    playerNames map[int32]string    // 玩家名称
    roomName    string              // 房间名称
}

// 新增方法

// CanStart 检查是否可以开始游戏
func (r *Room) CanStart(requestorID int32) (bool, string) {
    // 1. 必须是房主
    if requestorID != r.hostID {
        return false, "只有房主可以开始游戏"
    }
    
    // 2. 至少 2 个玩家（真人 + AI）
    totalPlayers := len(r.connections) + len(r.aiControllers)
    if totalPlayers < 2 {
        return false, "至少需要 2 名玩家"
    }
    
    // 3. 所有真人玩家必须准备
    for playerID := range r.connections {
        if playerID == r.hostID {
            continue // 房主不需要准备
        }
        if !r.readyStatus[playerID] {
            return false, "有玩家未准备"
        }
    }
    
    return true, ""
}

// HandleRoomAction 处理房间操作
func (r *Room) HandleRoomAction(playerID int32, action *gamev1.RoomAction) error {
    switch action.Type {
    case gamev1.RoomActionType_ROOM_ACTION_READY:
        r.readyStatus[playerID] = action.Ready
        r.broadcastRoomState()
        
    case gamev1.RoomActionType_ROOM_ACTION_START:
        if ok, msg := r.CanStart(playerID); !ok {
            return errors.New(msg)
        }
        r.startGame()
        
    case gamev1.RoomActionType_ROOM_ACTION_ADD_AI:
        if playerID != r.hostID {
            return errors.New("只有房主可以添加 AI")
        }
        r.addAI(int(action.AiCount))
        r.broadcastRoomState()
        
    case gamev1.RoomActionType_ROOM_ACTION_LEAVE:
        r.Leave(playerID)
        
    case gamev1.RoomActionType_ROOM_ACTION_KICK:
        if playerID != r.hostID {
            return errors.New("只有房主可以踢人")
        }
        r.kickPlayer(action.TargetPlayer)
    }
    return nil
}

// transferHost 转移房主
func (r *Room) transferHost() {
    for playerID := range r.connections {
        if playerID != r.hostID {
            r.hostID = playerID
            log.Printf("房主转移给玩家 %d", playerID)
            r.broadcastRoomState()
            return
        }
    }
}
```

### 3.3 RoomManager 扩展

```go
// internal/server/room_manager.go

type RoomManager struct {
    // ... 现有字段 ...
    
    // 新增：大厅连接（未加入房间的玩家）
    lobbyConnections map[int32]Session
    lobbyMutex       sync.RWMutex
}

// Join 智能加入逻辑
func (m *RoomManager) Join(session Session, req JoinEvent) error {
    roomID := req.RoomID
    
    switch roomID {
    case "":
        // 快速匹配：找一个可加入的房间
        roomID = m.findAvailableRoom()
        if roomID == "" {
            roomID = m.CreateRoom() // 没有则创建
        }
        
    case "CREATE":
        // 强制创建新房间
        roomID = m.CreateRoom()
        
    default:
        // 加入指定房间
        if !m.roomExists(roomID) {
            return fmt.Errorf("房间 %s 不存在", roomID)
        }
    }
    
    // 从大厅移除
    m.removeFromLobby(session.ID())
    
    // 加入房间
    return m.joinRoom(session, roomID, req)
}

// findAvailableRoom 查找可加入的房间
func (m *RoomManager) findAvailableRoom() string {
    m.roomMutex.RLock()
    defer m.roomMutex.RUnlock()
    
    for roomID, room := range m.rooms {
        if roomID == DefaultRoomID {
            continue // 跳过默认房间（兼容模式）
        }
        if room.state == StateWaiting && 
           len(room.connections) < MaxPlayers {
            return roomID
        }
    }
    return ""
}

// GetRoomList 获取房间列表
func (m *RoomManager) GetRoomList() []*gamev1.RoomInfo {
    m.roomMutex.RLock()
    defer m.roomMutex.RUnlock()
    
    var list []*gamev1.RoomInfo
    for roomID, room := range m.rooms {
        if roomID == DefaultRoomID {
            continue
        }
        list = append(list, &gamev1.RoomInfo{
            Id:             roomID,
            Name:           room.roomName,
            CurrentPlayers: int32(len(room.connections)),
            AiCount:        int32(len(room.aiControllers)),
            MaxPlayers:     MaxPlayers,
            Status:         gamev1.RoomStatus(room.state),
            HostName:       room.playerNames[room.hostID],
        })
    }
    return list
}
```

### 3.4 GameServer 消息路由

```go
// internal/server/game_server.go

func (s *GameServer) handleMessage(conn Session, pkt *gamev1.Packet) {
    switch pkt.Type {
    // ... 现有处理 ...
    
    case gamev1.MessageType_MESSAGE_TYPE_ROOM_LIST_REQUEST:
        s.handleRoomListRequest(conn)
        
    case gamev1.MessageType_MESSAGE_TYPE_ROOM_ACTION:
        s.handleRoomAction(conn, pkt)
    }
}

func (s *GameServer) handleRoomListRequest(conn Session) {
    rooms := s.roomManager.GetRoomList()
    resp := &gamev1.RoomListResponse{
        Rooms: rooms,
        Total: int32(len(rooms)),
    }
    // 发送响应...
}
```

---

## 4. 重连机制兼容

### 4.1 Token 扩展

当前 JWT Claims：
```go
type Claims struct {
    PlayerID int32  `json:"player_id"`
    RoomID   string `json:"room_id,omitempty"`
    jwt.RegisteredClaims
}
```

**无需修改**。重连逻辑：

```go
func (s *GameServer) handleReconnect(conn Session, req *ReconnectEvent) {
    playerID, roomID, err := VerifySessionToken(req.SessionToken)
    if err != nil {
        // Token 无效
        s.sendReconnectResponse(conn, false, "会话过期", nil)
        return
    }
    
    if roomID == "" {
        // 玩家在大厅断线，恢复到大厅
        s.roomManager.AddToLobby(conn, playerID)
        s.sendReconnectResponse(conn, true, "", nil)
        return
    }
    
    // 玩家在房间中断线，恢复到房间
    state, err := s.roomManager.ReconnectPlayer(conn.ID(), playerID, roomID, conn)
    if err != nil {
        // 房间已不存在，返回大厅
        s.roomManager.AddToLobby(conn, playerID)
        s.sendReconnectResponse(conn, true, "房间已关闭，返回大厅", nil)
        return
    }
    
    s.sendReconnectResponse(conn, true, "", state)
}
```

### 4.2 重连时 Token 更新

当玩家状态变化时（加入/离开房间），需要更新 Token：

```go
// 加入房间后更新 Token
func (r *Room) handleJoin(req joinRequest) {
    // ... 现有逻辑 ...
    
    // 生成新 Token（包含 roomID）
    newToken, _ := GenerateSessionToken(playerID, r.id)
    
    // 发送 JoinResponse 时包含新 Token
    resp.SessionToken = newToken
}

// 离开房间后更新 Token
func (r *Room) handleLeave(playerID int32) {
    // ... 现有逻辑 ...
    
    // 通知 RoomManager 将玩家移回大厅
    // RoomManager 负责生成新 Token（roomID=""）
}
```

---

## 5. 客户端实现

### 5.1 NetworkClient 扩展

```go
// internal/client/network.go

type NetworkClient struct {
    // ... 现有字段 ...
    
    // 新增：大厅状态
    state          ClientState
    currentRoomID  string
    roomStateChan  chan *gamev1.RoomStateUpdate
    roomListChan   chan *gamev1.RoomListResponse
}

type ClientState int

const (
    ClientStateDisconnected ClientState = iota
    ClientStateLobby     // 在大厅
    ClientStateInRoom    // 在房间（等待中）
    ClientStatePlaying   // 游戏中
)

// Connect 连接到服务器（进入大厅，不自动加入房间）
func (nc *NetworkClient) Connect() error {
    // ... 建立连接 ...
    nc.state = ClientStateLobby
    return nil
}

// RequestRoomList 请求房间列表
func (nc *NetworkClient) RequestRoomList() error {
    packet, _ := protocol.NewRoomListRequestPacket()
    data, _ := protocol.MarshalPacket(packet)
    return nc.sendMessage(data)
}

// JoinRoom 加入房间
// roomID: "" = 快速匹配, "CREATE" = 创建, 其他 = 指定房间
func (nc *NetworkClient) JoinRoom(roomID string) error {
    // 发送 JoinRequest
    // ...
    nc.state = ClientStateInRoom
    return nil
}

// SendRoomAction 发送房间操作
func (nc *NetworkClient) SendRoomAction(action *gamev1.RoomAction) error {
    packet, _ := protocol.NewRoomActionPacket(action)
    data, _ := protocol.MarshalPacket(packet)
    return nc.sendMessage(data)
}

// LeaveRoom 离开房间回到大厅
func (nc *NetworkClient) LeaveRoom() error {
    action := &gamev1.RoomAction{
        Type: gamev1.RoomActionType_ROOM_ACTION_LEAVE,
    }
    err := nc.SendRoomAction(action)
    if err == nil {
        nc.state = ClientStateLobby
        nc.currentRoomID = ""
    }
    return err
}
```

### 5.2 UI 状态机

```go
// internal/client/lobby_ui.go

type LobbyUI struct {
    network    *NetworkClient
    screen     UIScreen
    roomList   []*gamev1.RoomInfo
    roomState  *gamev1.RoomStateUpdate
}

type UIScreen int

const (
    ScreenLobby    UIScreen = iota // 大厅界面
    ScreenRoom                     // 房间界面（等待中）
    ScreenGame                     // 游戏界面
)

func (ui *LobbyUI) Update() {
    switch ui.screen {
    case ScreenLobby:
        ui.updateLobby()
    case ScreenRoom:
        ui.updateRoom()
    case ScreenGame:
        // 交给 NetworkGameClient 处理
    }
}

func (ui *LobbyUI) updateLobby() {
    // 处理按钮点击
    if quickMatchClicked {
        ui.network.JoinRoom("")
    }
    if createRoomClicked {
        ui.network.JoinRoom("CREATE")
    }
    if roomItemClicked {
        ui.network.JoinRoom(selectedRoomID)
    }
    
    // 处理服务器响应
    if resp := ui.network.ReceiveJoinResponse(); resp != nil {
        if resp.Success {
            ui.roomState = resp.RoomState
            ui.screen = ScreenRoom
        }
    }
}

func (ui *LobbyUI) updateRoom() {
    // 处理按钮点击
    if readyClicked {
        ui.network.SendRoomAction(&gamev1.RoomAction{
            Type:  gamev1.RoomActionType_ROOM_ACTION_READY,
            Ready: !currentReadyState,
        })
    }
    if startClicked && isHost {
        ui.network.SendRoomAction(&gamev1.RoomAction{
            Type: gamev1.RoomActionType_ROOM_ACTION_START,
        })
    }
    if leaveClicked {
        ui.network.LeaveRoom()
        ui.screen = ScreenLobby
    }
    
    // 处理状态更新
    if update := ui.network.ReceiveRoomState(); update != nil {
        ui.roomState = update
    }
    
    // 处理游戏开始
    if event := ui.network.ReceiveEvent(); event != nil {
        if _, ok := event.Event.(*gamev1.GameEvent_GameStart); ok {
            ui.screen = ScreenGame
        }
    }
}
```

---

## 6. 向后兼容

### 6.1 兼容模式

保留 `DefaultRoomID = "default"` 作为兼容模式：

```go
// 如果客户端直接发送 JoinRequest（旧版本行为）
// 且 room_id == ""，自动加入默认房间并立即开始
func (m *RoomManager) Join(session Session, req JoinEvent) error {
    // 检测是否为旧版本客户端（不支持大厅）
    if req.IsLegacyClient {
        return m.joinDefaultRoom(session, req)
    }
    // ... 新逻辑
}
```

### 6.2 启动参数

```bash
# 新模式（默认）：启用大厅
go run cmd/client/main.go -server=localhost:8080

# 兼容模式：跳过大厅，直接进入游戏
go run cmd/client/main.go -server=localhost:8080 -quick

# 单机模式：不变
go run cmd/client/main.go
```

---

## 7. 实施步骤

### Phase 1: 协议层 (0.5 天)

1. 修改 `api/proto/bomberman/v1/game.proto`
   - 添加 `RoomStatus`, `RoomActionType` 枚举
   - 添加 `RoomListRequest/Response`, `RoomAction`, `RoomStateUpdate` 消息
   - 扩展 `MessageType` 枚举
2. 运行 `make gen`
3. 更新 `pkg/protocol/helper.go` 添加新消息构造函数

### Phase 2: 服务器核心 (1 天)

1. **Room 扩展**：
   - 添加 `hostID`, `readyStatus`, `playerNames` 字段
   - 实现 `HandleRoomAction`, `CanStart`, `transferHost`
   - 修改 `handleJoin` 设置房主
   - 修改 `handleLeave` 处理房主离开

2. **RoomManager 扩展**：
   - 添加 `lobbyConnections` 管理
   - 实现 `GetRoomList`, `findAvailableRoom`
   - 修改 `Join` 实现智能匹配

3. **GameServer 扩展**：
   - 添加消息路由
   - 处理 `RoomListRequest`, `RoomAction`

### Phase 3: 客户端网络 (0.5 天)

1. **NetworkClient 扩展**：
   - 添加状态管理
   - 实现 `RequestRoomList`, `JoinRoom`, `SendRoomAction`, `LeaveRoom`
   - 添加 `roomStateChan`, `roomListChan`

2. **协议处理**：
   - 更新 `handleMessage` 处理新消息类型

### Phase 4: 客户端 UI (1 天)

1. **LobbyUI**：
   - 房间列表展示
   - 快速匹配 / 创建房间 / 加入房间 按钮
   
2. **RoomUI**：
   - 玩家列表（头像、名称、准备状态）
   - 准备 / 开始游戏 / 添加AI / 离开 按钮

3. **状态机集成**：
   - 修改 `cmd/client/main.go` 入口

### Phase 5: 测试 (0.5 天)

1. 单人流程测试
2. 多人匹配测试
3. 重连测试（大厅断线、房间断线）
4. TCP/KCP 切换测试
5. 向后兼容测试

---

## 8. 数据流示意

### 8.1 快速匹配流程

```
Client                          Server
   │                               │
   │──── Connect (TCP/KCP) ───────▶│
   │◀─── Connected ────────────────│ state = LOBBY
   │                               │
   │──── JoinRequest(room="") ────▶│ findAvailableRoom()
   │                               │ → 找到 room_123
   │◀─── JoinResponse ────────────│ (roomID, roomState, token)
   │     + RoomStateUpdate         │ state = IN_ROOM
   │                               │
   │──── RoomAction(READY) ───────▶│ readyStatus[player] = true
   │◀─── RoomStateUpdate ─────────│ 广播给房间所有人
   │                               │
   │     (其他玩家也准备好)         │
   │                               │
   │──── RoomAction(START) ───────▶│ (房主点击开始)
   │◀─── GameEvent(GameStart) ────│ state = PLAYING
   │                               │
   │     ... 游戏进行中 ...         │
   │                               │
   │◀─── GameEvent(GameOver) ─────│ winnerID
   │◀─── RoomStateUpdate ─────────│ state = WAITING
   │                               │ (自动返回房间等待界面)
```

### 8.2 重连流程

```
Client                          Server
   │                               │
   │     (断线)                     │
   │                               │
   │──── Connect (KCP) ───────────▶│
   │──── ReconnectRequest(token) ─▶│ VerifyToken()
   │                               │ → playerID=5, roomID="room_123"
   │                               │ ReplaceConnection()
   │◀─── ReconnectResponse ───────│ (success, currentState)
   │                               │
   │     (继续游戏)                 │
```

### 8.3 TCP → KCP 协议切换

**现有机制完全保留**：

1. **首次连接**：客户端通过 `-proto=tcp` 或 `-proto=kcp` 选择协议
2. **重连时切换**：`Reconnect()` 方法自动使用 KCP 以获得更低延迟
3. **服务器双监听**：同时接受 TCP 和 KCP 连接，对 Session 接口透明

```go
// internal/client/network.go (现有代码)
func (nc *NetworkClient) Reconnect() (*gamev1.GameState, error) {
    // ...
    // 重连时使用 KCP
    conn, err := nc.dialKCP()
    // ...
}
```

---

## 9. 扩展方向

### 9.1 房间设置

```protobuf
message CreateRoomRequest {
  string room_name = 1;    // 房间名称
  int32 max_players = 2;   // 最大玩家数
  bool private = 3;        // 是否私有
  string password = 4;     // 房间密码
}
```

### 9.2 聊天系统

```protobuf
message ChatMessage {
  int32 sender_id = 1;
  string content = 2;
  int64 timestamp = 3;
  ChatScope scope = 4;     // ROOM / LOBBY / WHISPER
}
```

### 9.3 匹配优化

- 基于技能等级 (MMR) 匹配
- 地区优先匹配
- 好友邀请系统

---

## 10. 总结

本设计基于"一切皆房间"的简化理念，通过以下方式实现大厅与匹配系统：

| 功能点 | 实现方式 |
|-------|---------|
| **大厅** | 连接后 `roomID=""` 的特殊状态 |
| **快速匹配** | `JoinRequest(room_id="")` 智能选择房间 |
| **创建房间** | `JoinRequest(room_id="CREATE")` |
| **加入房间** | `JoinRequest(room_id="xxx")` |
| **房间操作** | 统一的 `RoomAction` 消息 |
| **重连** | JWT Token 携带 `roomID`，无缝恢复 |
| **协议切换** | 重连时自动使用 KCP |

**优势**：
- ✅ 最小化协议改动（复用 `JoinRequest`）
- ✅ 完全兼容现有 TCP/KCP 双协议
- ✅ 完全兼容现有 JWT 重连机制
- ✅ 向后兼容旧版客户端
- ✅ 实现成本低（约 3.5 天）
