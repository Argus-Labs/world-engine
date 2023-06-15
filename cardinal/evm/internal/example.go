package main

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type SendEnergyTx struct {
	To     string
	From   string
	Amount uint64
}

type ClaimPlanetTx struct {
	UserId   string
	PlanetId uint64
	Healthy  bool
}

type FooBarTx struct {
	Memes         []int
	Stories       []string
	Okays         []bool
	ManyLargeNums []*big.Int
	ManyAddrs     []common.Address
	Okay          bool
	SmolNum       int8
	SmolUNum      uint8
	LargeNum      *big.Int
	Addr          common.Address
	Bytes         []byte
}
