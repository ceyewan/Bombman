package core

//go:generate stringer -linecomment -type=CharacterType

// CharacterType 角色类型
type CharacterType int

const (
	CharacterWhite CharacterType = iota // 经典白色炸弹人
	CharacterBlack                      // 黑色炸弹人
	CharacterRed                        // 红色炸弹人
	CharacterBlue                       // 蓝色炸弹人
)

// String 返回角色类型的字符串表示
func (c CharacterType) String() string {
	switch c {
	case CharacterWhite:
		return "经典白"
	case CharacterBlack:
		return "暗夜黑"
	case CharacterRed:
		return "烈焰红"
	case CharacterBlue:
		return "冰霜蓝"
	}
	return "未知"
}
