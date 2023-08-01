#ifndef PYPERF_H
#define PYPERF_H

#include "pyperf.bpf.h"
#include "pthread.bpf.h"


#define PYTHON_STACK_FRAMES_PER_PROG 25
#define PYTHON_STACK_PROG_CNT 3
#define PYTHON_STACK_MAX_LEN (PYTHON_STACK_FRAMES_PER_PROG * PYTHON_STACK_PROG_CNT)
#define PYTHON_CLASS_NAME_LEN 32
#define PYTHON_FUNCTION_NAME_LEN 64
#define PYTHON_FILE_NAME_LEN 128

enum {
    STACK_STATUS_COMPLETE = 0,
    STACK_STATUS_ERROR = 1,
    STACK_STATUS_TRUNCATED = 2,
};


//todo compress it a bit, reuse fields depending on version
typedef struct {
    int16_t PyVarObject_ob_size;
    int16_t PyObject_ob_type;
    int16_t PyTypeObject_tp_name;
    int16_t PyThreadState_frame;
    int16_t PyThreadState_cframe;
    int16_t PyCFrame_current_frame;
    int16_t PyCodeObject_co_filename;
    int16_t PyCodeObject_co_name;
    int16_t PyCodeObject_co_varnames;
    int16_t PyCodeObject_co_localsplusnames;
    int16_t PyTupleObject_ob_item;

    int16_t VFrame_code; // PyFrameObject_f_code pre 311 or PyInterpreterFrame_f_code post 311
    int16_t VFrame_previous; // PyFrameObject_f_back pre 311 or PyInterpreterFrame_previous post 311
    int16_t VFrame_localsplus; // PyFrameObject_localsplus pre 311 or PyInterpreterFrame_localsplus post 311

    int16_t String_size;
} py_offset_config;

typedef struct {
    uint32_t major;
    uint32_t minor;
    uint32_t patch;
} py_version;

typedef struct {
    py_offset_config offsets;
    py_version version;
    uint8_t musl;// 1,2 if musl libc is used, 0 otherwise
    int32_t tssKey;
} py_pid_data;

typedef struct {
    char classname[PYTHON_CLASS_NAME_LEN];
    char name[PYTHON_FUNCTION_NAME_LEN];
    char file[PYTHON_FILE_NAME_LEN];

    // NOTE: PyFrameObject also has line number but it is typically just the
    // first line of that function and PyCode_Addr2Line needs to be called
    // to get the actual line
} py_symbol;


typedef struct {
    uint32_t pid;
    uint8_t stack_status;
    // instead of storing symbol name here directly, we add it to another
    // hashmap with Symbols and only store the ids here
    int64_t stack_len;
    uint32_t stack[PYTHON_STACK_MAX_LEN];
} py_event;

#define _STR_CONCAT(str1, str2) str1##str2
#define STR_CONCAT(str1, str2) _STR_CONCAT(str1, str2)
#define FAIL_COMPILATION_IF(condition)            \
  typedef struct {                                \
    char _condition_check[1 - 2 * !!(condition)]; \
  } STR_CONCAT(compile_time_condition_check, __COUNTER__);
// See comments in get_frame_data
FAIL_COMPILATION_IF(sizeof(py_symbol) == sizeof(struct bpf_perf_event_value))

//todo make it void *back, keep u64 in struct only
typedef u64 frame_ptr_t;

typedef struct {
    py_offset_config offsets;
    uint32_t cur_cpu;
    int64_t symbol_counter;
    frame_ptr_t frame_ptr;
    int64_t python_stack_prog_call_cnt;
    py_event event;
} py_sample_state_t;

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __type(key, u32);
    __type(value, py_sample_state_t);
    __uint(max_entries, 1);
} py_state_heap SEC(".maps");

typedef uint32_t py_symbol_id;

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
    __uint(max_entries, 10240); // todo
} py_pid_config SEC(".maps");

#define PYTHON_PROG_IDX_READ_PYTHON_STACK 0

int read_python_stack(struct bpf_perf_event_data *ctx);


struct {
    __uint(type, BPF_MAP_TYPE_PROG_ARRAY);
    __uint(max_entries, 2);
    __type(key, int);
    __array(values, int (void *)); //todo
} py_progs SEC(".maps") = {
        .values = {
                [PYTHON_PROG_IDX_READ_PYTHON_STACK] = (void *) &read_python_stack,
        },
};


struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
    __uint(key_size, sizeof(u32));
    __uint(value_size, sizeof(u32));
} py_events SEC(".maps");

static inline __attribute__((__always_inline__)) int get_thread_state(
        void *tls_base,
        py_pid_data *pid_data,
        void **out_thread_state) {
    return pyro_pthread_getspecific(pid_data->musl, pid_data->tssKey, tls_base, out_thread_state);
}

static inline __attribute__((__always_inline__)) int submit_sample(
        void *ctx,
        py_sample_state_t *state) {
    bpf_perf_event_output(ctx, &py_events, BPF_F_CURRENT_CPU, &state->event, sizeof(py_event));
    return 0; // todo return value
}

