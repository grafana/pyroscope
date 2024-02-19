#ifndef PYPERF_H
#define PYPERF_H

#include "vmlinux.h"
#include "bpf_helpers.h"

#include "pthread.bpf.h"
#include "pid.h"
#include "stacks.h"
#include "pystr.h"
#include "pyoffsets.h"
#include "hash.h"

#define PYTHON_STACK_FRAMES_PER_PROG 32
#define PYTHON_STACK_PROG_CNT 3
#define PYTHON_STACK_MAX_LEN (PYTHON_STACK_FRAMES_PER_PROG * PYTHON_STACK_PROG_CNT)
#define PYTHON_CLASS_NAME_LEN 32
#define PYTHON_FUNCTION_NAME_LEN 64
#define PYTHON_FILE_NAME_LEN 128

enum {
    PY_ERROR_GENERIC = 1,
    PY_ERROR_THREAD_STATE = 2,
    PY_ERROR_THREAD_STATE_NULL = 3,
    PY_ERROR_TOP_FRAME = 4,
    PY_ERROR_FRAME_CODE = 5,
    PY_ERROR_FRAME_PREV = 6,
    PY_ERROR_SYMBOL = 7,
    PY_ERROR_TLSBASE = 8,
    PY_ERROR_FIRST_ARG = 9,
    PY_ERROR_CLASS_NAME = 10,
    PY_ERROR_FILE_NAME = 11,
    PY_ERROR_NAME = 12,
    PY_ERROR_FRAME_OWNER = 13,
    PY_ERROR_FRAME_OWNER_INVALID = 14,


};

struct global_config_t {
    uint8_t bpf_log_err;
    uint8_t bpf_log_debug;
    uint64_t ns_pid_ino;
};

const volatile struct global_config_t global_config;
#define log_error(fmt, ...) if (global_config.bpf_log_err) bpf_printk(fmt, ##__VA_ARGS__)
#define log_debug(fmt, ...) if (global_config.bpf_log_debug) bpf_printk(fmt, ##__VA_ARGS__)

typedef struct {
    uint32_t major;
    uint32_t minor;
    uint32_t patch;
} py_version;

typedef struct {
    py_offset_config offsets;
    py_version version;
    struct libc libc;
    int32_t tssKey;
    uint8_t collect_kernel;
} py_pid_data;

typedef struct {
    char classname[PYTHON_CLASS_NAME_LEN];
    char name[PYTHON_FUNCTION_NAME_LEN];
    char file[PYTHON_FILE_NAME_LEN];

    struct py_str_type classname_type;
    struct py_str_type name_type;
    struct py_str_type file_type;
    struct py_str_type __padding;

    // NOTE: PyFrameObject also has line number but it is typically just the
    // first line of that function and PyCode_Addr2Line needs to be called
    // to get the actual line
} py_symbol;


typedef uint32_t py_symbol_id;

typedef struct {
    struct sample_key k;
    uint32_t stack_len;
    // instead of storing symbol name here directly, we add it to another
    // hashmap with Symbols and only store the ids here
    py_symbol_id stack[PYTHON_STACK_MAX_LEN];
} py_event;

#define _STR_CONCAT(str1, str2) str1##str2
#define STR_CONCAT(str1, str2) _STR_CONCAT(str1, str2)
#define FAIL_COMPILATION_IF(condition)            \
  typedef struct {                                \
    char _condition_check[1 - 2 * !!(condition)]; \
  } STR_CONCAT(compile_time_condition_check, __COUNTER__);
// See comments in get_frame_data
FAIL_COMPILATION_IF(sizeof(py_symbol) == sizeof(struct bpf_perf_event_value))
FAIL_COMPILATION_IF(HASH_LIMIT != PYTHON_STACK_MAX_LEN * sizeof(py_symbol_id))

typedef struct {
    int64_t symbol_counter;
    py_offset_config offsets;
    uint32_t cur_cpu;
    uint64_t frame_ptr;
    int64_t python_stack_prog_call_cnt;
    py_symbol sym;
    py_event event;
    uint64_t padding;// satisfy verifier for hash function
} py_sample_state_t;

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(key_size, sizeof(u32));
    __uint(value_size, PYTHON_STACK_MAX_LEN * sizeof(py_symbol_id));
    __uint(max_entries, PROFILE_MAPS_SIZE);
} python_stacks SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __type(key, u32);
    __type(value, py_sample_state_t);
    __uint(max_entries, 1);
} py_state_heap SEC(".maps");


struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, py_symbol);
    __type(value, py_symbol_id);
    __uint(max_entries, 16384);
} py_symbols SEC(".maps");


struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, pid_t);
    __type(value, py_pid_data);
    __uint(max_entries, 10240);
} py_pid_config SEC(".maps");

#define PYTHON_PROG_IDX_READ_PYTHON_STACK 0

int read_python_stack(struct bpf_perf_event_data *ctx);


struct {
    __uint(type, BPF_MAP_TYPE_PROG_ARRAY);
    __uint(max_entries, 2);
    __type(key, int);
    __array(values, int (void *));
} py_progs SEC(".maps") = {
        .values = {
                [PYTHON_PROG_IDX_READ_PYTHON_STACK] = (void *) &read_python_stack,
        },
};


static __always_inline int get_thread_state(
        py_pid_data *pid_data,
        void **out_thread_state) {
    return pyro_pthread_getspecific(&pid_data->libc, pid_data->tssKey, out_thread_state);
}

static __always_inline int submit_sample(
    py_sample_state_t* state) {
    uint32_t one = 1;
    if (state->event.stack_len < PYTHON_STACK_MAX_LEN) {
        state->event.stack[state->event.stack_len] = 0;
    }
    u64 h = MurmurHash64A(&state->event.stack, state->event.stack_len * sizeof(state->event.stack[0]), 0);
    state->event.k.user_stack = h;
    if (bpf_map_update_elem(&python_stacks, &h, &state->event.stack, BPF_ANY)) {
        return -1;
    }
    uint32_t* val = bpf_map_lookup_elem(&counts, &state->event.k);
    if (val) {
        (*val)++;
    }
    else {
        bpf_map_update_elem(&counts, &state->event.k, &one, BPF_NOEXIST);
    }


    return 0;
}

static __always_inline int submit_error_sample(
    uint8_t err) { //todo replace with more useful log
    log_error("pyperf_err: %d\n", err);
    return -1;
}

// this function is trivial, but we need to do map lookup in separate function,
// because BCC doesn't allow direct map calls (including lookups) from inside
// a macro (which we want to do in GET_STATE() macro below)
static __always_inline py_sample_state_t *get_state() {
    int zero = 0;
    return bpf_map_lookup_elem(&py_state_heap, &zero);
}

#define GET_STATE()                     \
  py_sample_state_t* state = get_state();  \
  if (!state) {                         \
    return -1; /* should never happen */ \
  }

static __always_inline int get_top_frame(py_pid_data *pid_data, py_sample_state_t *state, void *thread_state) {
    if (pid_data->offsets.PyThreadState_frame == -1) {
        // >= py311 && <= py312
        void *cframe;
        if (bpf_probe_read_user(
                &cframe,
                sizeof(void *),
                thread_state + pid_data->offsets.PyThreadState_cframe)) {
            return -1;
        }
        if (cframe == 0) {
            return -1;
        }
        if (bpf_probe_read_user(
                &state->frame_ptr,
                sizeof(void *),
                cframe + pid_data->offsets.PyCFrame_current_frame)) {
            return -1;
        }
        return 0;
    }
    // < py311 && >= py313
    if (bpf_probe_read_user(
            &state->frame_ptr,
            sizeof(void *),
            thread_state + pid_data->offsets.PyThreadState_frame)) {
        return -1;
    }
    return 0;
}

static __always_inline int pyperf_collect_impl(struct bpf_perf_event_data* ctx, pid_t pid) {
    py_pid_data *pid_data = bpf_map_lookup_elem(&py_pid_config, &pid);
    if (!pid_data) {
        return 0;
    }

    GET_STATE();

    state->offsets = pid_data->offsets;
    state->cur_cpu = bpf_get_smp_processor_id();
    state->python_stack_prog_call_cnt = 0;

    py_event *event = &state->event;
    event->k.pid = pid;
    if (pid_data->collect_kernel) {
        event->k.kern_stack = bpf_get_stackid(ctx, &stacks, KERN_STACKID_FLAGS);
    } else {
        event->k.kern_stack = -1;
    }


    // Read PyThreadState of this Thread from TLS
    void *thread_state;
    if (get_thread_state(pid_data, &thread_state)) {
        return submit_error_sample(PY_ERROR_THREAD_STATE);
    }

    // pre-initialize event struct in case any subprogram below fails
    event->stack_len = 0;

    if (thread_state != 0) {
        if (get_top_frame(pid_data, state, thread_state)) {
            return submit_error_sample(PY_ERROR_TOP_FRAME);
        }
        // jump to reading first set of Python frames
        bpf_tail_call(ctx, &py_progs, PYTHON_PROG_IDX_READ_PYTHON_STACK);
        // we won't ever get here
    }
    return submit_error_sample(PY_ERROR_THREAD_STATE_NULL);
}

