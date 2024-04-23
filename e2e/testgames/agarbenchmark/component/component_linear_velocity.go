package component

import (
	"github.com/ByteArena/box2d"
)

type LinearVelocity struct {
	X float64
	Y float64
}

func (LinearVelocity) Name() string {
	return "LinearVelocity"
}

func (lv LinearVelocity) ToB2Vec2() box2d.B2Vec2 {
	return box2d.B2Vec2{X: lv.X, Y: lv.Y}
}

func (lv LinearVelocity) FromB2Vec2(v box2d.B2Vec2) LinearVelocity {
	return LinearVelocity{X: v.X, Y: v.Y}
}
