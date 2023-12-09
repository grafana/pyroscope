
#if !defined(PYPERF_TEMPLATE_SECTION)
#error "PYPERF_TEMPLATE_SECTION must be defined"
#endif

#if !defined(PYPERF_TEMPLATE_SUFFIX)
#error "PYPERF_TEMPLATE_SUFFIX must be defined"
#endif

#if !defined(PYPERF_TEMPLATE_COLLECT_FLAGS)
#error "PYPERF_TEMPLATE_COLLECT_FLAGS must be defined"
#endif

#define sym_py_progs   CONCAT(py_progs__, PYPERF_TEMPLATE_SUFFIX)
#define sym_collect   CONCAT(collect__, PYPERF_TEMPLATE_SUFFIX)
#define sym_read_python_stack   CONCAT(read_python_stack__, PYPERF_TEMPLATE_SUFFIX)

#if PYPERF_TEMPLATE_COLLECT_FLAGS == FLAG_IS_CPU
#define prog_ctx struct bpf_perf_event_data
#else
#define prog_ctx struct pt_regs
#endif


int sym_read_python_stack( prog_ctx *ctx);

#define PYTHON_PROG_IDX_READ_PYTHON_STACK 0

struct {
    __uint(type, BPF_MAP_TYPE_PROG_ARRAY);
    __uint(max_entries, 2);
    __type(key, int);
    __array(values, int (void *));
} sym_py_progs SEC(".maps") = {
        .values = {
                [PYTHON_PROG_IDX_READ_PYTHON_STACK] = (void *) &sym_read_python_stack,
        },
};

SEC(PYPERF_TEMPLATE_SECTION)
int sym_collect( prog_ctx *ctx) {
    u32 pid;
    current_pid(&pid);
    if (pid == 0) {
        return 0;
    }
    uint64_t val;
//todo extract to function or macro
    uint64_t pc = 239;
#if PYPERF_TEMPLATE_COLLECT_FLAGS == FLAG_IS_MEM
#if defined(__TARGET_ARCH_x86)
    val = ctx->di;
    pc = ctx->ip;
#elif defined(__TARGET_ARCH_arm64)
    val = ctx->regs[0];
#endif
    bpf_dbg_printk("collect %d %d %d at %lx\n", pid, PYPERF_TEMPLATE_COLLECT_FLAGS,  val, pc);
#else
    val = 1;
#endif
    py_event *e = pyperf_collect_impl(ctx, (pid_t) pid, /* collect_kern_stack */ false); // todo allow configuring it

    if (e == NULL) {
        return 0;
    }
    e->value = val;
    e->hdr.flags = PYPERF_TEMPLATE_COLLECT_FLAGS;
    // jump to reading first set of Python frames
    bpf_tail_call(ctx, &sym_py_progs, PYTHON_PROG_IDX_READ_PYTHON_STACK);
    // we won't ever get here
    return 0;
}




SEC(PYPERF_TEMPLATE_SECTION)
int sym_read_python_stack( prog_ctx *ctx) {
    py_sample_state_t *state = get_state();
    if (!state) {
        return 0;
    }

    state->python_stack_prog_call_cnt++;
    py_event *sample = &state->event;

    py_symbol sym = {};
    int last_res;
#pragma unroll
    for (int i = 0; i < PYTHON_STACK_FRAMES_PER_PROG; i++) {
        last_res = get_frame_data((void **) &state->frame_ptr, &state->offsets, &sym, ctx);
        if (last_res < 0) {
            return submit_error_sample(state, (uint8_t) (-last_res));
        }
        if (last_res == 0) {
            break;
        }
        if (last_res == 1) {
            py_symbol_id symbol_id;
            if (get_symbol_id(state, &sym, &symbol_id)) {
                return submit_error_sample(state, PY_ERROR_SYMBOL);
            }
            uint32_t cur_len = sample->stack_len;
            if (cur_len < PYTHON_STACK_MAX_LEN) {
                sample->stack[cur_len].symbol_id = symbol_id;
                sample->stack_len++;
            }
        }
    }

    if (last_res == 0) {
        sample->hdr.stack_status = STACK_STATUS_COMPLETE;
    } else {
        sample->hdr.stack_status = STACK_STATUS_TRUNCATED;
    }

    if (sample->hdr.stack_status == STACK_STATUS_TRUNCATED &&
        state->python_stack_prog_call_cnt < PYTHON_STACK_PROG_CNT) {
        // read next batch of frames
        bpf_tail_call(ctx, &sym_py_progs, PYTHON_PROG_IDX_READ_PYTHON_STACK);
        return -1;
    }

    return submit_sample(state);
}

#undef sym_py_progs
#undef sym_collect
#undef sym_read_python_stack
#undef prog_ctx