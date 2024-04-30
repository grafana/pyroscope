#ifndef PYROEBPF_PYTYPECHECK_H
#define PYROEBPF_PYTYPECHECK_H

//#define PY_TYPECHECK_ENABLED

#if defined(PY_TYPECHECK_ENABLED)

struct py_object_header {
    ssize_t ob_refcnt;
    void *ob_type;
};


int pytypecheck_version_supported(py_sample_state_t  *state) {
    return state->version.minor == 8;
}

#define pytypecheck_version_check(state) if (!pytypecheck_version_supported(state)) { return 0; }

static __always_inline int pytypecheck_obj(void *o, uint64_t typ) {
    if (typ == 0 || o == 0) {
        log_error("[pytypecheck] obj expected type is null");
        return -1;
    }
    struct py_object_header obj = {};
    try_read(obj, o)
    log_debug("[pytypecheck] obj o=%llx ob_type = %llx refcount = %llx ", o, obj.ob_type, obj.ob_refcnt);
    if (obj.ob_refcnt < 0) {
        log_error("[pytypecheck] obj uaf");
        return -1;
    }
    if (obj.ob_type != (void *) typ) {
        log_error("[pytypecheck] obj type mismatch %llx %llx", obj.ob_type, typ);
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
        log_error("[pytypecheck] o=%llx  allocationSize=%llx mchunk_sz=%llx OK", o, allocationSize, mchunk_sz);
        return -1;
    }
    log_debug("[pytypecheck] o=%llx  allocationSize=%llx mchunk_sz=%llx", o, allocationSize, mchunk_sz);
    return 0;

}

static __always_inline int pytypecheck_interpreter_state(py_sample_state_t *state, void *is) {
    pytypecheck_version_check(state)
    log_debug("[pytypecheck] is = %llx", is);
    void *tstate_head = NULL;
    void *modules = NULL;
    void *importlib = NULL;
    uint32_t finalizing = 0;

    try_read(tstate_head, is + state->typecheck.o_PyInterpreterState_tstate_head)
    try_read(modules, is + state->typecheck.o_PyInterpreterState_modules)
    try_read(importlib, is + state->typecheck.o_PyInterpreterState_importlib)
    try_read(finalizing, is + state->typecheck.o_PyInterpreterState_finalizing)

    log_debug("[pytypecheck] ts = %llx modules = %llx importlib = %llx", tstate_head, modules, importlib);

    if (finalizing) {
        log_error("[pytypecheck] interpreter is finalizing");
        return -1;
    }
    try (pytypecheck_glibc_header_size(is, state->typecheck.size_PyInterpreterState))

    if (modules) {
        try (pytypecheck_obj(modules, state->typecheck.PyDict_Type))
    } else {
        log_debug("[pytypecheck] modules is null");
    }

    if (importlib) {

        try (pytypecheck_obj(importlib, state->typecheck.PyModule_Type))
    } else {
        log_debug("[pytypecheck] importlib is null");
    }

    log_debug("[pytypecheck] ts = %llx modules = %llx importlib = %llx", tstate_head, modules, importlib);
    {
        if (tstate_head == 0) {
            log_error("[pytypecheck] tstate_head is null");
            return -1;
        }
        void *dict;
        try_read(dict, tstate_head + state->typecheck.o_PyThreadState_dict)
        if (dict != 0) {
            try (pytypecheck_obj(dict, state->typecheck.PyDict_Type))
        }
        try (pytypecheck_glibc_header_size(tstate_head, state->typecheck.size_PyThreadState))
    }
    log_debug("[pytypecheck] is %llx ok", is);
    return 0;
}

static __always_inline int pytypecheck_thread_state(py_sample_state_t *state, void *ts, bool check_interp) {
    pytypecheck_version_check(state)
    log_debug("[pytypecheck] ts %llx", ts);
    void *dict=NULL, *interp=NULL;
    try_read(dict, ts + state->typecheck.o_PyThreadState_dict)
    try_read(interp, ts + state->typecheck.o_PyThreadState_interp)
    log_debug("[pytypecheck] %llx dict=%llx interp=%llx", ts, dict, interp);
    if (dict != 0) {
        if (pytypecheck_obj(dict, state->typecheck.PyDict_Type)) {
            return -1;
        };
    }

    try (pytypecheck_glibc_header_size(ts, state->typecheck.size_PyThreadState))
    
    if (check_interp) {
        if (pytypecheck_interpreter_state(state, interp)) {
            return -1;
        }
    }
    log_debug("[pytypecheck] ts %llx ok", ts);

    return 0;
}


static __always_inline int pytypecheck_frame(py_sample_state_t *state, void *f) {
    pytypecheck_version_check(state)
    if (f == 0) {
        log_debug("[pytypecheck] f %llx", f);
        return 0;
    }
    if (pytypecheck_obj(f, state->typecheck.PyFrame_Type)) {
        return -1;
    }
    log_debug("[pytypecheck] f %llx ok", f);
    return 0;
}

static __always_inline int pytypecheck_code(py_sample_state_t *state, void *code, void *frame) {
    pytypecheck_version_check(state)
    if (code == 0) {
        log_debug("[pytypecheck] code %llx null", code);
        return 0;
    }
    if (pytypecheck_obj(code, state->typecheck.PyCode_Type)) {
        return -1;
    }
    log_debug("[pytypecheck] code %llx ok", code);
    return 0;
}

static __always_inline int pytypecheck_tuple(py_sample_state_t *state, void *tuple) {
    pytypecheck_version_check(state)
    if (tuple == 0) {
        log_debug("[pytypecheck] tuple %llx null", tuple);
        return 0;
    }
    if (pytypecheck_obj(tuple, state->typecheck.PyTuple_Type)) {
        return -1;
    }
    log_debug("[pytypecheck] tuple %llx ok", tuple);
    return 0;
}

static __always_inline int pytypecheck_unicode(py_sample_state_t *state, void *tuple) {
    pytypecheck_version_check(state)
    if (tuple == 0) {
        log_debug("[pytypecheck] unicode %llx null", tuple);
        return 0;
    }
    if (pytypecheck_obj(tuple, state->typecheck.PyUnicode_Type)) {
        return -1;
    }
    log_debug("[pytypecheck] unicode %llx ok", tuple);
    return 0;
}

//PyTypeObject
static __always_inline int pytypecheck_typeobject(py_sample_state_t *state, void *typ) {
    pytypecheck_version_check(state)
    if (typ == 0) {
        log_debug("[pytypecheck] PyTypeObject null");
        return 0;
    }
    if (pytypecheck_obj(typ, state->typecheck.PyType_Type)) {
        return -1;
    }
    log_debug("[pytypecheck] PyTypeObject %llx ok", typ);
    return 0;
}




#else

#define pytypecheck_version_supported(state) 1
#define pytypecheck_version_check(state) 1
#define pytypecheck_obj(o, typ) 0
#define pytypecheck_glibc_header_size(o, allocationSize) 0
#define pytypecheck_interpreter_state(state, is) 0
#define pytypecheck_thread_state(state, ts, check_interp) 0
#define pytypecheck_frame(state, f) 0
#define pytypecheck_code(state, code, frame) 0
#define pytypecheck_tuple(state, tuple) 0
#define pytypecheck_unicode(state, tuple) 0
#define pytypecheck_typeobject(state, typ) 0


#endif

#endif