package world

type PickupType int

const CoinPickup PickupType = 0
const MedpackPickup PickupType = 1

type GameSettings struct {
	seed int

	gridWidth  int
	gridHeight int

	maxPlayers          int
	playerRadius        float64
	playerSpeed         float64
	initialPlayerHealth int
	maxPlayerHealth     int

	coinRadius         float64
	maxConcurrentCoins int
	coinScore          int

	medpackRadius          float64
	maxConcurrentMedpacks  int
	medpacksPerSquareMeter float64
	medpackScore           int

	keepAliveInterval int
}

type GameSettingsJSON struct {
	Seed int `json:"seed"`

	GridWidth  int `json:"gridWidth"`
	GridHeight int `json:"gridHeight"`

	MaxPlayers          int     `json:"maxPlayers"`
	PlayerRadius        float64 `json:"playerRadius"`
	PlayerSpeed         float64 `json:"playerSpeed"`
	InitialPlayerHealth int     `json:"initialPlayerHealth"`
	MaxPlayerHealth     int     `json:"maxPlayerHealth"`

	CoinRadius         float64 `json:"coinRadius"`
	MaxConcurrentCoins int     `json:"maxConcurrentCoins"`
	CoinScore          int     `json:"coinScore"`

	MedpackRadius          float64 `json:"medpackRadius"`
	MaxConcurrentMedpacks  int     `json:"maxConcurrentMedpacks"`
	MedpacksPerSquareMeter float64 `json:"medpacksPerSquareMeter"`
	MedpackScore           int     `json:"medpackScore"`

	KeepAliveInterval int `json:"keepAliveInterval"`
}

func (gs *GameSettings) Seed() int {
	return gs.seed
}

func (gs *GameSettings) GridWidth() int {
	return gs.gridWidth
}

func (gs *GameSettings) GridHeight() int {
	return gs.gridHeight
}

func (gs *GameSettings) MaxPlayers() int {
	return gs.maxPlayers
}

func (gs *GameSettings) PlayerRadius() float64 {
	return gs.playerRadius
}

func (gs *GameSettings) PlayerSpeed() float64 {
	return gs.playerSpeed
}

func (gs *GameSettings) InitialPlayerHealth() int {
	return gs.initialPlayerHealth
}

func (gs *GameSettings) MaxPlayerHealth() int {
	return gs.maxPlayerHealth
}

func (gs *GameSettings) CoinRadius() float64 {
	return gs.coinRadius
}

func (gs *GameSettings) MaxConcurrentCoins() int {
	return gs.maxConcurrentCoins
}

func (gs *GameSettings) CoinScore() int {
	return gs.coinScore
}

func (gs *GameSettings) MedpackRadius() float64 {
	return gs.medpackRadius
}

func (gs *GameSettings) MaxConcurrentMedpacks() int {
	return gs.maxConcurrentMedpacks
}

func (gs *GameSettings) MedpacksPerSquareMeter() float64 {
	return gs.medpacksPerSquareMeter
}

func (gs *GameSettings) MedpackScore() int {
	return gs.medpackScore
}

func (gs *GameSettings) KeepAliveInterval() int {
	return gs.keepAliveInterval
}

func (gs *GameSettings) GameSettingsJSONAble() GameSettingsJSON {
	return GameSettingsJSON{
		Seed:                   gs.seed,
		GridWidth:              gs.gridWidth,
		GridHeight:             gs.gridHeight,
		MaxPlayers:             gs.maxPlayers,
		PlayerRadius:           gs.playerRadius,
		PlayerSpeed:            gs.playerSpeed,
		InitialPlayerHealth:    gs.initialPlayerHealth,
		MaxPlayerHealth:        gs.maxPlayerHealth,
		CoinRadius:             gs.coinRadius,
		MaxConcurrentCoins:     gs.maxConcurrentCoins,
		CoinScore:              gs.coinScore,
		MedpackRadius:          gs.medpackRadius,
		MaxConcurrentMedpacks:  gs.maxConcurrentMedpacks,
		MedpacksPerSquareMeter: gs.medpacksPerSquareMeter,
		MedpackScore:           gs.medpackScore,
		KeepAliveInterval:      gs.keepAliveInterval,
	}
}

var (
	Settings = GameSettings{
		seed: 0,

		gridWidth:  128,
		gridHeight: 128,

		maxPlayers:          100,
		playerRadius:        0.8,
		playerSpeed:         2.0,
		initialPlayerHealth: 100,
		maxPlayerHealth:     100,

		coinRadius:         0.5,
		maxConcurrentCoins: 100,
		coinScore:          1,

		medpackRadius:          0.5,
		maxConcurrentMedpacks:  10,
		medpacksPerSquareMeter: 0.025,
		medpackScore:           20,

		keepAliveInterval: 20 * 10,
	}
)