// this function is trivial, but we need to do map lookup in separate function,
// because BCC doesn't allow direct map calls (including lookups) from inside
// a macro (which we want to do in GET_STATE() macro below)
static inline __attribute__((__always_inline__)) py_sample_state_t *get_state() {
    int zero = 0;
    return bpf_map_lookup_elem(&py_state_heap, &zero);
}

#define GET_STATE()                     \
  py_sample_state_t* state = get_state();  \
  if (!state) {                         \
    return -1; /* should never happen */ \
  }

int get_top_frame(py_pid_data *pid_data, py_sample_state_t *state, void *thread_state) {
    if (pid_data->offsets.PyThreadState_frame == -1) {
        // py311
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
    if (bpf_probe_read_user(
            &state->frame_ptr,
            sizeof(void *),
            thread_state + pid_data->offsets.PyThreadState_frame)) {
        return -1;
    }
    return 0;
}

static inline int pyperf_collect_impl(struct bpf_perf_event_data *ctx, pid_t pid) {

    py_pid_data *pid_data = bpf_map_lookup_elem(&py_pid_config, &pid);
    if (!pid_data) {
        return 0;
    }

    GET_STATE();

    state->offsets = pid_data->offsets;
    state->cur_cpu = bpf_get_smp_processor_id();
    state->python_stack_prog_call_cnt = 0;

    py_event *event = &state->event;
    event->pid = pid;


    void *tls_base = NULL;
    if (pyro_get_tlsbase(&tls_base)) {
        return -1;
    }

    // Read PyThreadState of this Thread from TLS
    void *thread_state;
    if (get_thread_state(tls_base, pid_data, &thread_state)) {
        return -1;
    }

    // pre-initialize event struct in case any subprogram below fails
    event->stack_status = STACK_STATUS_COMPLETE;
    event->stack_len = 0;

    if (thread_state != 0) {
        if (get_top_frame(pid_data, state, thread_state)) {
            return -1;
        }
        // jump to reading first set of Python frames
        bpf_tail_call(ctx, &py_progs, PYTHON_PROG_IDX_READ_PYTHON_STACK);
        // we won't ever get here
    }

    return submit_sample(ctx, state);
}

SEC("perf_event")
int pyperf_collect(struct bpf_perf_event_data *ctx) {
    uint64_t pid_tgid = bpf_get_current_pid_tgid();
    pid_t pid = (pid_t) (pid_tgid >> 32);
    return pyperf_collect_impl(ctx, pid);
}

static inline __attribute__((__always_inline__)) int check_first_arg(void *code_ptr,
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
    if (bpf_probe_read_user_str(
            &symbol->name, sizeof(symbol->name), args_ptr + offsets->String_size) < 0) {
        return -1;
    }
    // compare strings as ints to save instructions
    char self_str[4] = {'s', 'e', 'l', 'f'};
    char cls_str[4] = {'c', 'l', 's', '\0'};
    *out_first_self = *(int32_t *) symbol->name == *(int32_t *) self_str;
    *out_first_cls = *(int32_t *) symbol->name == *(int32_t *) cls_str;

    return 0;
}

static inline __attribute__((__always_inline__)) int get_names(
        frame_ptr_t cur_frame,
        void *code_ptr,
        py_offset_config *offsets,
        py_symbol *symbol,
        void *ctx) {

    bool first_self;
    bool first_cls;
    if (check_first_arg(code_ptr, offsets, symbol, &first_self, &first_cls)) {
        return -1;
    }

    // We re-use the same py_symbol instance across loop iterations, which means
    // we will have left-over data in the struct. Although this won't affect
    // correctness of the result because we have '\0' at end of the strings read,
    // it would affect effectiveness of the deduplication.
    // Helper bpf_perf_prog_read_value clears the buffer on error, so here we
    // (ab)use this behavior to clear the memory. It requires the size of py_symbol
    // to be different from struct bpf_perf_event_value, which we check at
    // compilation time using the FAIL_COMPILATION_IF macro.
    //todo just do bpf_probe_read_user of zero page, how to find one?
    bpf_perf_prog_read_value(ctx, (struct bpf_perf_event_value *) symbol, sizeof(py_symbol));

    // Read class name from $frame->f_localsplus[0]->ob_type->tp_name.
    if (first_self || first_cls) {
        void *ptr;
        if (bpf_probe_read_user(
                &ptr, sizeof(void *), (void *) (cur_frame + offsets->VFrame_localsplus))) {
            return -1;
        }
        if (first_self) {
            // we are working with an instance, first we need to get type
            if (bpf_probe_read_user(&ptr, sizeof(void *), ptr + offsets->PyObject_ob_type)) {
                return -1;
            }
        }
        if (bpf_probe_read_user(&ptr, sizeof(void *), ptr + offsets->PyTypeObject_tp_name)) {
            return -1;
        }
        if (bpf_probe_read_user_str(&symbol->classname, sizeof(symbol->classname), ptr) < 0) {
            return -1;
        }
    }

    void *pystr_ptr;
    // read PyCodeObject's filename into symbol
    if (bpf_probe_read_user(
            &pystr_ptr, sizeof(void *), code_ptr + offsets->PyCodeObject_co_filename)) {
        return -1;
    }
    if (bpf_probe_read_user_str(
            &symbol->file, sizeof(symbol->file), pystr_ptr + offsets->String_size)) {
        return -1;
    }
    // read PyCodeObject's name into symbol
    if (bpf_probe_read_user(
            &pystr_ptr, sizeof(void *), code_ptr + offsets->PyCodeObject_co_name)) {
        return -1;
    }
    if (bpf_probe_read_user_str(
            &symbol->name, sizeof(symbol->name), pystr_ptr + offsets->String_size) < 0) {
        return -1;
    }
    return 0;
}

// get_frame_data reads current PyFrameObject filename/name and updates
// stack_info->frame_ptr with pointer to next PyFrameObject
// since 311 frame_ptr is pointing to _PyInterpreterFrame
// returns -1 on error, 1 on success, 0 if no more frames
static inline __attribute__((__always_inline__)) int get_frame_data(
        frame_ptr_t *frame_ptr,
        py_offset_config *offsets,
        py_symbol *symbol,
        // ctx is only used to call helper to clear symbol, see documentation below
        void *ctx) {
    void *code_ptr;
    frame_ptr_t cur_frame = *frame_ptr;
//    bpf_printk("fp %x", cur_frame);
    if (!cur_frame) {
        return 0;
    }

    //        py311
//        _PyInterpreterFrame *frame = tstate->cframe->current_frame
//        if frame == NULL:
//            return
//        while True:
//            yield frame
//            frame = frame->previous
//            if frame == NULL:
//                break
//            }
//
    // read PyCodeObject first, if that fails, then no point reading next frame
    if (bpf_probe_read_user(
            &code_ptr, sizeof(void *), (void *) (cur_frame + offsets->VFrame_code))) {
        return -1;
    }
    if (!code_ptr) {
        return 0; // todo learn when this happens, c extension?
    }

    if (get_names(cur_frame, code_ptr, offsets, symbol, ctx)) {
        return -1;
    }

    // read next PyFrameObject/PyInterpreterFrame pointer, update in place
    if (bpf_probe_read_user(
            frame_ptr, sizeof(void *), (void *) (cur_frame + offsets->VFrame_previous))) {
        return -1;
    }

    return 1;
}
// should be enough
#define PY_NUM_CPU 512

// To avoid duplicate ids, every CPU needs to use different ids when inserting
// into the hashmap. NUM_CPUS is defined at PyPerf backend side and passed
// through CFlag.
static inline __attribute__((__always_inline__)) int64_t get_symbol_id(
        py_sample_state_t *state,
        py_symbol *sym) {

    py_symbol_id *symbol_id_ptr = bpf_map_lookup_elem(&py_symbols, sym);
    if (symbol_id_ptr) {
        return *symbol_id_ptr;
    }
    // the symbol is new, bump the counter
    state->symbol_counter++;
    py_symbol_id symbol_id = state->symbol_counter * PY_NUM_CPU + state->cur_cpu;
    bpf_map_update_elem(&py_symbols, sym, &symbol_id, BPF_ANY);
    // todo do not use BPF_ANY here?
    // todo retry lookup if failed
    // todo handler error
    return symbol_id;
}

SEC("perf_event")
int read_python_stack(struct bpf_perf_event_data *ctx) {
    GET_STATE();

    state->python_stack_prog_call_cnt++;
    py_event *sample = &state->event;

    py_symbol sym = {};
    int last_res = false;
#pragma unroll
    for (int i = 0; i < PYTHON_STACK_FRAMES_PER_PROG; i++) {
        last_res = get_frame_data(&state->frame_ptr, &state->offsets, &sym, ctx);
        if (last_res == -1) {
            return -1;
        }
        if (last_res == 0) {
            break;
        }
        if (last_res == 1) {
            uint32_t symbol_id = get_symbol_id(state, &sym);
            int64_t cur_len = sample->stack_len;
            if (cur_len >= 0 && cur_len < PYTHON_STACK_MAX_LEN) {
                sample->stack[cur_len] = symbol_id;
                sample->stack_len++;
            }
        }
    }

    if (!state->frame_ptr) {
        sample->stack_status = STACK_STATUS_COMPLETE;
    } else {
        if (!last_res) {
            sample->stack_status = STACK_STATUS_ERROR;
        } else {
            sample->stack_status = STACK_STATUS_TRUNCATED;
        }
    }

    if (sample->stack_status == STACK_STATUS_TRUNCATED &&
        state->python_stack_prog_call_cnt < PYTHON_STACK_PROG_CNT) {
        // read next batch of frames
        bpf_tail_call(ctx, &py_progs, PYTHON_PROG_IDX_READ_PYTHON_STACK);
    }

    return submit_sample(ctx, state);
}

#endif // PYPERF_H