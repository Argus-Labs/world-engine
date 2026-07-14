#ifndef PHYSICS2D_UINT32_MAP_H
#define PHYSICS2D_UINT32_MAP_H

// Open-addressed, linear-probing uint32 -> int map.
//
// Capacity is always a power of two (lazy-initialized to 64 on first insert,
// doubled at ~70% load). The empty sentinel for the key slot is 0xFFFFFFFF,
// which implies the caller must never use that value as a real key — in
// practice the bridge stores Cardinal ECS entity ids, which are allocated
// from 1 and never reach the max uint32.

#include <stdint.h>

#define U32_MAP_EMPTY 0xFFFFFFFFu

typedef struct {
    uint32_t key;
    int      value;
} U32MapSlot;

typedef struct {
    U32MapSlot* slots;
    int         cap;
    int         count;
} U32Map;

// Looks up key. Returns the stored value, or -1 if the key is absent.
int  u32map_find(const U32Map* m, uint32_t key);

// Inserts or overwrites key -> value. Lazily allocates storage on the first
// call and grows at ~70% load factor.
void u32map_insert(U32Map* m, uint32_t key, int value);

// Overwrites the value of an existing key in place. No-op if the key is
// absent. Never resizes — split from u32map_insert so callers that know the
// key is present (e.g. fixing up a moved entry after a swap-remove) cannot
// accidentally enter the resize branch mid-walk, where it would be safe
// only by arithmetic coincidence on the load factor.
void u32map_update(U32Map* m, uint32_t key, int value);

// Removes key. No-op if absent. Rehashes the remainder of the probe cluster
// so subsequent lookups still find their slots.
void u32map_remove(U32Map* m, uint32_t key);

// Clears all entries but keeps the existing backing storage.
void u32map_clear(U32Map* m);

// Frees backing storage and zeroes the struct.
void u32map_free(U32Map* m);

// Halves capacity when count < cap/4, with a minimum cap of 64. Call only
// at quiescent points (outside any probe-cluster walk), since it reallocates
// the backing array.
void u32map_maybe_shrink(U32Map* m);

#endif // PHYSICS2D_UINT32_MAP_H
