package ecs

type System func(*World, *TransactionQueue) error
