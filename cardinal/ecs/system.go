package ecs

type registeredSystem func(*World, *TransactionQueue) error
type System func(*World, *TransactionQueue, *Logger) error