SEC("perf_event")
int pyperf_collect(struct bpf_perf_event_data *ctx) {
    u32 pid;
    current_pid(global_config.ns_pid_ino, &pid);
    if (pid == 0) {
        return 0;
    }
    return pyperf_collect_impl(ctx, (pid_t) pid);
}


static __always_inline int check_first_arg(void *code_ptr,
                                           py_offset_config *offsets,
                                           py_symbol *symbol,
                                           bool *out_first_self,
                                           bool *out_first_cls) {
    // Figure out if we want to parse class name, basically checking the name of
    // the first argument,
    //   ((PyTupleObject*)$frame->f_code->co_varnames)->ob_item[0]
    // If it's 'self', we get the type and it's name, if it's cls, we just get
    // the name. This is not perfect but there is no better way to figure this
    // out from the code object.
    void *args_ptr;
    uint64_t args_size;
    if (offsets->PyCodeObject_co_varnames == -1) {
        if (bpf_probe_read_user(
                &args_ptr, sizeof(void *), code_ptr + offsets->PyCodeObject_co_localsplusnames)) {
            return -1;
        }
    } else {
        if (bpf_probe_read_user(
                &args_ptr, sizeof(void *), code_ptr + offsets->PyCodeObject_co_varnames)) {
            return -1;
        }
    }
    if (args_ptr == 0) {
        *out_first_self = false;
        *out_first_cls = false;
        return 0;
    }
    if (bpf_probe_read_user(
            &args_size, sizeof(args_size), args_ptr + offsets->PyVarObject_ob_size)) {
        return -1;
    }
    if (args_size < 1) {
        *out_first_self = false;
        *out_first_cls = false;
        return 0;
    }
    if (bpf_probe_read_user(
            &args_ptr, sizeof(void *), args_ptr + offsets->PyTupleObject_ob_item)) {
        return -1;
    }
    if (pystr_read(args_ptr, offsets, symbol->name, sizeof(symbol->name), &symbol->name_type)) {
        return -1;
    }
    // compare strings as ints to save instructions
    char self_str[4] = {'s', 'e', 'l', 'f'};
    char cls_str[4] = {'c', 'l', 's', '\0'};
    *out_first_self = *(int32_t *) symbol->name == *(int32_t *) self_str;
    *out_first_cls = *(int32_t *) symbol->name == *(int32_t *) cls_str;

    return 0;
}

