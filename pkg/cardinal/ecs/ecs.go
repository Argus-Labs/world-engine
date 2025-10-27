package ecs

import "github.com/rotisserie/eris"

// Create creates an entity without any components.
func Create(ws *worldState) EntityID {
	return ws.newEntity()
}

// Destroy deletes an entity and all its components from the world. Returns true if the entity is
// deleted, false otherwise.
func Destroy(ws *worldState, eid EntityID) bool {
	return ws.removeEntity(eid)
}

// Alive checks if an entity exists in the world.
func Alive(ws *worldState, eid EntityID) bool {
	_, exists := ws.entityArch.get(eid)
	return exists
}

// Set sets a component on an entity. If the entity contains the component type, it will update the
// value. If it doesn't, it will add the component.
func Set[T Component](ws *worldState, eid EntityID, component T) error {
	return setComponent(ws, eid, component)
}

// Get gets a component from an entity.
// Returns an error if the entity doesn't exist or doesn't contain the component type.
func Get[T Component](ws *worldState, eid EntityID) (T, error) {
	return getComponent[T](ws, eid)
}

// Remove removes a component from an entity.
// Returns an error if the entity or the component to remove doesn't exist.
func Remove[T Component](ws *worldState, eid EntityID) error {
	return removeComponent[T](ws, eid)
}

// Has checks if an entity has a specific component type.
// Returns false if either the entity doesn't exist or doesn't have the component.
func Has[T Component](ws *worldState, eid EntityID) bool {
	_, err := Get[T](ws, eid)
	if err == nil {
		return true
	}
	return eris.Is(err, ErrComponentNotFound)
}
