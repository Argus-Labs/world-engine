package comp

import (
	cryptorand "crypto/rand"
	"math/big"
)

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
	}
	randomIndex, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(t.Nums))))
	if err != nil {
		panic("error generating random number, check the code.")
	}
	t.Nums[randomIndex.Int64()]++
	t.Left.UpdateTree()
	t.Right.UpdateTree()
}

func CreateTree(depth int) *Tree {
	if depth == 0 {
		return nil
	}
	children := CreateTree(depth - 1)
	return &Tree{
		Nums:  [10]int{},
		Left:  children,
		Right: children,
	}
}
