package cardinal

// Namespace is a unique identifier for a world.
type Namespace string

func (n Namespace) String() string {
	return string(n)
}
