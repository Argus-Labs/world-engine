package ecs

type System func(*World, *TransactionQueue, *Logger) error
