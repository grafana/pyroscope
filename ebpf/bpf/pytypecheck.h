#ifndef PYROEBPF_PYTYPECHECK_H
#define PYROEBPF_PYTYPECHECK_H

#define PY_TYPECHECK_ENABLED

#if defined(PY_TYPECHECK_ENABLED)

struct py_object_header {
    ssize_t ob_refcnt;
    void *ob_type;
};

#define pytypecheck_read(dst, src) \
    if (bpf_probe_read_user(&(dst), sizeof((dst)), src)) { \
        /* log_error("ptc _read failed  %llx", src); */  \
        return -1; \
    }


static __always_inline int pytypecheck_obj(void *o, uint64_t typ) {
    if (typ == 0 || o == 0) {
//        log_error("ptc _obj expected type is null");
        return -1;
    }
    struct py_object_header obj = {};
    pytypecheck_read(obj, o)
    log_debug("ptc _obj ob_type=%llx refcount =%llx o=%llx", obj.ob_type, obj.ob_refcnt, o);
    if (obj.ob_refcnt < 0) {
//        log_error("ptc _obj uaf");
        return -1;
    }
    if (obj.ob_type != (void *) typ) {
        log_error("ptc _obj type mismatch %llx %llx", obj.ob_type, typ);
        return -1;
    }


    return 0;
}

static __always_inline int pytypecheck_glibc_header_size(py_sample_state_t *state, void *o, uint64_t allocationSize) {
    u64 mchunk_sz;
    pytypecheck_read(mchunk_sz, o - 0x8)
//#define chunksize(p) (chunksize_nomask (p) & ~(SIZE_BITS))
    mchunk_sz = mchunk_sz & ~(0x7);
//    log_debug("ptc _thread_state mchunk_sz=%llx", mchunk_sz);


    allocationSize += 0x8;
    if (allocationSize & 0xf) {
        allocationSize += 0x8;
    }
//    log_debug("ptc _thread_state allocationSize=%llx", allocationSize);
    if (mchunk_sz != allocationSize) {
//        log_error("ptc malloc sz %llx %llx %llx", mchunk_sz, allocationSize, o);
        return -1;
    }
    return 0;
}

static __always_inline int pytypecheck_interpreter_state(py_sample_state_t *state, void *interp) {
    log_debug("is = %llx", interp);
    void *tstate_head;
    void *modules;
    void *importlib;
    uint32_t finalizing;
//    log_debug("o_tshead = %llx o_modules = %llx o_importlib = %llx o_finalizing = %llx",
//              state->typecheck.o_PyInterpreterState_tstate_head,
//              state->typecheck.o_PyInterpreterState_modules,
//              state->typecheck.o_PyInterpreterState_importlib,
//              state->typecheck.o_PyInterpreterState_finalizing);
    pytypecheck_read(tstate_head, interp + state->typecheck.o_PyInterpreterState_tstate_head)
    log_debug("tstate_head = %llx", tstate_head);
    pytypecheck_read(modules, interp + state->typecheck.o_PyInterpreterState_modules)
    log_debug("modules = %llx", modules);
    pytypecheck_read(importlib, interp + state->typecheck.o_PyInterpreterState_importlib)
    log_debug("importlib = %llx", importlib);
    pytypecheck_read(finalizing, interp + state->typecheck.o_PyInterpreterState_finalizing)
    log_debug("finalizing = %x", finalizing);

    if (finalizing) {
        return -1;
    }
    if (pytypecheck_glibc_header_size(state, interp, state->typecheck.size_PyInterpreterState_tstate)) {
        return -1;
    }

    if (modules) {
        if (pytypecheck_obj(modules, state->typecheck.PyDict_Type)) {
            return -1;
        }
    } else {
        log_debug("modules is null");
    }

    if (importlib) {

        if (pytypecheck_obj(importlib, state->typecheck.PyModule_Type)) {
            return -1;
        }
    } else {
        log_debug("importlib is null");
    }

    log_debug("ts = %llx mo = %llx il = %llx", tstate_head, modules, importlib);
    {
        if (tstate_head == 0) {
            log_error("tstate_head is null");
            return -1;
        }
        void *dict;
        pytypecheck_read(dict, tstate_head + state->typecheck.o_PyThreadState_dict)
        if (dict != 0) {
            if (pytypecheck_obj(dict, state->typecheck.PyDict_Type)) {
                return -1;
            }
        }
        if (pytypecheck_glibc_header_size(state, tstate_head, state->typecheck.size_PyThreadState)) {
            return -1;
        }
    }
    log_debug("is = ok");
    return 0;
}

static __always_inline int pytypecheck_thread_state(py_sample_state_t *state, void *PyThreadState, bool check_interp) {
    log_debug("ts = %llx", PyThreadState);
    void *dict, *interp;
    pytypecheck_read(dict, PyThreadState + state->typecheck.o_PyThreadState_dict)
    pytypecheck_read(interp, PyThreadState + state->typecheck.o_PyThreadState_interp)
//    log_debug("ptc _thread_state dict=%llx interp=%llx", dict, interp);
    if (dict != 0) {
        if (pytypecheck_obj(dict, state->typecheck.PyDict_Type)) {
            return -1;
        };
    }

    if (pytypecheck_glibc_header_size(state, PyThreadState, state->typecheck.size_PyThreadState)) {
        return -1;
    }
    if (check_interp) {
        if (pytypecheck_interpreter_state(state, interp)) {
            return -1;
        }
    }

    return 0;
}


#else


#endif

#endif