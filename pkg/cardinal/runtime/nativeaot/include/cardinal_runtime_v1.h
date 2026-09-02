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
#define CARDINAL_RUNTIME_V1_LAST_ERROR_CAPACITY 1024

typedef uint64_t cardinal_runtime_handle_v1;

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

/*
 * A producer must NUL-terminate each UTF-8 name and version string.
 */
typedef struct cardinal_runtime_contract_v1 {
    uint32_t abi_version;
    char name[CARDINAL_RUNTIME_V1_NAME_CAPACITY];
    char version[CARDINAL_RUNTIME_V1_VERSION_CAPACITY];
} cardinal_runtime_contract_v1;

/*
 * A module borrows each input pointer for one call. The caller owns each output buffer.
 *
 * For tick, query, and snapshot, the module sets output_len to the number of bytes that it writes.
 * If the output buffer is too small, the module sets output_len to the required capacity. It then
 * returns BUFFER_TOO_SMALL. A call that returns BUFFER_TOO_SMALL must not change the module state or
 * the output buffer. The host can retry the call. Output can be NULL only when the output capacity
 * is zero.
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


/*
 * TODO: Get rid of buffer too small error and fix these ugly output_capacity params.
 */

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

/*
 * last_error copies at most output_capacity bytes of UTF-8 diagnostic text. If the full text does
 * not fit, last_error copies the longest valid UTF-8 prefix that fits. On SUCCESS, output_len is the
 * number of bytes that last_error writes. last_error must not return BUFFER_TOO_SMALL. The host sets
 * output_capacity to CARDINAL_RUNTIME_V1_LAST_ERROR_CAPACITY.
 */
CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_last_error(
    cardinal_runtime_handle_v1 handle,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len
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
