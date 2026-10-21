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
    uint64_t value;
    uint8_t output[11];
    unsigned output_mode;
    char error[FIXTURE_ERROR_CAPACITY];
} fixture_state;

static fixture_state states[FIXTURE_MAX_HANDLES];
static char global_error[FIXTURE_ERROR_CAPACITY];
static atomic_int active_probe_ticks;

static void set_error(char *destination, const char *message) {
    (void)snprintf(destination, FIXTURE_ERROR_CAPACITY, "%s", message);
}

static bool valid_input(const uint8_t *input, uint64_t input_len) {
    return input_len == 0 || input != NULL;
}

static fixture_state *find_state(cardinal_runtime_handle_v1 handle) {
    if (handle == 0 || handle > FIXTURE_MAX_HANDLES) {
        return NULL;
    }
    fixture_state *state = &states[handle - 1];
    return state->used ? state : NULL;
}

// This fixture accepts only an empty message or one field-1 int64 varint.
static bool parse_value(const uint8_t *input, uint64_t length, uint64_t *value) {
    *value = 0;
    if (length == 0) return true;
    if (input == NULL || length < 2 || length > 11 || input[0] != 0x08) return false;
    for (uint64_t index = 1; index < length; index++) {
        uint8_t byte = input[index];
        if (index == 10 && byte > 1) return false;
        *value |= (uint64_t)(byte & 0x7f) << ((index - 1) * 7);
        if ((byte & 0x80) == 0) return index == length - 1;
    }
    return false;
}

