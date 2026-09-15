package box2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/stretchr/testify/require"
)

func TestMakeProxyCopiesRequestedPrefix(t *testing.T) {
	points := []box2d.Vec2{{X: 1, Y: 2}, {X: 3, Y: 4}, {X: 5, Y: 6}}
	proxy := box2d.MakeProxy(points, 2, 0.5)
	points[0].X = 99
	require.Equal(t, 2, proxy.Count)
	require.Equal(t, []box2d.Vec2{{X: 1, Y: 2}, {X: 3, Y: 4}}, proxy.Points[:2])
	require.Equal(t, box2d.Vec2{}, proxy.Points[2])
	require.InDelta(t, 0.5, proxy.Radius, 0)
}

func TestMakeProxyRejectsCountOutsideInputLength(t *testing.T) {
	points := make([]box2d.Vec2, 1, 8)
	for _, count := range []int{-1, 2} {
		require.PanicsWithValue(t, "box2d: proxy point count is outside the input slice", func() {
			box2d.MakeProxy(points, count, 0)
		})
	}
}
