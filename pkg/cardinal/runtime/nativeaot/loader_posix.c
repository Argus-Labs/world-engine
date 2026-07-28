//go:build cgo && (linux || darwin)

#include "loader_unix.h"

#include <dlfcn.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef int32_t (*get_contract_fn)(cardinal_runtime_contract_v1 *);
typedef int32_t (*create_fn)(
    const uint8_t *,
    size_t,
    cardinal_runtime_handle_v1 *);
typedef int32_t (*initialize_fn)(
    cardinal_runtime_handle_v1,
    const uint8_t *,
    size_t);
typedef int32_t (*tick_fn)(
    cardinal_runtime_handle_v1,
    uint64_t,
    uint64_t,
    const uint8_t *,
    size_t,
    uint8_t *,
    size_t,
    size_t *);
typedef int32_t (*query_fn)(
    cardinal_runtime_handle_v1,
    uint32_t,
    const uint8_t *,
    size_t,
    uint8_t *,
    size_t,
    size_t *);
typedef int32_t (*snapshot_fn)(
    cardinal_runtime_handle_v1,
    uint8_t *,
    size_t,
    size_t *);
typedef int32_t (*restore_fn)(
    cardinal_runtime_handle_v1,
    const uint8_t *,
    size_t);
typedef int32_t (*last_error_fn)(
    cardinal_runtime_handle_v1,
    uint8_t *,
    size_t,
    size_t *);
typedef int32_t (*destroy_fn)(cardinal_runtime_handle_v1);

struct cardinal_nativeaot_library_v1 {
    get_contract_fn get_contract;
    create_fn create;
    initialize_fn initialize;
    tick_fn tick;
    query_fn query;
    snapshot_fn snapshot;
    restore_fn restore;
    last_error_fn last_error;
    destroy_fn destroy;
};

static void copy_error(char *output, size_t output_capacity, const char *message) {
    if (output == NULL || output_capacity == 0) {
        return;
    }
    if (message == NULL) {
        message = "unknown dynamic loader error";
    }
    (void)snprintf(output, output_capacity, "%s", message);
}

static void *load_symbol(
    void *dl_handle,
    const char *name,
    char *error,
    size_t error_capacity) {
    (void)dlerror();
    void *symbol = dlsym(dl_handle, name);
    const char *dl_error = dlerror();
    if (dl_error != NULL) {
        copy_error(error, error_capacity, dl_error);
        return NULL;
    }
    return symbol;
}

#define LOAD_SYMBOL(library, dl_handle, field, symbol_name, error, error_capacity) \
    do {                                                                            \
        void *symbol_pointer = load_symbol(                                         \
            (dl_handle), (symbol_name), (error), (error_capacity));                 \
        if (symbol_pointer == NULL) {                                               \
            free(library);                                                          \
            return NULL;                                                            \
        }                                                                           \
        memcpy(&(library)->field, &symbol_pointer, sizeof(symbol_pointer));         \
    } while (0)

cardinal_nativeaot_library_v1 *cardinal_nativeaot_library_open(
    const char *path,
    char *error,
    size_t error_capacity) {
    if (path == NULL || path[0] == '\0') {
        copy_error(error, error_capacity, "shared library path is empty");
        return NULL;
    }

    void *dl_handle = dlopen(path, RTLD_NOW | RTLD_LOCAL);
    if (dl_handle == NULL) {
        copy_error(error, error_capacity, dlerror());
        return NULL;
    }

    cardinal_nativeaot_library_v1 *library =
        calloc(1, sizeof(cardinal_nativeaot_library_v1));
    if (library == NULL) {
        /*
         * Do not dlclose even on setup failure. NativeAOT modules are
         * process-lifetime once loaded.
         */
        copy_error(error, error_capacity, "allocating loader dispatch table failed");
        return NULL;
    }

    LOAD_SYMBOL(
        library,
        dl_handle,
        get_contract,
        "cardinal_runtime_v1_get_contract",
        error,
        error_capacity);
    LOAD_SYMBOL(
        library,
        dl_handle,
        create,
        "cardinal_runtime_v1_create",
        error,
        error_capacity);
    LOAD_SYMBOL(
        library,
        dl_handle,
        initialize,
        "cardinal_runtime_v1_initialize",
        error,
        error_capacity);
    LOAD_SYMBOL(
        library,
        dl_handle,
        tick,
        "cardinal_runtime_v1_tick",
        error,
        error_capacity);
    LOAD_SYMBOL(
        library,
        dl_handle,
        query,
        "cardinal_runtime_v1_query",
        error,
        error_capacity);
    LOAD_SYMBOL(
        library,
        dl_handle,
        snapshot,
        "cardinal_runtime_v1_snapshot",
        error,
        error_capacity);
    LOAD_SYMBOL(
        library,
        dl_handle,
        restore,
        "cardinal_runtime_v1_restore",
        error,
        error_capacity);
    LOAD_SYMBOL(
        library,
        dl_handle,
        last_error,
        "cardinal_runtime_v1_last_error",
        error,
        error_capacity);
    LOAD_SYMBOL(
        library,
        dl_handle,
        destroy,
        "cardinal_runtime_v1_destroy",
        error,
        error_capacity);

    return library;
}

