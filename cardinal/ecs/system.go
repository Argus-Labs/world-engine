package ecs

type RegisteredSystem func(*World, *TransactionQueue) error
type System func(*World, *TransactionQueue, *Logger) error