static int32_t write_output(
    fixture_state *state,
    const uint8_t **output,
    uint64_t *output_len
) {
    if (output == NULL || output_len == NULL) return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    uint64_t value = state->value;
    *output = state->output;
    *output_len = 0;
    if (value != 0) {
        state->output[(*output_len)++] = 0x08;
        do {
            uint8_t byte = value & 0x7f;
            value >>= 7;
            state->output[(*output_len)++] = byte | (value != 0 ? 0x80 : 0);
        } while (value != 0);
    }
    // Fault modes exercise the host's checks without dereferencing invalid memory.
    if (state->output_mode == 1) { *output = NULL; *output_len = 1; }
    if (state->output_mode == 2) { *output_len = UINT64_MAX; }
    if (state->output_mode == 3) { state->output[0] = 0x80; *output_len = 1; }
    state->error[0] = '\0';
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
    strcpy(contract->input_type, "worldengine.cardinal.fixture.v1.FixtureInput");
    strcpy(contract->output_type, "worldengine.cardinal.fixture.v1.FixtureOutput");
    strcpy(contract->snapshot_type, "worldengine.cardinal.fixture.v1.FixtureSnapshot");
#ifdef FIXTURE_UNTERMINATED_FIELD
    memset(contract->FIXTURE_UNTERMINATED_FIELD, 'x', sizeof(contract->FIXTURE_UNTERMINATED_FIELD));
#endif
    global_error[0] = '\0';
    return CARDINAL_RUNTIME_STATUS_SUCCESS;
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_create(
    const uint8_t *config,
    uint64_t config_len,
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
            if (config_len == 1) states[index].output_mode = config[0];
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
    uint64_t snapshot_len
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

    uint64_t value;
    if (!parse_value(snapshot, snapshot_len, &value)) {
        set_error(state->error, "fixture snapshot is malformed");
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }
    state->value = value;
    state->initialized = true;
    state->error[0] = '\0';
    return CARDINAL_RUNTIME_STATUS_SUCCESS;
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_tick(
    cardinal_runtime_handle_v1 handle,
    uint64_t tick,
    uint64_t fixed_delta_ns,
    const uint8_t *input,
    uint64_t input_len,
    const uint8_t **output,
    uint64_t *output_len
) {
    (void)fixed_delta_ns;
    fixture_state *state = find_state(handle);
    if (state == NULL) {
        set_error(global_error, "fixture handle is invalid");
        return CARDINAL_RUNTIME_STATUS_INVALID_HANDLE;
    }
    if (!state->initialized) {
        set_error(state->error, "fixture is not initialized");
        return CARDINAL_RUNTIME_STATUS_INVALID_STATE;
    }
    uint64_t increment;
    if (output == NULL || output_len == NULL || !parse_value(input, input_len, &increment)) {
        set_error(state->error, "fixture tick input is malformed");
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }

    if (tick == UINT64_MAX) {
        set_error(state->error, "fixture tick failure");
        return CARDINAL_RUNTIME_STATUS_EXECUTION_FAILED;
    }
    if (tick == 77) {
        int previous =
            atomic_fetch_add_explicit(&active_probe_ticks, 1, memory_order_acq_rel);
        if (previous != 0) {
            (void)atomic_fetch_sub_explicit(
                &active_probe_ticks,
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
            &active_probe_ticks,
            1,
            memory_order_acq_rel
        );
    }

    // Unsigned arithmetic preserves protobuf int64 two's-complement wraparound without C UB.
    state->value += increment;
    return write_output(state, output, output_len);
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_snapshot(
    cardinal_runtime_handle_v1 handle,
    const uint8_t **output,
    uint64_t *output_len
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

    return write_output(state, output, output_len);
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_restore(
    cardinal_runtime_handle_v1 handle,
    const uint8_t *snapshot,
    uint64_t snapshot_len
) {
    fixture_state *state = find_state(handle);
    if (state == NULL) {
        set_error(global_error, "fixture handle is invalid");
        return CARDINAL_RUNTIME_STATUS_INVALID_HANDLE;
    }
    uint64_t value;
    if (!parse_value(snapshot, snapshot_len, &value)) {
        set_error(state->error, "fixture snapshot is malformed");
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }

    state->value = value;
    state->initialized = true;
    state->error[0] = '\0';
    return CARDINAL_RUNTIME_STATUS_SUCCESS;
}

CARDINAL_RUNTIME_EXPORT int32_t cardinal_runtime_v1_last_error(
    cardinal_runtime_handle_v1 handle,
    uint8_t *output,
    uint64_t output_capacity,
    uint64_t *output_len
) {
    fixture_state *state = find_state(handle);
    const char *message = state == NULL ? global_error : state->error;
    if (output_len == NULL || (output_capacity > 0 && output == NULL)) {
        return CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT;
    }

    uint64_t written = strlen(message);
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

#ifdef FIXTURE_SELF_TEST
#include <assert.h>

int main(void) {
    cardinal_runtime_handle_v1 first, second;
    assert(cardinal_runtime_v1_create(NULL, 0, &first) == 0);
    assert(cardinal_runtime_v1_create(NULL, 0, &second) == 0);
    assert(cardinal_runtime_v1_initialize(first, NULL, 0) == 0);
    assert(cardinal_runtime_v1_initialize(second, NULL, 0) == 0);
    const uint8_t input[] = {8, 127};
    const uint8_t *before, *after, *other;
    uint64_t length;
    assert(cardinal_runtime_v1_tick(first, 1, 2, input, sizeof(input), &before, &length) == 0);
    assert(length == 2 && before[0] == 8 && before[1] == 127);
    assert(cardinal_runtime_v1_tick(first, 2, 2, input, sizeof(input), &after, &length) == 0);
    assert(before == after && length == 3 && after[1] == 254 && after[2] == 1);
    assert(cardinal_runtime_v1_snapshot(first, &after, &length) == 0 && before == after);
    assert(cardinal_runtime_v1_tick(second, 1, 2, input, sizeof(input), &other, &length) == 0);
    assert(other != before && other[1] == 127 && before[1] == 254);

    const uint8_t malformed[][12] = {
        {8}, {8, 128}, {16, 1}, {8, 1, 8, 2},
        {8, 255, 255, 255, 255, 255, 255, 255, 255, 255, 2},
        {8, 128, 128, 128, 128, 128, 128, 128, 128, 128, 128, 0}
    };
    const uint64_t lengths[] = {1, 2, 2, 4, 11, 12};
    for (size_t index = 0; index < sizeof(lengths) / sizeof(lengths[0]); index++) {
        assert(cardinal_runtime_v1_tick(first, 1, 2, malformed[index], lengths[index],
            &after, &length) == CARDINAL_RUNTIME_STATUS_INVALID_ARGUMENT);
    }
    assert(cardinal_runtime_v1_snapshot(first, &after, &length) == 0);
    assert(length == 3 && after[1] == 254 && after[2] == 1);
    assert(cardinal_runtime_v1_destroy(first) == 0);
    assert(cardinal_runtime_v1_destroy(second) == 0);
    return 0;
}
#endif