void cardinal_nativeaot_library_forget(cardinal_nativeaot_library_v1 *library) {
    /*
     * Deliberately no dlclose. NativeAOT does not support unloading. Only the
     * dispatch table allocated by this loader is released.
     */
    free(library);
}

int32_t cardinal_nativeaot_get_contract(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_contract_v1 *contract) {
    if (library == NULL || contract == NULL) {
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }
    return library->get_contract(contract);
}

cardinal_nativeaot_create_result_v1 cardinal_nativeaot_create(
    cardinal_nativeaot_library_v1 *library,
    const uint8_t *config,
    size_t config_len) {
    cardinal_nativeaot_create_result_v1 result = {
        .status = CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT,
        .handle = 0,
    };
    if (library == NULL) {
        return result;
    }
    result.status = library->create(config, config_len, &result.handle);
    return result;
}

int32_t cardinal_nativeaot_initialize(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle,
    const uint8_t *snapshot,
    size_t snapshot_len) {
    if (library == NULL) {
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }
    return library->initialize(handle, snapshot, snapshot_len);
}

cardinal_nativeaot_call_result_v1 cardinal_nativeaot_tick(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle,
    uint64_t tick,
    uint64_t fixed_delta_ns,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_capacity) {
    cardinal_nativeaot_call_result_v1 result = {
        .status = CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT,
        .output_len = 0,
    };
    if (library == NULL) {
        return result;
    }
    result.status = library->tick(
        handle,
        tick,
        fixed_delta_ns,
        input,
        input_len,
        output,
        output_capacity,
        &result.output_len);
    return result;
}

cardinal_nativeaot_call_result_v1 cardinal_nativeaot_query(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle,
    uint32_t kind,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_capacity) {
    cardinal_nativeaot_call_result_v1 result = {
        .status = CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT,
        .output_len = 0,
    };
    if (library == NULL) {
        return result;
    }
    result.status = library->query(
        handle,
        kind,
        input,
        input_len,
        output,
        output_capacity,
        &result.output_len);
    return result;
}

cardinal_nativeaot_call_result_v1 cardinal_nativeaot_snapshot(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle,
    uint8_t *output,
    size_t output_capacity) {
    cardinal_nativeaot_call_result_v1 result = {
        .status = CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT,
        .output_len = 0,
    };
    if (library == NULL) {
        return result;
    }
    result.status =
        library->snapshot(handle, output, output_capacity, &result.output_len);
    return result;
}

int32_t cardinal_nativeaot_restore(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle,
    const uint8_t *snapshot,
    size_t snapshot_len) {
    if (library == NULL) {
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }
    return library->restore(handle, snapshot, snapshot_len);
}

cardinal_nativeaot_call_result_v1 cardinal_nativeaot_last_error(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle,
    uint8_t *output,
    size_t output_capacity) {
    cardinal_nativeaot_call_result_v1 result = {
        .status = CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT,
        .output_len = 0,
    };
    if (library == NULL) {
        return result;
    }
    result.status =
        library->last_error(handle, output, output_capacity, &result.output_len);
    return result;
}

int32_t cardinal_nativeaot_destroy(
    cardinal_nativeaot_library_v1 *library,
    cardinal_runtime_handle_v1 handle) {
    if (library == NULL) {
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }
    return library->destroy(handle);
}
