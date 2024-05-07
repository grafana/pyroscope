#ifndef PYPERF_H
#define PYPERF_H

#include "vmlinux.h"
#include "bpf_helpers.h"

struct global_config_t {
    uint8_t bpf_log_err;
    uint8_t bpf_log_debug;
    uint8_t typecheck;
    uint64_t ns_pid_ino;
};

const volatile struct global_config_t global_config;
#define log_error(fmt, ...) if (global_config.bpf_log_err) bpf_printk("[pyperf *error*] " fmt, ##__VA_ARGS__)
#define log_debug(fmt, ...) if (global_config.bpf_log_debug) bpf_printk("[pyperf  debug ] "fmt, ##__VA_ARGS__)



#include "pthread.bpf.h"
#include "pid.h"
#include "stacks.h"
#include "pystr.h"
#include "pyoffsets.h"
#include "hash.h"



enum {
    PY_ERROR_GENERIC = 1,
    PY_ERROR_THREAD_STATE = 2,
//    PY_ERROR_THREAD_STATE_NULL = 3,
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
    PY_ERROR_FRAME_TYPECHECK = 15,
    PY_ERROR_CODE_TYPECHECK = 16,
    PY_ERROR_RESERVED2 = 17,
    PY_ERROR__RESERVED3 = 18,

    __PY_ERROR_TOTAL_NUMBER_OF_ERRORS = 19, // not an error
};

struct error_stats {
    uint32_t errors[__PY_ERROR_TOTAL_NUMBER_OF_ERRORS];
};

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, struct error_stats);
} py_errors SEC(".maps");



typedef struct {
    py_offset_config offsets;
    py_typecheck_data typecheck;
    py_version version;
    struct libc libc;
    int32_t tssKey;
    uint8_t collect_kernel;
} py_pid_data;





#define _STR_CONCAT(str1, str2) str1##str2
#define STR_CONCAT(str1, str2) _STR_CONCAT(str1, str2)
#define FAIL_COMPILATION_IF(condition)            \
  typedef struct {                                \
    char _condition_check[1 - 2 * !!(condition)]; \
  } STR_CONCAT(compile_time_condition_check, __COUNTER__);
// See comments in get_frame_data
FAIL_COMPILATION_IF(sizeof(py_symbol) == sizeof(struct bpf_perf_event_value))
FAIL_COMPILATION_IF(HASH_LIMIT != PYTHON_STACK_MAX_LEN * sizeof(py_symbol_id))



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

