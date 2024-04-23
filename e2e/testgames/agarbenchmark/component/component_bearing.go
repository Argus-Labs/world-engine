package component

import (
	"math"

	"github.com/ByteArena/box2d"
)

type Bearing struct {
	Degrees float64
}

func (Bearing) Name() string {
	return "Bearing"
}

// ToDirection converts a bearing to a unit vector which may not be suitable for use as velocity.
func (bearing Bearing) ToDirection() box2d.B2Vec2 {
	radians := bearing.Degrees * (math.Pi / 180.0)
	x := math.Cos(radians)
	y := math.Sin(radians)
	return box2d.MakeB2Vec2(x, y)
}

// FromVelocity converts any vector to a 360° bearing.
func (bearing Bearing) FromVelocity(v box2d.B2Vec2) Bearing {
	radians := math.Atan2(v.Y, v.X)
	degrees := radians * (180.0 / math.Pi)
	return Bearing{Degrees: degrees}
}
