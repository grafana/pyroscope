//
// Created by korniltsev on 11/2/23.
//

#ifndef PYROEBPF_PYOFFSETS_H
#define PYROEBPF_PYOFFSETS_H



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

#endif //PYROEBPF_PYOFFSETS_H
