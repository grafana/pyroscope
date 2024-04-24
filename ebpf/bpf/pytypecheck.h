#ifndef PYROEBPF_PYTYPECHECK_H
#define PYROEBPF_PYTYPECHECK_H

#define PY_TYPECHECK_ENABLED

#if defined(PY_TYPECHECK_ENABLED)

struct py_object_header {
    ssize_t ob_refcnt;
    void *ob_type;
};




static __always_inline int pytypecheck_obj(void *o, uint64_t typ) {
    if (typ == 0 || o == 0) {
        log_error("ptc obj expected type is null");
        return -1;
    }
    struct py_object_header obj = {};
    try_read(obj, o)
    log_debug("ptc obj o=%llx ob_type=%llx refcount =%llx ", o, obj.ob_type, obj.ob_refcnt);
    if (obj.ob_refcnt < 0) {
        log_error("ptc obj uaf");
        return -1;
    }
    if (obj.ob_type != (void *) typ) {
        log_error("ptc obj type mismatch %llx %llx", obj.ob_type, typ);
        return -1;
    }
    return 0;
}

static __always_inline int pytypecheck_glibc_header_size(void *o, uint64_t allocationSize) {
    u64 mchunk_sz;
    try_read(mchunk_sz, o - 0x8)
//#define chunksize(p) (chunksize_nomask (p) & ~(SIZE_BITS))
    mchunk_sz = mchunk_sz & ~(0x7);


    allocationSize += 0x8;
    if (allocationSize & 0xf) {
        allocationSize += 0x8;
    }
    if (mchunk_sz != allocationSize) {
        log_error("ptc o=%llx  allocationSize=%llx mchunk_sz=%llx", o, allocationSize, mchunk_sz);
        return -1;
    }
    log_debug("ptc o=%llx  allocationSize=%llx mchunk_sz=%llx", o, allocationSize, mchunk_sz);
    return 0;

}

static __always_inline int pytypecheck_interpreter_state(py_sample_state_t *state, void *is) {
    log_debug("ptc is = %llx", is);
    void *tstate_head;
    void *modules;
    void *importlib;
    uint32_t finalizing;

    try_read(tstate_head, is + state->typecheck.o_PyInterpreterState_tstate_head)
    try_read(modules, is + state->typecheck.o_PyInterpreterState_modules)
    try_read(importlib, is + state->typecheck.o_PyInterpreterState_importlib)
    try_read(finalizing, is + state->typecheck.o_PyInterpreterState_finalizing)

    log_debug("ptc ts = %llx modules = %llx importlib = %llx", tstate_head, modules, importlib);

    if (finalizing) {
        log_error("ptc interpreter is finalizing");
        return -1;
    }
    if (pytypecheck_glibc_header_size(is, state->typecheck.size_PyInterpreterState_tstate)) {
        return -1;
    }

    if (modules) {
        if (pytypecheck_obj(modules, state->typecheck.PyDict_Type)) {
            return -1;
        }
    } else {
        log_debug("ptc modules is null");
    }

    if (importlib) {

        if (pytypecheck_obj(importlib, state->typecheck.PyModule_Type)) {
            return -1;
        }
    } else {
        log_debug("ptc importlib is null");
    }

    log_debug("ptc ts = %llx mo = %llx il = %llx", tstate_head, modules, importlib);
    {
        if (tstate_head == 0) {
            log_error("ptc tstate_head is null");
            return -1;
        }
        void *dict;
        try_read(dict, tstate_head + state->typecheck.o_PyThreadState_dict)
        if (dict != 0) {
            if (pytypecheck_obj(dict, state->typecheck.PyDict_Type)) {
                return -1;
            }
        }
        if (pytypecheck_glibc_header_size(tstate_head, state->typecheck.size_PyThreadState)) {
            return -1;
        }
    }
    log_debug("ptc is %llx ok", is);
    return 0;
}

static __always_inline int pytypecheck_thread_state(py_sample_state_t *state, void *ts, bool check_interp) {
    log_debug("ptc ts %llx", ts);
    void *dict, *interp;
    try_read(dict, ts + state->typecheck.o_PyThreadState_dict)
    try_read(interp, ts + state->typecheck.o_PyThreadState_interp)
    log_debug("ptc %llx dict=%llx interp=%llx", dict, interp);
    if (dict != 0) {
        if (pytypecheck_obj(dict, state->typecheck.PyDict_Type)) {
            return -1;
        };
    }

    if (pytypecheck_glibc_header_size(ts, state->typecheck.size_PyThreadState)) {
        return -1;
    }
    if (check_interp) {
        if (pytypecheck_interpreter_state(state, interp)) {
            return -1;
        }
    }
    log_debug("ptc ts %llx ok", ts);

    return 0;
}


static __always_inline int pytypecheck_frame(py_sample_state_t *state, void *f) {
    log_debug("ptc f %llx", f);
    if (f == 0) {
        return 0;
    }
    if (pytypecheck_obj(f, state->typecheck.PyFrame_Type)) {
        return -1;
    }
    log_debug("ptc f %llx ok", f);
    return 0;
}

static __always_inline int pytypecheck_code(py_sample_state_t *state, void *code) {
    log_debug("ptc code %llx", code);
    if (code == 0) {
        return 0;
    }
    if (pytypecheck_obj(code, state->typecheck.PyCode_Type)) {
        return -1;
    }
    log_debug("ptc code %llx ok", code);
    return 0;
}


#else


#endif

#endif