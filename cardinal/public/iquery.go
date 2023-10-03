package public

type IQuery interface {
	Each(w IWorld, callback QueryCallBackFn)
	Count(w IWorld) int
	First(w IWorld) (id EntityID, err error)
}

type QueryCallBackFn func(EntityID) bool
