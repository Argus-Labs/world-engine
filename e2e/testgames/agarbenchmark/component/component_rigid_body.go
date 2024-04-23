package component

type RigidBody struct {
	IsStatic bool
	IsSensor bool
}

func (RigidBody) Name() string {
	return "RigidBody"
}
