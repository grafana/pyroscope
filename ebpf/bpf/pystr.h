//
// Created by korniltsev on 11/2/23.
//

#ifndef PYROEBPF_PYSTR_H
#define PYROEBPF_PYSTR_H

#include "pyoffsets.h"

#define PYSTR_TYPE_1BYTE  1
#define PYSTR_TYPE_2BYTE  2
#define PYSTR_TYPE_4BYTE  4
#define PYSTR_TYPE_ASCII  8
#define PYSTR_TYPE_UTF8   16
#define PYSTR_TYPE_NOT_COMPACT  32


struct py_str_type {
    uint8_t type;
    uint8_t size_codepoints;
} ;


struct _object {
    union {
        u64         ob_refcnt;            /*     0     8 */
        uint32_t           ob_refcnt_split[2];   /*     0     8 */
    };                                               /*     0     8 */
    void *             ob_type;              /*     8     8 */
};


// Note: it is incomplete
// Some state fields may be ommited
// Also wstr field is ommited
// Only first 32 bytes  are here
typedef struct {
    struct _object                   ob_base;              /*     0    16 */
    u64                 length;               /*    16     8 */
    u64                  hash;                 /*    24     8 */
    struct {
        unsigned int       interned:2;           /*    32: 0  4 */
        unsigned int       kind:3;               /*    32: 2  4 */
        unsigned int       compact:1;            /*    32: 5  4 */
        unsigned int       ascii:1;              /*    32: 6  4 */
    } state;                                         /*    32     4 */
} PyASCIIObject;

// Read compact strings from PyASCIIObject or PyCompactUnicodeObject
static __always_inline int pystr_read(void *str, py_offset_config *offsets, char *buf, u64 buf_size, struct py_str_type *typ) {
    PyASCIIObject pystr = {};
    if (bpf_probe_read_user(&pystr, sizeof(PyASCIIObject), str)) {
        return -1;
    }
    if (pystr.state.compact == 0) { // not implemented, skip
        typ->type = PYSTR_TYPE_NOT_COMPACT;
        return 0;
    }
    u64 sz_bytes = pystr.state.kind * pystr.length;
    if (sz_bytes > buf_size) {
        sz_bytes = buf_size;
        typ->size_codepoints = sz_bytes/pystr.state.kind;
    } else {
        typ->size_codepoints = pystr.length;
    }
    void *data;
    if (pystr.state.ascii) {
        typ->type = pystr.state.kind | PYSTR_TYPE_ASCII;
        data = str + offsets->PyASCIIObject_size;
    } else {
        typ->type = pystr.state.kind;
        data = str + offsets->PyCompactUnicodeObject_size;
    }

    if (bpf_probe_read_user(buf, sz_bytes, data)) {
        return -1;
    };
    return 0;
}

#endif //PYROEBPF_PYSTR_H
