#define _POSIX_C_SOURCE 200809L

#include "cardinal_runtime_v1.h"

#include <stdbool.h>
#include <stdatomic.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <time.h>

#define FIXTURE_MAX_HANDLES 32
#define FIXTURE_ERROR_CAPACITY (CARDINAL_RUNTIME_V1_LAST_ERROR_CAPACITY + 2)

typedef struct fixture_state {
    bool used;
    bool initialized;
    uint64_t tick;
    uint64_t fixed_delta_ns;
    uint64_t config_hash;
    char error[FIXTURE_ERROR_CAPACITY];
} fixture_state;

static fixture_state states[FIXTURE_MAX_HANDLES];
static char global_error[FIXTURE_ERROR_CAPACITY];
static atomic_int active_probe_queries;

static void set_error(char *destination, const char *message) {
    (void)snprintf(destination, FIXTURE_ERROR_CAPACITY, "%s", message);
}

static bool valid_input(const uint8_t *input, size_t input_len) {
    return input_len == 0 || input != NULL;
}

static fixture_state *find_state(cardinal_runtime_handle_v1 handle) {
    if (handle == 0 || handle > FIXTURE_MAX_HANDLES) {
        return NULL;
    }
    fixture_state *state = &states[handle - 1];
    return state->used ? state : NULL;
}

static uint64_t hash_bytes(const uint8_t *data, size_t data_len) {
    uint64_t hash = UINT64_C(1469598103934665603);
    for (size_t index = 0; index < data_len; index++) {
        hash ^= data[index];
        hash *= UINT64_C(1099511628211);
    }
    return hash;
}

static void write_uint32_le(uint8_t *output, uint32_t value) {
    for (size_t index = 0; index < 4; index++) {
        output[index] = (uint8_t)(value >> (index * 8));
    }
}

static void write_uint64_le(uint8_t *output, uint64_t value) {
    for (size_t index = 0; index < 8; index++) {
        output[index] = (uint8_t)(value >> (index * 8));
    }
}

static uint64_t read_uint64_le(const uint8_t *input) {
    uint64_t value = 0;
    for (size_t index = 0; index < 8; index++) {
        value |= (uint64_t)input[index] << (index * 8);
    }
    return value;
}

