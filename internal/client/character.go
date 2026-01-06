package client

import (
	"image/color"
	"bomberman/pkg/core"
)

// CharacterType 重新导出 core.CharacterType，方便使用
type CharacterType = core.CharacterType

// 常量重新导出
const (
	CharacterWhite = core.CharacterWhite
	CharacterBlack = core.CharacterBlack
	CharacterRed   = core.CharacterRed
	CharacterBlue  = core.CharacterBlue
)

// CharacterInfo 角色信息（渲染相关）
type CharacterInfo struct {
	Type         core.CharacterType
	Name         string
	BodyColor    color.RGBA
	OutlineColor color.RGBA
	HandColor    color.RGBA
	ShoeColor    color.RGBA
	Description  string
}

// GetCharacterInfo 获取角色信息
func GetCharacterInfo(charType core.CharacterType) CharacterInfo {
	switch charType {
	case core.CharacterWhite:
		return CharacterInfo{
			Type:         core.CharacterWhite,
			Name:         "经典白",
			BodyColor:    color.RGBA{255, 255, 255, 255},
			OutlineColor: color.RGBA{0, 0, 0, 255},
			HandColor:    color.RGBA{255, 150, 150, 255},
			ShoeColor:    color.RGBA{50, 50, 50, 255},
			Description:  "经典炸弹人造型",
		}
	case core.CharacterBlack:
		return CharacterInfo{
			Type:         core.CharacterBlack,
			Name:         "暗夜黑",
			BodyColor:    color.RGBA{40, 40, 40, 255},
			OutlineColor: color.RGBA{200, 200, 200, 255},
			HandColor:    color.RGBA{80, 80, 120, 255},
			ShoeColor:    color.RGBA{180, 180, 180, 255},
			Description:  "神秘的暗夜战士",
		}
	case core.CharacterRed:
		return CharacterInfo{
			Type:         core.CharacterRed,
			Name:         "烈焰红",
			BodyColor:    color.RGBA{255, 80, 80, 255},
			OutlineColor: color.RGBA{150, 0, 0, 255},
			HandColor:    color.RGBA{255, 200, 100, 255},
			ShoeColor:    color.RGBA{100, 0, 0, 255},
			Description:  "火热的爆破专家",
		}
	case core.CharacterBlue:
		return CharacterInfo{
			Type:         core.CharacterBlue,
			Name:         "冰霜蓝",
			BodyColor:    color.RGBA{100, 180, 255, 255},
			OutlineColor: color.RGBA{0, 50, 150, 255},
			HandColor:    color.RGBA{150, 220, 255, 255},
			ShoeColor:    color.RGBA{0, 30, 100, 255},
			Description:  "冷静的策略大师",
		}
	default:
		return GetCharacterInfo(core.CharacterWhite)
	}
}

// GetAllCharacters 获取所有角色
func GetAllCharacters() []CharacterInfo {
	return []CharacterInfo{
		GetCharacterInfo(core.CharacterWhite),
		GetCharacterInfo(core.CharacterBlack),
		GetCharacterInfo(core.CharacterRed),
		GetCharacterInfo(core.CharacterBlue),
	}
}
