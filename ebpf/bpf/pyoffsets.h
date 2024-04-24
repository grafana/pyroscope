//
// Created by korniltsev on 11/2/23.
//

#ifndef PYROEBPF_PYOFFSETS_H
#define PYROEBPF_PYOFFSETS_H

#include "stacks.h"


#define try_read(dst, src) \
    {if (bpf_probe_read_user(&(dst), sizeof((dst)), (src))) { \
        log_error("read failed  %llx (%s : %d)", (src), __FILE__, __LINE__); \
        return -1; \
    }}

#define try(code) \
    {if ((code)) { \
        log_error("try failed  %s : %d", __FILE__, __LINE__); \
        return -1; \
    }}


#define PYTHON_STACK_FRAMES_PER_PROG 32
#define PYTHON_STACK_PROG_CNT 3
#define PYTHON_STACK_MAX_LEN (PYTHON_STACK_FRAMES_PER_PROG * PYTHON_STACK_PROG_CNT)
#define PYTHON_CLASS_NAME_LEN 32
#define PYTHON_FUNCTION_NAME_LEN 64
#define PYTHON_FILE_NAME_LEN 128


enum frame_owner {
    FRAME_OWNED_BY_THREAD = 0,
    FRAME_OWNED_BY_GENERATOR = 1,
    FRAME_OWNED_BY_FRAME_OBJECT = 2,
    FRAME_OWNED_BY_CSTACK = 3,
};

struct libc {
    bool musl; //
    int16_t pthread_size;
    int16_t pthread_specific1stblock; // tsd for musl, specific_1stblock for glibc
};

typedef struct {
    int16_t PyThreadState_frame;
    int16_t PyThreadState_cframe;
    int16_t PyCFrame_current_frame;
    int16_t PyCodeObject_co_filename;
    int16_t PyCodeObject_co_name;
    int16_t PyCodeObject_co_varnames;
    int16_t PyCodeObject_co_localsplusnames;
    int16_t PyTupleObject_ob_item;

    int16_t PyVarObject_ob_size;
    int16_t PyObject_ob_type;
    int16_t PyTypeObject_tp_name;

    int16_t VFrame_code; // PyFrameObject_f_code pre 311 or PyInterpreterFrame_f_code post 311
    int16_t VFrame_previous; // PyFrameObject_f_back pre 311 or PyInterpreterFrame_previous post 311
    int16_t VFrame_localsplus; // PyFrameObject_localsplus pre 311 or PyInterpreterFrame_localsplus post 311
    int16_t PyInterpreterFrame_owner;
    int16_t PyASCIIObject_size; // sizeof(PyASCIIObject)
    int16_t PyCompactUnicodeObject_size; // sizeof(PyCompactUnicodeObject)

} py_offset_config;

typedef struct {
    uint64_t PyCode_Type;
    uint64_t PyFrame_Type;
    uint64_t PyBytes_Type;
    uint64_t PyUnicode_Type;
    uint64_t PyType_Type;
    uint64_t PyDict_Type;
    uint64_t PyNone_Type;
    uint64_t PyModule_Type;
    uint64_t PyTuple_Type;

    uint64_t o_PyThreadState_dict;
    uint64_t o_PyThreadState_interp;
    uint64_t size_PyThreadState;
    uint64_t o_PyInterpreterState_tstate_head;
    uint64_t o_PyInterpreterState_finalizing;
    uint64_t o_PyInterpreterState_modules;
    uint64_t o_PyInterpreterState_importlib;
    uint64_t size_PyInterpreterState_tstate;

} py_typecheck_data;


typedef uint32_t py_symbol_id;

typedef struct {
    struct sample_key k;
    uint32_t stack_len;
    // instead of storing symbol name here directly, we add it to another
    // hashmap with Symbols and only store the ids here
    py_symbol_id stack[PYTHON_STACK_MAX_LEN];
} py_event;

struct py_str_type {
    uint8_t type;
    uint8_t size_codepoints;
} ;

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

typedef struct {
    int64_t symbol_counter;
    py_offset_config offsets;
    py_typecheck_data typecheck;
    uint32_t cur_cpu;
    uint64_t frame_ptr;
    int64_t python_stack_prog_call_cnt;
    py_symbol sym;
    py_event event;
    uint64_t padding;// satisfy verifier for hash function
} py_sample_state_t;


#endif //PYROEBPF_PYOFFSETS_H
