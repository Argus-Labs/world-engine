#include "uint32_map.h"

#include <stdlib.h>
#include <string.h>

// Fibonacci hashing. `cap` is always a power of two, so the mask is cap-1.
static uint32_t u32map_hash(uint32_t key, int cap) {
    uint32_t h = key * 2654435761u;
    return h & (uint32_t)(cap - 1);
}

static void u32map_init(U32Map* m, int cap) {
    m->cap = cap;
    m->count = 0;
    m->slots = (U32MapSlot*)malloc(sizeof(U32MapSlot) * (size_t)cap);
    for (int i = 0; i < cap; i++) {
        m->slots[i].key = U32_MAP_EMPTY;
        m->slots[i].value = -1;
    }
}

// Inserts without checking load factor or resizing. Used internally by
// u32map_insert's grow branch, by u32map_resize during rehash, and by
// u32map_remove when rehashing the tail of a probe cluster.
static void u32map_insert_no_resize(U32Map* m, uint32_t key, int value) {
    uint32_t idx = u32map_hash(key, m->cap);
    for (int i = 0; i < m->cap; i++) {
        uint32_t slot = (idx + (uint32_t)i) & (uint32_t)(m->cap - 1);
        if (m->slots[slot].key == U32_MAP_EMPTY || m->slots[slot].key == key) {
            m->slots[slot].key = key;
            m->slots[slot].value = value;
            return;
        }
    }
}

static void u32map_resize(U32Map* m, int new_cap) {
    U32MapSlot* old = m->slots;
    int old_cap = m->cap;
    m->cap = new_cap;
    m->slots = (U32MapSlot*)malloc(sizeof(U32MapSlot) * (size_t)new_cap);
    for (int i = 0; i < new_cap; i++) {
        m->slots[i].key = U32_MAP_EMPTY;
        m->slots[i].value = -1;
    }
    for (int i = 0; i < old_cap; i++) {
        if (old[i].key != U32_MAP_EMPTY) {
            u32map_insert_no_resize(m, old[i].key, old[i].value);
        }
    }
    free(old);
}

int u32map_find(const U32Map* m, uint32_t key) {
    if (m->cap == 0) return -1;
    uint32_t idx = u32map_hash(key, m->cap);
    for (int i = 0; i < m->cap; i++) {
        uint32_t slot = (idx + (uint32_t)i) & (uint32_t)(m->cap - 1);
        if (m->slots[slot].key == key) return m->slots[slot].value;
        if (m->slots[slot].key == U32_MAP_EMPTY) return -1;
    }
    return -1;
}

void u32map_insert(U32Map* m, uint32_t key, int value) {
    if (m->cap == 0) u32map_init(m, 64);
    // Resize at ~70% load.
    if (m->count * 10 >= m->cap * 7) {
        u32map_resize(m, m->cap * 2);
    }
    uint32_t idx = u32map_hash(key, m->cap);
    for (int i = 0; i < m->cap; i++) {
        uint32_t slot = (idx + (uint32_t)i) & (uint32_t)(m->cap - 1);
        if (m->slots[slot].key == key) {
            m->slots[slot].value = value;
            return;
        }
        if (m->slots[slot].key == U32_MAP_EMPTY) {
            m->slots[slot].key = key;
            m->slots[slot].value = value;
            m->count++;
            return;
        }
    }
}

void u32map_update(U32Map* m, uint32_t key, int value) {
    if (m->cap == 0) return;
    uint32_t idx = u32map_hash(key, m->cap);
    for (int i = 0; i < m->cap; i++) {
        uint32_t slot = (idx + (uint32_t)i) & (uint32_t)(m->cap - 1);
        if (m->slots[slot].key == key) {
            m->slots[slot].value = value;
            return;
        }
        if (m->slots[slot].key == U32_MAP_EMPTY) return;
    }
}

void u32map_remove(U32Map* m, uint32_t key) {
    if (m->cap == 0) return;
    uint32_t idx = u32map_hash(key, m->cap);
    for (int i = 0; i < m->cap; i++) {
        uint32_t slot = (idx + (uint32_t)i) & (uint32_t)(m->cap - 1);
        if (m->slots[slot].key == U32_MAP_EMPTY) return;
        if (m->slots[slot].key == key) {
            // Clear and rehash the rest of the cluster so probes stay
            // reachable. Use insert_no_resize: the resize branch of
            // u32map_insert would free m->slots out from under `next`.
            m->slots[slot].key = U32_MAP_EMPTY;
            m->slots[slot].value = -1;
            m->count--;
            uint32_t next = (slot + 1) & (uint32_t)(m->cap - 1);
            while (m->slots[next].key != U32_MAP_EMPTY) {
                uint32_t tmp_key = m->slots[next].key;
                int tmp_val = m->slots[next].value;
                m->slots[next].key = U32_MAP_EMPTY;
                m->slots[next].value = -1;
                u32map_insert_no_resize(m, tmp_key, tmp_val);
                next = (next + 1) & (uint32_t)(m->cap - 1);
            }
            return;
        }
    }
}

void u32map_clear(U32Map* m) {
    if (m->slots == NULL) return;
    for (int i = 0; i < m->cap; i++) {
        m->slots[i].key = U32_MAP_EMPTY;
        m->slots[i].value = -1;
    }
    m->count = 0;
}

void u32map_free(U32Map* m) {
    free(m->slots);
    m->slots = NULL;
    m->cap = 0;
    m->count = 0;
}

void u32map_maybe_shrink(U32Map* m) {
    if (m->cap > 64 && m->count * 4 < m->cap) {
        u32map_resize(m, m->cap / 2);
    }
}