// return -PY_ERR_XX on error, 0 on success
static __always_inline int get_names(
        void *cur_frame,
        void *code_ptr,
        py_offset_config *offsets,
        py_symbol *symbol,
        void *ctx) {

    bool first_self;
    bool first_cls;
    if (check_first_arg(code_ptr, offsets, symbol, &first_self, &first_cls)) {
        return -PY_ERROR_FIRST_ARG;
    }

    // We re-use the same py_symbol instance across loop iterations, which means
    // we will have left-over data in the struct. Although this won't affect
    // correctness of the result because we have '\0' at end of the strings read,
    // it would affect effectiveness of the deduplication.
    // Helper bpf_perf_prog_read_value clears the buffer on error, so here we
    // (ab)use this behavior to clear the memory. It requires the size of py_symbol
    // to be different from struct bpf_perf_event_value, which we check at
    // compilation time using the FAIL_COMPILATION_IF macro.
    bpf_perf_prog_read_value(ctx, (struct bpf_perf_event_value *) symbol, sizeof(py_symbol));

    // Read class name from $frame->f_localsplus[0]->ob_type->tp_name.
    if (first_self || first_cls) {
        void *ptr;
        if (bpf_probe_read_user(
                &ptr, sizeof(void *), (void *) (cur_frame + offsets->VFrame_localsplus))) {
            bpf_dbg_printk("failed to read f_localsplus at %x\n", cur_frame + offsets->VFrame_localsplus);
            return -PY_ERROR_CLASS_NAME;
        }
        if (ptr) {
            if (first_self) {
                // we are working with an instance, first we need to get type
                if (bpf_probe_read_user(&ptr, sizeof(void *), ptr + offsets->PyObject_ob_type)) {
                    bpf_dbg_printk("failed to read ob_type at %x\n", ptr);
                    return -PY_ERROR_CLASS_NAME;
                }
            }
            // https://github.com/python/cpython/blob/d73501602f863a54c872ce103cd3fa119e38bac9/Include/cpython/object.h#L106
            if (bpf_probe_read_user(&ptr, sizeof(void *), ptr + offsets->PyTypeObject_tp_name)) {
                bpf_dbg_printk("failed to read tp_name at %x\n", ptr);
                return -PY_ERROR_CLASS_NAME;
            }
            long len = bpf_probe_read_user_str(&symbol->classname, sizeof(symbol->classname), ptr);
            if (len < 0) {
                bpf_dbg_printk("failed to read class name at %x\n", ptr);
                return -PY_ERROR_CLASS_NAME;
            }
            symbol->classname_type.type = PYSTR_TYPE_UTF8;
            symbol->classname_type.size_codepoints = len - 1;
        } else {
            // this happens in rideshare flask example under 3.9.18
            // todo: we should be able to get the class name
            // https://github.com/fabioz/PyDev.Debugger/blob/2cf10e3fb2ace33b6ef36d66332c82b62815e856/_pydevd_bundle/pydevd_utils.py#L104
            *((u64 *) symbol->classname) = 0x736c436c6c754e; // NullCls
            symbol->classname_type.type = PYSTR_TYPE_1BYTE | PYSTR_TYPE_ASCII;
            symbol->classname_type.size_codepoints = 7;
        }
    }

    void *pystr_ptr;
    // read PyCodeObject's filename into symbol
    if (bpf_probe_read_user(
            &pystr_ptr, sizeof(void *), code_ptr + offsets->PyCodeObject_co_filename)) {
        return -PY_ERROR_FILE_NAME;
    }
    if (pystr_ptr == 0) {
        return 0;
    }
    if (pystr_read(pystr_ptr, offsets, symbol->file, sizeof(symbol->file), &symbol->file_type)) {
        bpf_dbg_printk("failed to read file name at %x\n", pystr_ptr);
        return -PY_ERROR_FILE_NAME;
    }
    // read PyCodeObject's name into symbol
    if (bpf_probe_read_user(
            &pystr_ptr, sizeof(void *), code_ptr + offsets->PyCodeObject_co_name)) {
        return -PY_ERROR_NAME;
    }
    if (pystr_read(pystr_ptr, offsets, symbol->name, sizeof(symbol->name), &symbol->name_type)) {
        return -PY_ERROR_NAME;
    }
    return 0;
}

// get_frame_data reads current PyFrameObject filename/name and updates
// stack_info->frame_ptr with pointer to next PyFrameObject
// since 311 frame_ptr is pointing to _PyInterpreterFrame
// returns -PY_ERR_XXX on error, 1 on success, 0 if no more frames
static __always_inline int get_frame_data(
        void **frame_ptr,
        py_offset_config *offsets,
        py_symbol *symbol,
        // ctx is only used to call helper to clear symbol, see documentation below
        void *ctx) {
    void *code_ptr;
    void *cur_frame = *frame_ptr;
    if (!cur_frame) {
        return 0;
    }

    if (offsets->PyInterpreterFrame_owner != -1) {
        // https://github.com/python/cpython/blob/e7331365b488382d906ce6733ab1349ded49c928/Python/traceback.c#L991
        char owner = 0;
        if (bpf_probe_read_user(
                &owner, sizeof(owner), (void *) (cur_frame + offsets->PyInterpreterFrame_owner))) {
            return -PY_ERROR_FRAME_OWNER;
        }
        if (owner == FRAME_OWNED_BY_CSTACK) {
            if (bpf_probe_read_user(
                    frame_ptr, sizeof(void *), (void *) (cur_frame + offsets->VFrame_previous))) {
                return -PY_ERROR_FRAME_PREV;
            }
            cur_frame = *frame_ptr;
            if (!cur_frame) {
                return 0;
            }
        } else if (owner != FRAME_OWNED_BY_THREAD &&
                   owner != FRAME_OWNED_BY_GENERATOR &&
                   owner != FRAME_OWNED_BY_FRAME_OBJECT) {
            return -PY_ERROR_FRAME_OWNER_INVALID;
        }
    }
    // read PyCodeObject first, if that fails, then no point reading next frame
    if (bpf_probe_read_user(
            &code_ptr, sizeof(void *), (void *) (cur_frame + offsets->VFrame_code))) {
        return -PY_ERROR_FRAME_CODE;
    }
    if (!code_ptr) {
        return 0; // todo learn when this happens, c extension?
    }

    int res = get_names(cur_frame, code_ptr, offsets, symbol, ctx);
    if (res < 0) {
        return res;
    }

    // read next PyFrameObject/PyInterpreterFrame pointer, update in place
    if (bpf_probe_read_user(
            frame_ptr, sizeof(void *), (void *) (cur_frame + offsets->VFrame_previous))) {
        return -PY_ERROR_FRAME_PREV;
    }

    return 1;
}
// should be enough
#define PY_NUM_CPU 512

