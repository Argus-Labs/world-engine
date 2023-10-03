package public

type ArchetypeID int

type IArchtypeIterator interface {
	HasNext() bool
	Next() ArchetypeID
	GetCurrent() int
	GetValues() []ArchetypeID
}
