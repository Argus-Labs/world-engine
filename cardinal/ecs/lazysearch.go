package ecs

import "pkg.world.dev/world-engine/cardinal/types/entity"

type LazySearch struct {
	Container LazyContainer[*Search]
}

func (q *LazySearch) Each(wCtx WorldContext, callback SearchCallBackFn) error {
	query, err := q.Container.Unbox()
	if err != nil {
		return err
	}
	return query.Each(wCtx, callback)
}

func (q *LazySearch) Count(wCtx WorldContext) (int, error) {
	query, err := q.Container.Unbox()
	if err != nil {
		return 0, err
	}
	return query.Count(wCtx)
}

func (q *LazySearch) First(wCtx WorldContext) (id entity.ID, err error) {
	query, err := q.Container.Unbox()
	if err != nil {
		return 0, err
	}
	return query.First(wCtx)
}

func (q *LazySearch) MustFirst(wCtx WorldContext) entity.ID {
	query, err := q.Container.Unbox()
	if err != nil {
		panic("error building query")
	}
	return query.MustFirst(wCtx)
}

func NewLazySearch(callback func() (*Search, error)) *LazySearch {
	return &LazySearch{Container: NewLazyContainer(callback)}
}
