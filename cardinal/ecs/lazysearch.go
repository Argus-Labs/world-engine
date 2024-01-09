package ecs

import "pkg.world.dev/world-engine/cardinal/types/entity"

// LazySearch stores a LazyContainer of Search. It essentially delays the error created by instantiating search
// so that the error happens on the method calls of Each, Count, First and MustFirst
type LazySearch struct {
	Container LazyContainer[*Search]
}

func (q *LazySearch) Each(eCtx EngineContext, callback SearchCallBackFn) error {
	query, err := q.Container.Unbox()
	if err != nil {
		return err
	}
	return query.Each(eCtx, callback)
}

func (q *LazySearch) Count(eCtx EngineContext) (int, error) {
	query, err := q.Container.Unbox()
	if err != nil {
		return 0, err
	}
	return query.Count(eCtx)
}

func (q *LazySearch) First(eCtx EngineContext) (id entity.ID, err error) {
	query, err := q.Container.Unbox()
	if err != nil {
		return 0, err
	}
	return query.First(eCtx)
}

func (q *LazySearch) MustFirst(eCtx EngineContext) entity.ID {
	query, err := q.Container.Unbox()
	if err != nil {
		panic("error building query")
	}
	return query.MustFirst(eCtx)
}

func NewLazySearch(callback func() (*Search, error)) *LazySearch {
	return &LazySearch{Container: NewLazyContainer(callback)}
}