// To avoid duplicate ids, every CPU needs to use different ids when inserting
// into the hashmap. NUM_CPUS is defined at PyPerf backend side and passed
// through CFlag.
static __always_inline int get_symbol_id(
        py_sample_state_t *state,
        py_symbol *sym,
        py_symbol_id *out_symbol_id) {

    py_symbol_id *symbol_id_ptr = bpf_map_lookup_elem(&py_symbols, sym);
    if (symbol_id_ptr) {
        *out_symbol_id = *symbol_id_ptr;
        return 0;
    }
    // the symbol is new, bump the counter
    state->symbol_counter++;
    py_symbol_id symbol_id = state->symbol_counter * PY_NUM_CPU + state->cur_cpu;
    if (bpf_map_update_elem(&py_symbols, sym, &symbol_id, BPF_NOEXIST) == 0) {
        *out_symbol_id = symbol_id;
        return 0;
    }
    symbol_id_ptr = bpf_map_lookup_elem(&py_symbols, sym);
    if (symbol_id_ptr) {
        *out_symbol_id = *symbol_id_ptr;
        return 0;
    }
    *out_symbol_id = 0;
    return -1;
}

SEC("perf_event")
int read_python_stack(struct bpf_perf_event_data *ctx) {
    GET_STATE();

    state->python_stack_prog_call_cnt++;
    py_event *sample = &state->event;

    int last_res;
    py_symbol *sym = &state->sym;
#pragma unroll
    for (int i = 0; i < PYTHON_STACK_FRAMES_PER_PROG; i++) {
        last_res = get_frame_data((void **) &state->frame_ptr, &state->offsets, sym, ctx);
        if (last_res < 0) {
            return submit_error_sample((uint8_t) (-last_res));
        }
        if (last_res == 0) {
            break;
        }
        if (last_res == 1) {
            py_symbol_id symbol_id;
            if (get_symbol_id(state, sym, &symbol_id)) {
                return submit_error_sample(PY_ERROR_SYMBOL);
            }
            uint32_t cur_len = sample->stack_len;
            if (cur_len < PYTHON_STACK_MAX_LEN) {
                sample->stack[cur_len] = symbol_id;
                sample->stack_len++;
            }
        }
    }

    if (last_res == 0) {
        sample->k.flags = SAMPLE_KEY_FLAG_PYTHON_STACK;
    } else {
        sample->k.flags = (SAMPLE_KEY_FLAG_PYTHON_STACK|SAMPLE_KEY_FLAG_STACK_TRUNCATED);
    }

    if (sample->k.flags == (SAMPLE_KEY_FLAG_PYTHON_STACK|SAMPLE_KEY_FLAG_STACK_TRUNCATED) &&
        state->python_stack_prog_call_cnt < PYTHON_STACK_PROG_CNT) {
        // read next batch of frames
        bpf_tail_call(ctx, &py_progs, PYTHON_PROG_IDX_READ_PYTHON_STACK);
        return -1;
    }

    return submit_sample(state);
}

#endif // PYPERF_H

char _license[] SEC("license") = "GPL";