static int32_t prepare_output(
    uint8_t *output,
    size_t output_capacity,
    size_t required,
    size_t *output_len
) {
    if (output_len == NULL) {
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }
    *output_len = required;
    if (output_capacity < required) {
        return CARDINAL_RUNTIME_STATUS_BUFFER_TOO_SMALL;
    }
    if (required > 0 && output == NULL) {
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }
    return CARDINAL_RUNTIME_STATUS_SUCCESS;
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_get_contract(
    cardinal_runtime_contract_v1 *contract
) {
    if (contract == NULL) {
        set_error(global_error, "contract output is null");
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }

    memset(contract, 0, sizeof(*contract));
#ifdef FIXTURE_BAD_ABI
    contract->abi_version = CARDINAL_RUNTIME_V1_ABI_VERSION + 1;
#else
    contract->abi_version = CARDINAL_RUNTIME_V1_ABI_VERSION;
#endif
    (void)snprintf(contract->name, sizeof(contract->name), "%s", "nativeaot-fixture");
    (void)snprintf(contract->version, sizeof(contract->version), "%s", "1.2.3");
    global_error[0] = '\0';
    return CARDINAL_RUNTIME_STATUS_SUCCESS;
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_create(
    const uint8_t *config,
    size_t config_len,
    cardinal_runtime_handle_v1 *handle
) {
    if (handle == NULL || !valid_input(config, config_len)) {
        set_error(global_error, "invalid create arguments");
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }
    if (config_len == strlen("fail-create") &&
        memcmp(config, "fail-create", config_len) == 0) {
        set_error(global_error, "fixture create failure");
        return CARDINAL_RUNTIME_STATUS_EXECUTION_FAILED;
    }
    if (config_len == strlen("fail-create-long") &&
        memcmp(config, "fail-create-long", config_len) == 0) {
        memset(global_error, 'x', sizeof(global_error) - 1);
        global_error[sizeof(global_error) - 1] = '\0';
        return CARDINAL_RUNTIME_STATUS_EXECUTION_FAILED;
    }

    for (size_t index = 0; index < FIXTURE_MAX_HANDLES; index++) {
        if (!states[index].used) {
            memset(&states[index], 0, sizeof(states[index]));
            states[index].used = true;
            states[index].config_hash = hash_bytes(config, config_len);
            *handle = index + 1;
            global_error[0] = '\0';
            return CARDINAL_RUNTIME_STATUS_SUCCESS;
        }
    }

    set_error(global_error, "fixture handle capacity reached");
    return CARDINAL_RUNTIME_STATUS_EXECUTION_FAILED;
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_initialize(
    cardinal_runtime_handle_v1 handle,
    const uint8_t *snapshot,
    size_t snapshot_len
) {
    fixture_state *state = find_state(handle);
    if (state == NULL) {
        set_error(global_error, "fixture handle is invalid");
        return CARDINAL_RUNTIME_STATUS_INVALID_HANDLE;
    }
    if (!valid_input(snapshot, snapshot_len)) {
        set_error(state->error, "snapshot pointer is null");
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }
    if (state->initialized) {
        set_error(state->error, "fixture is already initialized");
        return CARDINAL_RUNTIME_STATUS_INVALID_STATE;
    }

    if (snapshot_len == 16) {
        state->tick = read_uint64_le(snapshot);
        state->fixed_delta_ns = read_uint64_le(snapshot + 8);
    }
    state->initialized = true;
    state->error[0] = '\0';
    return CARDINAL_RUNTIME_STATUS_SUCCESS;
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_tick(
    cardinal_runtime_handle_v1 handle,
    uint64_t tick,
    uint64_t fixed_delta_ns,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len
) {
    fixture_state *state = find_state(handle);
    if (state == NULL) {
        set_error(global_error, "fixture handle is invalid");
        return CARDINAL_RUNTIME_STATUS_INVALID_HANDLE;
    }
    if (!state->initialized) {
        set_error(state->error, "fixture is not initialized");
        return CARDINAL_RUNTIME_STATUS_INVALID_STATE;
    }
    if (!valid_input(input, input_len)) {
        set_error(state->error, "tick input pointer is null");
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }

    size_t required = 16 + input_len;
    int32_t status =
        prepare_output(output, output_capacity, required, output_len);
    if (status != CARDINAL_RUNTIME_STATUS_SUCCESS) {
        return status;
    }

    state->tick = tick;
    state->fixed_delta_ns = fixed_delta_ns;
    write_uint64_le(output, tick);
    write_uint64_le(output + 8, fixed_delta_ns);
    if (input_len > 0) {
        memcpy(output + 16, input, input_len);
    }
    state->error[0] = '\0';
    return CARDINAL_RUNTIME_STATUS_SUCCESS;
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_query(
    cardinal_runtime_handle_v1 handle,
    uint32_t kind,
    const uint8_t *input,
    size_t input_len,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len
) {
    fixture_state *state = find_state(handle);
    if (state == NULL) {
        set_error(global_error, "fixture handle is invalid");
        return CARDINAL_RUNTIME_STATUS_INVALID_HANDLE;
    }
    if (!state->initialized) {
        set_error(state->error, "fixture is not initialized");
        return CARDINAL_RUNTIME_STATUS_INVALID_STATE;
    }
    if (!valid_input(input, input_len)) {
        set_error(state->error, "query input pointer is null");
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }
    if (kind == UINT32_MAX) {
        set_error(state->error, "fixture query failure");
        return CARDINAL_RUNTIME_STATUS_EXECUTION_FAILED;
    }
    if (kind == 77) {
        int previous =
            atomic_fetch_add_explicit(&active_probe_queries, 1, memory_order_acq_rel);
        if (previous != 0) {
            (void)atomic_fetch_sub_explicit(
                &active_probe_queries,
                1,
                memory_order_acq_rel
            );
            set_error(state->error, "fixture observed concurrent calls");
            return CARDINAL_RUNTIME_STATUS_EXECUTION_FAILED;
        }
        const struct timespec delay = {
            .tv_sec = 0,
            .tv_nsec = 2 * 1000 * 1000,
        };
        (void)nanosleep(&delay, NULL);
        (void)atomic_fetch_sub_explicit(
            &active_probe_queries,
            1,
            memory_order_acq_rel
        );
    }

    size_t required = 4 + input_len;
    int32_t status =
        prepare_output(output, output_capacity, required, output_len);
    if (status != CARDINAL_RUNTIME_STATUS_SUCCESS) {
        return status;
    }

    write_uint32_le(output, kind);
    if (input_len > 0) {
        memcpy(output + 4, input, input_len);
    }
    state->error[0] = '\0';
    return CARDINAL_RUNTIME_STATUS_SUCCESS;
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_snapshot(
    cardinal_runtime_handle_v1 handle,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len
) {
    fixture_state *state = find_state(handle);
    if (state == NULL) {
        set_error(global_error, "fixture handle is invalid");
        return CARDINAL_RUNTIME_STATUS_INVALID_HANDLE;
    }
    if (!state->initialized) {
        set_error(state->error, "fixture is not initialized");
        return CARDINAL_RUNTIME_STATUS_INVALID_STATE;
    }

    int32_t status = prepare_output(output, output_capacity, 16, output_len);
    if (status != CARDINAL_RUNTIME_STATUS_SUCCESS) {
        return status;
    }
    write_uint64_le(output, state->tick);
    write_uint64_le(output + 8, state->fixed_delta_ns);
    state->error[0] = '\0';
    return CARDINAL_RUNTIME_STATUS_SUCCESS;
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_restore(
    cardinal_runtime_handle_v1 handle,
    const uint8_t *snapshot,
    size_t snapshot_len
) {
    fixture_state *state = find_state(handle);
    if (state == NULL) {
        set_error(global_error, "fixture handle is invalid");
        return CARDINAL_RUNTIME_STATUS_INVALID_HANDLE;
    }
    if (!valid_input(snapshot, snapshot_len) || snapshot_len != 16) {
        set_error(state->error, "fixture snapshot must be 16 bytes");
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }

    state->tick = read_uint64_le(snapshot);
    state->fixed_delta_ns = read_uint64_le(snapshot + 8);
    state->initialized = true;
    state->error[0] = '\0';
    return CARDINAL_RUNTIME_STATUS_SUCCESS;
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_last_error(
    cardinal_runtime_handle_v1 handle,
    uint8_t *output,
    size_t output_capacity,
    size_t *output_len
) {
    fixture_state *state = find_state(handle);
    const char *message = state == NULL ? global_error : state->error;
    if (output_len == NULL || (output_capacity > 0 && output == NULL)) {
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }

    size_t written = strlen(message);
    if (written > output_capacity) {
        written = output_capacity;
    }
    *output_len = written;
    if (written > 0) {
        memcpy(output, message, written);
    }
    return CARDINAL_RUNTIME_STATUS_SUCCESS;
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_destroy(
    cardinal_runtime_handle_v1 handle
) {
    fixture_state *state = find_state(handle);
    if (state == NULL) {
        set_error(global_error, "fixture handle is invalid");
        return CARDINAL_RUNTIME_STATUS_INVALID_HANDLE;
    }
    memset(state, 0, sizeof(*state));
    global_error[0] = '\0';
    return CARDINAL_RUNTIME_STATUS_SUCCESS;
}
