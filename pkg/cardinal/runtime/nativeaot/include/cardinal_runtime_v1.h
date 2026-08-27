#ifndef CARDINAL_RUNTIME_V1_H
#define CARDINAL_RUNTIME_V1_H

#include <stddef.h>
#include <stdint.h>

#if defined(_WIN32)
#define CARDINAL_RUNTIME_EXPORT __declspec(dllexport)
#else
#define CARDINAL_RUNTIME_EXPORT __attribute__((visibility("default")))
#endif

#ifdef __cplusplus
extern "C" {
#endif

#define CARDINAL_RUNTIME_V1_ABI_VERSION UINT32_C(1)
#define CARDINAL_RUNTIME_V1_NAME_CAPACITY 64
#define CARDINAL_RUNTIME_V1_VERSION_CAPACITY 32
#define CARDINAL_RUNTIME_V1_SCHEMA_HASH_SIZE 32

typedef uint64_t cardinal_runtime_handle_v1;
typedef uint64_t cardinal_runtime_capabilities_v1;

enum cardinal_runtime_status_v1 {
    CARDINAL_RUNTIME_STATUS_SUCCESS = 0,
    CARDINAL_RUNTIME_STATUS_BUFFER_TOO_SMALL = 1,
    CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT = 2,
    CARDINAL_RUNTIME_STATUS_INVALID_HANDLE = 3,
    CARDINAL_RUNTIME_STATUS_INVALID_STATE = 4,
    CARDINAL_RUNTIME_STATUS_UNSUPPORTED = 5,
    CARDINAL_RUNTIME_STATUS_EXECUTION_FAILED = 6,
    CARDINAL_RUNTIME_STATUS_ABI_MISMATCH = 7,
};

enum cardinal_runtime_capability_v1 {
    CARDINAL_RUNTIME_CAPABILITY_INITIALIZE = UINT64_C(1) << 0,
    CARDINAL_RUNTIME_CAPABILITY_TICK = UINT64_C(1) << 1,
    CARDINAL_RUNTIME_CAPABILITY_QUERY = UINT64_C(1) << 2,
    CARDINAL_RUNTIME_CAPABILITY_SNAPSHOT = UINT64_C(1) << 3,
    CARDINAL_RUNTIME_CAPABILITY_RESTORE = UINT64_C(1) << 4,
    CARDINAL_RUNTIME_CAPABILITY_STATELESS = UINT64_C(1) << 5,
};

/*
 * Frozen v1 contract. Producers must set struct_size to exactly sizeof this struct, zero reserved
 * fields, and NUL-terminate UTF-8 name and version strings. schema_hash is an opaque 32-byte
 * compatibility fingerprint.
 */
typedef struct cardinal_runtime_contract_v1 {
    uint32_t abi_version;
    uint32_t struct_size;
    cardinal_runtime_capabilities_v1 capabilities;
    uint8_t schema_hash[CARDINAL_RUNTIME_V1_SCHEMA_HASH_SIZE];
    char name[CARDINAL_RUNTIME_V1_NAME_CAPACITY];
    char version[CARDINAL_RUNTIME_V1_VERSION_CAPACITY];
    uint64_t reserved[4];
} cardinal_runtime_contract_v1;

/*
 * Every input pointer is borrowed only for the duration of its call. Every output buffer is
 * caller-owned. On SUCCESS, output_len is bytes written. On BUFFER_TOO_SMALL, output_len is the
 * required capacity and the output buffer and module state must remain unmodified because the host
 * may retry the call. A zero capacity permits output=NULL.
 *
 * Handle zero is invalid. A module must serialize neither globally nor across handles; the host
 * serializes calls belonging to one handle. Errors raised before a handle exists are local to the
 * calling native thread; retrieve them immediately with last_error(0) on that same thread.
 */
CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_get_contract(
    cardinal_runtime_contract_v1 *contract
);

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_create(
    const uint8_t *config,
    size_t config_len,
    cardinal_runtime_handle_v1 *handle
);

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_initialize(
    cardinal_runtime_handle_v1 handle,
    const uint8_t *snapshot,
    size_t snapshot_len
);

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_tick(
    cardinal_runtime_handle_v1 handle,
    uint64_t tick,
    uint64_t fixed_delta_ns,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len
);

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_query(
    cardinal_runtime_handle_v1 handle,
    uint32_t kind,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len
);

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_snapshot(
    cardinal_runtime_handle_v1 handle,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len
);

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_restore(
    cardinal_runtime_handle_v1 handle,
    const uint8_t *snapshot,
    size_t snapshot_len
);

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_last_error(
    cardinal_runtime_handle_v1 handle,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len
);

/*
 * Destroy consumes the handle regardless of the returned status. Callers must not retry or use the
 * handle after this call.
 */
CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_destroy(
    cardinal_runtime_handle_v1 handle
);

#ifdef __cplusplus
}
#endif

#endif
