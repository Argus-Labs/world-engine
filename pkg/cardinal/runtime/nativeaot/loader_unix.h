#ifndef CARDINAL_NATIVEAOT_LOADER_UNIX_H
#define CARDINAL_NATIVEAOT_LOADER_UNIX_H

#include "include/cardinal_runtime_v1.h"

typedef struct cardinal_nativeaot_library_v1 cardinal_nativeaot_library_v1;

typedef struct cardinal_nativeaot_call_result_v1 {
    int32_t status;
    uint64_t output_len;
} cardinal_nativeaot_call_result_v1;

typedef struct cardinal_nativeaot_create_result_v1 {
    int32_t status;
    cardinal_runtime_handle_v1 handle;
} cardinal_nativeaot_create_result_v1;

cardinal_nativeaot_library_v1 *cardinal_nativeaot_library_open(
    const char *path,
    char *error,
    uint64_t error_capacity
);

/*
 * cardinal_nativeaot_library_forget frees only the loader dispatch table. It does not unload the
 * NativeAOT library. The process keeps the library loaded until the process exits.
 */
void cardinal_nativeaot_library_forget(cardinal_nativeaot_library_v1 *library);

int32_t cardinal_nativeaot_get_contract(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_contract_v1 *contract
);

cardinal_nativeaot_create_result_v1 cardinal_nativeaot_create(
    cardinal_nativeaot_library_v1 *library,
    const uint8_t *config,
    uint64_t config_len
);

int32_t cardinal_nativeaot_initialize(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle,
    const uint8_t *snapshot,
    uint64_t snapshot_len
);

cardinal_nativeaot_call_result_v1 cardinal_nativeaot_tick(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle,
    uint64_t tick,
    uint64_t fixed_delta_ns,
    const uint8_t *input,
    uint64_t input_len,
    uint8_t *output,
    uint64_t output_capacity
);

cardinal_nativeaot_call_result_v1 cardinal_nativeaot_query(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle,
    uint32_t kind,
    const uint8_t *input,
    uint64_t input_len,
    uint8_t *output,
    uint64_t output_capacity
);

cardinal_nativeaot_call_result_v1 cardinal_nativeaot_snapshot(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle,
    uint8_t *output,
    uint64_t output_capacity
);

int32_t cardinal_nativeaot_restore(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle,
    const uint8_t *snapshot,
    uint64_t snapshot_len
);

cardinal_nativeaot_call_result_v1 cardinal_nativeaot_last_error(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle,
    uint8_t *output
);

int32_t cardinal_nativeaot_destroy(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle
);

#endif
