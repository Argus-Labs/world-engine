#ifndef CARDINAL_RUNTIME_V1_H
#define CARDINAL_RUNTIME_V1_H

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
#define CARDINAL_RUNTIME_V1_TYPE_CAPACITY 128
#define CARDINAL_RUNTIME_V1_LAST_ERROR_CAPACITY 1024

typedef uint64_t cardinal_runtime_handle_v1;

enum cardinal_runtime_status_v1 {
    CARDINAL_RUNTIME_STATUS_SUCCESS = 0,
    CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT = 2,
    CARDINAL_RUNTIME_STATUS_INVALID_HANDLE = 3,
    CARDINAL_RUNTIME_STATUS_INVALID_STATE = 4,
    CARDINAL_RUNTIME_STATUS_UNSUPPORTED = 5,
    CARDINAL_RUNTIME_STATUS_EXECUTION_FAILED = 6,
    CARDINAL_RUNTIME_STATUS_ABI_MISMATCH = 7,
};

/*
 * A producer must NUL-terminate every string. Values that do not fit must be rejected,
 * not truncated. The type fields contain protobuf full message names, not language-specific
 * class names. The host must validate the ABI version and all contract fields before create.
 */
typedef struct cardinal_runtime_contract_v1 {
    uint32_t abi_version;
    char name[CARDINAL_RUNTIME_V1_NAME_CAPACITY];
    char version[CARDINAL_RUNTIME_V1_VERSION_CAPACITY];
    char input_type[CARDINAL_RUNTIME_V1_TYPE_CAPACITY];
    char output_type[CARDINAL_RUNTIME_V1_TYPE_CAPACITY];
    char snapshot_type[CARDINAL_RUNTIME_V1_TYPE_CAPACITY];
} cardinal_runtime_contract_v1;

/*
 * A module borrows each input pointer for one call. The host owns the input buffers.
 *
 * For tick and snapshot, output and output_len must be non-NULL. On SUCCESS, the module sets
 * *output to its own read-only buffer and *output_len to the number of bytes in that buffer.
 * *output may be NULL only when *output_len is zero. The host must not free or modify this buffer.
 * The pointer is valid only until the next call on the same handle, including destroy. The host
 * must consume or copy the bytes before that call. On failure, the host must not consume output.
 *
 * Zero is not a valid handle. The host serializes calls for one handle. A module must permit
 * concurrent calls for different handles.
 *
 * Before a handle exists, the module stores each error for the current native thread. The host must
 * immediately call last_error(0) on the same native thread.
 */
CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_get_contract(
    cardinal_runtime_contract_v1 *contract
);

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_create(
    const uint8_t *config,
    uint64_t config_len,
    cardinal_runtime_handle_v1 *handle
);

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_initialize(
    cardinal_runtime_handle_v1 handle,
    const uint8_t *snapshot,
    uint64_t snapshot_len
);

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_tick(
    cardinal_runtime_handle_v1 handle,
    uint64_t tick,
    uint64_t fixed_delta_ns,
    const uint8_t *input,
    uint64_t input_len,
    const uint8_t **output,
    uint64_t *output_len
);

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_snapshot(
    cardinal_runtime_handle_v1 handle,
    const uint8_t **output,
    uint64_t *output_len
);

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_restore(
    cardinal_runtime_handle_v1 handle,
    const uint8_t *snapshot,
    uint64_t snapshot_len
);

/*
 * last_error copies at most output_capacity bytes of UTF-8 diagnostic text. If the full text does
 * not fit, last_error copies the longest valid UTF-8 prefix that fits. On SUCCESS, output_len is the
 * number of bytes that last_error writes. The host owns this output buffer and sets output_capacity
 * to CARDINAL_RUNTIME_V1_LAST_ERROR_CAPACITY.
 */
CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_last_error(
    cardinal_runtime_handle_v1 handle,
    uint8_t *output,
    uint64_t output_capacity,
    uint64_t *output_len
);

/*
 * Destroy consumes the handle even when Destroy returns an error. The caller must not retry this
 * call. The caller must not use the handle again.
 */
CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_destroy(
    cardinal_runtime_handle_v1 handle
);

#ifdef __cplusplus
}
#endif

#endif