#include "pytypecheck.h"


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
        void *ctx,
        py_pid_data *pid_data,
        void **out_thread_state) {
    return pyro_pthread_getspecific(ctx, &pid_data->libc, pid_data->tssKey, out_thread_state);
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
    uint8_t err) {
    log_error("pyperf_err: %d\n", err);
    uint32_t zero = 0;
    struct error_stats *stats = bpf_map_lookup_elem(&py_errors, &zero);
    if (stats) {
        if (err < __PY_ERROR_TOTAL_NUMBER_OF_ERRORS) {
            stats->errors[err]++;
        } else {
            stats->errors[PY_ERROR_GENERIC]++; // should not happen
        }
    }
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

static __always_inline int get_top_frame(void *ctx, py_pid_data *pid_data, py_sample_state_t *state, void *thread_state) {
    if (pid_data->offsets.PyThreadState_frame == -1) {
        // >= py311 && <= py312
        void *cframe;
        try_read(cframe, thread_state + pid_data->offsets.PyThreadState_cframe)
        log_debug("cframe %llx", cframe);
        if (cframe == 0) {
            return -1;
        }
        try_read(state->frame_ptr, cframe + pid_data->offsets.PyCFrame_current_frame)
        return 0;
    }
    // < py311 && >= py313
    try_read(state->frame_ptr, thread_state + pid_data->offsets.PyThreadState_frame)
    return 0;
}

static __always_inline int pyperf_collect_impl(struct bpf_perf_event_data* ctx, pid_t pid) {
    py_pid_data *pid_data = bpf_map_lookup_elem(&py_pid_config, &pid);
    if (!pid_data) {
        return 0;
    }

    GET_STATE();

    state->offsets = pid_data->offsets;
#if defined(PY_TYPECHECK_ENABLED)
    state->typecheck = pid_data->typecheck;
#endif
    state->version = pid_data->version;
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
    void *thread_state = NULL;
    if (get_thread_state(ctx, pid_data, &thread_state)) {
        return submit_error_sample(PY_ERROR_THREAD_STATE);
    }
    log_debug("thread_state %llx base %llx", thread_state, pid_data->offsets.Base);
    if (pytypecheck_thread_state(state, thread_state, /* check_interp= */ true)) {
        return submit_error_sample(PY_ERROR_THREAD_STATE);
    }

    // pre-initialize event struct in case any subprogram below fails
    event->stack_len = 0;

    if (thread_state == 0) {
        return 0;// not a python thread or not initialized or finalized thread
    }
    if (get_top_frame(ctx, pid_data, state, thread_state)) {
        return submit_error_sample(PY_ERROR_TOP_FRAME);
    }
    log_debug("top frame %llx", state->frame_ptr);
    if (pytypecheck_frame(state, (void*)state->frame_ptr)) {
        return submit_error_sample(PY_ERROR_FRAME_TYPECHECK);
    }
    // jump to reading first set of Python frames
    bpf_tail_call(ctx, &py_progs, PYTHON_PROG_IDX_READ_PYTHON_STACK);
    // we won't ever get here
    return 0;
}

SEC("perf_event")
int pyperf_collect(struct bpf_perf_event_data *ctx) {
    log_debug(" ================ collect sample ================ ");
    u32 pid = 0;
    current_pid(global_config.ns_pid_ino, &pid);
    if (pid == 0) {
        return 0;
    }
#if defined(__TARGET_ARCH_x86)

    uint64_t pid_tgid = bpf_get_current_pid_tgid();
    u32 hostpid = (u32)(pid_tgid >> 32);

    log_debug("pid %d | %d )", pid, hostpid);
    log_debug("userspace=%d cs=0x%x)", ctx->regs.cs == 0x33, ctx->regs.cs);
#elif defined(__TARGET_ARCH_arm64)

#else
#error "Unknown architecture"
#endif

    return pyperf_collect_impl(ctx, (pid_t) pid);
}


static __always_inline int check_first_arg(void *ctx, void *code_ptr,
                                           py_sample_state_t *state,
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
    log_debug("check_first_arg");

    if (state->offsets.PyCodeObject_co_varnames == -1) { // was removed in 3.11 // https://github.com/python/cpython/commit/2bde6827ea4f136297b2d882480b981ff26262b6
        try_read(args_ptr, code_ptr + state->offsets.PyCodeObject_co_localsplusnames) /* tuple mapping offsets to names */
    } else {
        try_read(args_ptr, code_ptr + state->offsets.PyCodeObject_co_varnames) /* tuple of strings (local variable names) */
    }
    if (args_ptr == 0) {
        log_debug("args NULL");
        *out_first_self = false;
        *out_first_cls = false;
        return 0;
    }
    try(pytypecheck_tuple(state, args_ptr))
    log_debug("args_ptr %llx", args_ptr);

    try_read(args_size, args_ptr + state->offsets.PyVarObject_ob_size)
    if (args_size < 1) {
        log_debug("args empty");
        *out_first_self = false;
        *out_first_cls = false;
        return 0;
    }
    try_read(args_ptr, args_ptr + state->offsets.PyTupleObject_ob_item)
    log_debug("ob_item %llx", args_ptr);
    uint64_t symbol_name = 0;
    struct py_str_type symbol_name_type = {};
    try (pystr_read(ctx, args_ptr, state, (char *)&symbol_name, sizeof(symbol_name), &symbol_name_type))
    // compare strings as ints to save instructions
    char self_str[4] = {'s', 'e', 'l', 'f'};
    char cls_str[4] = {'c', 'l', 's', '\0'};
    *out_first_self = (*(int32_t *) &symbol_name) == *(int32_t *) self_str;
    *out_first_cls = (*(int32_t *) &symbol_name) == *(int32_t *) cls_str;
    log_debug("first arg %s", &symbol_name);
    return 0;
}

#define IGNORE_NAMES_ERROR 1


static __always_inline int get_code_name(void *ctx, void *code_ptr,
                                          py_sample_state_t *state,
                                          py_symbol *symbol) {
    void *pystr_ptr;

    // read PyCodeObject's name into symbol
    if (read_user_faulty(ctx,
            &pystr_ptr, sizeof(void *), code_ptr + state->offsets.PyCodeObject_co_name)) {
        log_error("failed to read co_name at %llx", code_ptr);
#if defined(IGNORE_NAMES_ERROR)
        log_error("CodErr1");
        *((u64 *) symbol->classname) = 0x31727245646f43; // CodErr1
        symbol->classname_type.type = PYSTR_TYPE_1BYTE | PYSTR_TYPE_ASCII;
        symbol->classname_type.size_codepoints = 7;
        return 0;
#else
        return -PY_ERROR_NAME;
#endif
    }
    if (pystr_read(ctx,pystr_ptr, state, symbol->name, sizeof(symbol->name), &symbol->name_type)) {
        log_error("failed to read name at %llx", pystr_ptr);
#if defined(IGNORE_NAMES_ERROR)
        log_error("CodErr2");
        *((u64 *) symbol->classname) = 0x32727245646f43; // CodErr2
        symbol->classname_type.type = PYSTR_TYPE_1BYTE | PYSTR_TYPE_ASCII;
        symbol->classname_type.size_codepoints = 7;
        return 0;
#else
        return -PY_ERROR_NAME;
#endif
    }
    return 0;
}
static __always_inline int get_file_name(void *ctx,
                                           void *code_ptr,
                                           py_sample_state_t *state,
                                           py_symbol *symbol) {
    void *pystr_ptr;
    // read PyCodeObject's filename into symbol
    if (read_user_faulty(ctx,
            &pystr_ptr, sizeof(void *), code_ptr + state->offsets.PyCodeObject_co_filename)) {
        log_error("failed to read co_filename at %llx", code_ptr);
#if defined(IGNORE_NAMES_ERROR)
        log_error("FilErr1");
        *((u64 *) symbol->classname) = 0x317272456c6946; // FilErr1
        symbol->classname_type.type = PYSTR_TYPE_1BYTE | PYSTR_TYPE_ASCII;
        symbol->classname_type.size_codepoints = 7;
        return 0;
#else
        return -PY_ERROR_FILE_NAME;
#endif
    }
    if (pystr_ptr == 0) {
        log_error("null file name");
        return 0;
    }
    if (pystr_read(ctx, pystr_ptr, state, symbol->file, sizeof(symbol->file), &symbol->file_type)) {
        log_error("failed to read file name at %llx", pystr_ptr);
#if defined(IGNORE_NAMES_ERROR)
        log_error("FilErr2");
        *((u64 *) symbol->classname) = 0x327272456c6946; // FilErr2
        symbol->classname_type.type = PYSTR_TYPE_1BYTE | PYSTR_TYPE_ASCII;
        symbol->classname_type.size_codepoints = 7;
        return 0;
#else
        return -PY_ERROR_FILE_NAME;
#endif
    }
    return 0;

}
static __always_inline int get_class_name(void *ctx, void *cur_frame,
                                           void *code_ptr,
                                           py_sample_state_t *state,
                                           py_symbol *symbol) {
    uint64_t objheader[2] = {0, 0};
    
    bool first_self = false;
    bool first_cls = false;
    log_debug("get_names");
    if (check_first_arg(ctx, code_ptr, state, &first_self, &first_cls)) {

#if defined(IGNORE_NAMES_ERROR)
        log_debug("ignore_names_error check_first_arg");
        first_self = false;
        first_cls = false;
#else
        return -PY_ERROR_FIRST_ARG;
#endif
    }
    log_debug("first_self %d first_cls %d", first_self, first_cls);

    if (!first_self && !first_cls) {
        return 0;
    }
    int co_nlocals = 0;
    try_read(co_nlocals, code_ptr + state->offsets.PyCodeObject_co_nlocals)
    log_debug("co_nlocals %d", co_nlocals);
    if (co_nlocals < 1) {
        *((u64 *) symbol->classname) = 0x31736c436f4e; // NoCls1
        symbol->classname_type.type = PYSTR_TYPE_1BYTE | PYSTR_TYPE_ASCII;
        symbol->classname_type.size_codepoints = 7;
        return 0;
    }
    // Read class name from $frame->f_localsplus[0]->ob_type->tp_name.
    void *ptr = NULL;
    if (read_user_faulty(ctx,
            &ptr, sizeof(void *), (void *) (cur_frame + state->offsets.VFrame_localsplus))) {
        log_error("failed to read f_localsplus at %llx", cur_frame);
#if defined(IGNORE_NAMES_ERROR)
        log_error("ErrCls1");
        *((u64 *) symbol->classname) = 0x31736c43727245; // ErrCls1
        symbol->classname_type.type = PYSTR_TYPE_1BYTE | PYSTR_TYPE_ASCII;
        symbol->classname_type.size_codepoints = 7;
        return 0;
#else
        return -PY_ERROR_CLASS_NAME;
#endif
    }
    bpf_probe_read_user(&objheader, sizeof(objheader), ptr);
    log_debug("first local %llx | %llx %llx", ptr, objheader[0], objheader[1]);
    if (ptr) {
        if (first_self) {
            // we are working with an instance, first we need to get type
            if (read_user_faulty(ctx, &ptr, sizeof(void *), ptr + state->offsets.PyObject_ob_type)) {
                log_error("failed to read ob_type at %llx", ptr);
#if defined(IGNORE_NAMES_ERROR)
                log_error("ErrCls2");
                *((u64 *) symbol->classname) = 0x32736c43727245; // ErrCls2
                symbol->classname_type.type = PYSTR_TYPE_1BYTE | PYSTR_TYPE_ASCII;
                symbol->classname_type.size_codepoints = 7;
                return 0;
#else
                return -PY_ERROR_CLASS_NAME;
#endif
            }
            log_debug("ob_type %llx", ptr);
            if (ptr == NULL) {
                // never seen, added just in case
                log_error("NilCls2");
                *((u64 *) symbol->classname) = 0x32736c436c694e; // NilCls2
                symbol->classname_type.type = PYSTR_TYPE_1BYTE | PYSTR_TYPE_ASCII;
                symbol->classname_type.size_codepoints = 7;
                return 0;
            }
            bpf_probe_read_user(&objheader, sizeof(objheader), ptr);
            log_debug("    first local->ob_type %llx | %llx %llx", ptr, objheader[0], objheader[1]);
        }
        // https://github.com/python/cpython/blob/d73501602f863a54c872ce103cd3fa119e38bac9/Include/cpython/object.h#L106
        if (read_user_faulty(ctx, &ptr, sizeof(void *), ptr + state->offsets.PyTypeObject_tp_name)) {
            log_error("failed to read tp_name at %llx", ptr);
#if defined(IGNORE_NAMES_ERROR)
            log_error("ErrCls3");
            *((u64 *) symbol->classname) = 0x33736c43727245; // ErrCls3
            symbol->classname_type.type = PYSTR_TYPE_1BYTE | PYSTR_TYPE_ASCII;
            symbol->classname_type.size_codepoints = 7;
            return 0;
#else
            return -PY_ERROR_CLASS_NAME;
#endif
        }
        log_debug("tp_name %llx", ptr);
        long len = bpf_probe_read_user_str(&symbol->classname, sizeof(symbol->classname), ptr);
        if (len < 0) {
            submit_fault_event(ctx, (void *) ptr);
            log_error("failed to read class name at %llx", ptr);
#if defined(IGNORE_NAMES_ERROR)
            log_error("ErrCls4");
            *((u64 *) symbol->classname) = 0x34736c43727245; // ErrCls4
            symbol->classname_type.type = PYSTR_TYPE_1BYTE | PYSTR_TYPE_ASCII;
            symbol->classname_type.size_codepoints = 7;
            return 0;
#else
            return -PY_ERROR_CLASS_NAME;
#endif
        }
        symbol->classname_type.type = PYSTR_TYPE_UTF8;
        symbol->classname_type.size_codepoints = len - 1;
    } else {
        log_error("NullCls");
        // this happens in rideshare flask example under 3.9.18
        // todo: we should be able to get the class name
        // https://github.com/fabioz/PyDev.Debugger/blob/2cf10e3fb2ace33b6ef36d66332c82b62815e856/_pydevd_bundle/pydevd_utils.py#L104
        *((u64 *) symbol->classname) = 0x736c436c6c754e; // NullCls
        symbol->classname_type.type = PYSTR_TYPE_1BYTE | PYSTR_TYPE_ASCII;
        symbol->classname_type.size_codepoints = 7;
    }
    return 0;
}


// return -PY_ERR_XX on error, 0 on success
static __always_inline int get_names(
        void *cur_frame,
        void *code_ptr,
        py_sample_state_t *state,
        py_symbol *symbol,
        void *ctx) {
    // We re-use the same py_symbol instance across loop iterations, which means
    // we will have left-over data in the struct. Although this won't affect
    // correctness of the result because we have '\0' at end of the strings read,
    // it would affect effectiveness of the deduplication.
    // Helper bpf_perf_prog_read_value clears the buffer on error, so here we
    // (ab)use this behavior to clear the memory. It requires the size of py_symbol
    // to be different from struct bpf_perf_event_value, which we check at
    // compilation time using the FAIL_COMPILATION_IF macro.
    bpf_perf_prog_read_value(ctx, (struct bpf_perf_event_value *) symbol, sizeof(py_symbol));

    if (get_class_name(ctx, cur_frame, code_ptr, state, symbol)) {
        return -PY_ERROR_CLASS_NAME;
    }

    if (get_file_name(ctx, code_ptr, state, symbol)) {
        return -PY_ERROR_FILE_NAME;
    }

    if (get_code_name(ctx, code_ptr, state, symbol)) {
        return -PY_ERROR_NAME;
    }
    return 0;
}

// get_frame_data reads current PyFrameObject filename/name and updates
// stack_info->frame_ptr with pointer to next PyFrameObject
// since 311 frame_ptr is pointing to _PyInterpreterFrame
// returns -PY_ERR_XXX on error, 1 on success, 0 if no more frames
static __always_inline int get_frame_data(
        py_sample_state_t *state,
        py_symbol *symbol,
        // ctx is only used to call helper to clear symbol, see documentation below
        void *ctx) {
    uint64_t objheader[2] = {0, 0};
    void **frame_ptr = (void **) &state->frame_ptr;
    py_offset_config *offsets = &state->offsets;
    void *code_ptr;
    void *cur_frame = *frame_ptr;
    if (!cur_frame) {
        return 0;
    }

    if (offsets->PyInterpreterFrame_owner != -1) {
        // https://github.com/python/cpython/blob/e7331365b488382d906ce6733ab1349ded49c928/Python/traceback.c#L991
        char owner = 0;
        if (read_user_faulty(ctx,
                &owner, sizeof(owner), (void *) (cur_frame + offsets->PyInterpreterFrame_owner))) {
            return -PY_ERROR_FRAME_OWNER;
        }
        log_debug("frame owner %llx", owner);
        if (owner == FRAME_OWNED_BY_CSTACK) {
            if (read_user_faulty(ctx,
                    frame_ptr, sizeof(void *), (void *) (cur_frame + offsets->VFrame_previous))) {
                return -PY_ERROR_FRAME_PREV;
            }
            cur_frame = *frame_ptr;
            log_debug("frame %llx", cur_frame);
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
    if (read_user_faulty(ctx,
            &code_ptr, sizeof(void *), (void *) (cur_frame + offsets->VFrame_code))) {
        return -PY_ERROR_FRAME_CODE;
    }
    if (!code_ptr) {
        return 0; // todo learn when this happens, c extension?
    }
    bpf_probe_read_user(&objheader[0], sizeof (objheader), code_ptr);
    log_debug("code %llx | %llx %llx", code_ptr, objheader[0], objheader[1]);


    if (pytypecheck_code(state, (void*)code_ptr, (void*)cur_frame)) {
        return -PY_ERROR_CODE_TYPECHECK;
    }

    int res = get_names(cur_frame, code_ptr, state, symbol, ctx);
    if (res < 0) {
        return res;
    }
    log_debug("sym name %s", symbol->name);
    log_debug("sym file %s", symbol->file);
    log_debug("sym cls  %s", symbol->classname);

    // read next PyFrameObject/PyInterpreterFrame pointer, update in place
    if (read_user_faulty(ctx,
            frame_ptr, sizeof(void *), (void *) (cur_frame + offsets->VFrame_previous))) {
        log_error("failed to read f_back at %llx", cur_frame);
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
    log_error("get_symbol_id failed");
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
        log_debug("------- frame %d %llx ---------- ", sample->stack_len, state->frame_ptr);
        if (pytypecheck_frame(state, (void*)state->frame_ptr)) {
            return submit_error_sample(PY_ERROR_FRAME_TYPECHECK);
        }
        last_res = get_frame_data(state, sym, ctx);
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
