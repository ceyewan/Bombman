package server

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWT 相关配置
const (
	// Session 有效期：5 分钟（一局游戏的时间）
	SessionTTL = 5 * time.Minute

	// Token 签名者
	tokenIssuer = "bomberman-server"
)

// Claims 定义 JWT Claims
type Claims struct {
	PlayerID int32  `json:"player_id"`
	RoomID   string `json:"room_id,omitempty"`
	jwt.RegisteredClaims
}

// getSigningKey 获取签名密钥
// 从环境变量 JWT_SECRET 读取，如果不存在则使用默认值
func getSigningKey() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// 开发环境默认密钥，生产环境应设置环境变量
		secret = "bomberman-dev-secret-change-in-production"
	}
	return []byte(secret)
}

// GenerateSessionToken 生成会话 Token
func GenerateSessionToken(playerID int32, roomID string) (string, error) {
	now := time.Now()
	claims := Claims{
		PlayerID: playerID,
		RoomID:   roomID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   fmt.Sprintf("player-%d", playerID),
			ExpiresAt: jwt.NewNumericDate(now.Add(SessionTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(getSigningKey())
}

// VerifySessionToken 验证并解析 Token
// 返回 playerID 和 error
func VerifySessionToken(tokenString string) (int32, string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// 验证签名算法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return getSigningKey(), nil
	})

	if err != nil {
		return 0, "", fmt.Errorf("token parsing failed: %w", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims.PlayerID, claims.RoomID, nil
	}

	return 0, "", fmt.Errorf("invalid token")
}
