package ecs

import (
	"errors"

	"github.com/cometbft/cometbft/crypto"
)

const numOfCharactersInGUID = 24

// GUIDComponent is a unique game identification string. It is generated randomly on tick 0 and
// will never change.
type GUIDComponent struct {
	GUID string `json:"guid"`
}

func (GUIDComponent) Name() string {
	return "GameUniqueIDComponent"
}

var ErrGUIDNotFound = errors.New("guid not found")

func getGUID(wCtx WorldContext) (guid string, err error) {
	search, err := wCtx.NewSearch(Exact(GUIDComponent{}))
	if err != nil {
		return "", err
	}
	count, err := search.Count(wCtx)
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "", ErrGUIDNotFound
	}
	id, err := search.First(wCtx)
	if err != nil {
		return "", err
	}
	guidComp, err := GetComponent[GUIDComponent](wCtx, id)
	if err != nil {
		return "", err
	}
	return guidComp.GUID, nil
}

func CreateGUIDSystem(wCtx WorldContext) error {
	_, err := getGUID(wCtx)
	if err == nil {
		// The GUID has already been created
		return nil
	}
	if !errors.Is(err, ErrGUIDNotFound) {
		// There was some error when looking for the guid
		return err
	}
	// The guid hasn't been created yet
	guid := crypto.CRandHex(numOfCharactersInGUID)
	id, err := Create(wCtx, GUIDComponent{})
	if err != nil {
		return err
	}
	return SetComponent[GUIDComponent](wCtx, id, &GUIDComponent{guid})
}

type GUIDRequest struct{}
type GUIDReply struct {
	GUID string `json:"guid"`
}

func setupGUIDQuery(world *World) error {
	return RegisterQuery[GUIDRequest, GUIDReply](
		world,
		"guid",
		func(wCtx WorldContext, _ *GUIDRequest) (*GUIDReply, error) {
			guid, err := getGUID(wCtx)
			if err != nil {
				return nil, err
			}
			return &GUIDReply{guid}, nil
		})
}
