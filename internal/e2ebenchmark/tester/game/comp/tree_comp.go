package comp

import "math/rand"

type Tree struct {
	Nums  [10]int `json:"nums"`
	Left  *Tree   `json:"left"`
	Right *Tree   `json:"right"`
}

func (Tree) Name() string {
	return "Tree"
}

func (t *Tree) UpdateTree() {
	if t == nil {
		return
	} else {
		randomIndex := rand.Int() % 10
		t.Nums[randomIndex] += 1
		t.Left.UpdateTree()
		t.Right.UpdateTree()
	}
}

func CreateTree(depth int) *Tree {
	if depth == 0 {
		return nil
	} else {
		children := CreateTree(depth - 1)
		return &Tree{
			Nums:  [10]int{},
			Left:  children,
			Right: children,
		}
	}
}
