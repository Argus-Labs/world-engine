package testutils

// Commands.

type AttackPlayerCommand struct{ Value int }

func (AttackPlayerCommand) Name() string { return "attack-player" }

type InvalidEmptyCommand struct{}

func (InvalidEmptyCommand) Name() string { return "" }

type CreatePlayerCommand struct{ Value int }

func (CreatePlayerCommand) Name() string { return "create-player" }

// Events.

type PlayerDeathEvent struct{ Value int }

func (PlayerDeathEvent) Name() string { return "player-death" }

type ItemDropEvent struct{ Value int }

func (ItemDropEvent) Name() string { return "item-drop" }

type EmptySubjectEvent struct{ Value int }

func (EmptySubjectEvent) Name() string { return "" }

// System events.

type PlayerDeathSystemEvent struct{ Value int }

func (PlayerDeathSystemEvent) Name() string { return "player-death-system" }

type ItemDropSystemEvent struct{ Value int }

func (ItemDropSystemEvent) Name() string { return "item-drop-system" }
