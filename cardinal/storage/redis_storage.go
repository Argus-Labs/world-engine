package storage

//
//import (
//	"github.com/redis/go-redis/v9"
//
//	"github.com/argus-labs/cardinal/component"
//)
//
//type redisStorage struct {
//	c *redis.Client
//}
//
//var _ ComponentStorageManager = redisStorage{}
//
//func NewRedisStorage(c *redis.Client) ComponentStorageManager {
//	return &redisStorage{c: c}
//}
//
//func (r redisStorage) PushComponent(component component.IComponentType, index ArchetypeIndex) error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (r redisStorage) ComponentIndex(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) []byte {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (r redisStorage) SetComponent(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex, compBz []byte) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (r redisStorage) MoveComponent(source ArchetypeIndex, index ComponentIndex, dst ArchetypeIndex) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (r redisStorage) SwapRemove(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) []byte {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (r redisStorage) Contains(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) bool {
//	//TODO implement me
//	panic("implement me")
//}
