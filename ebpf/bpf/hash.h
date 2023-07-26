#ifndef PYROSCOPE_HASH_H
#define PYROSCOPE_HASH_H
// Avoid pulling in any other headers.
typedef unsigned int uint32_t;

// murmurhash2 from
// https://github.com/aappleby/smhasher/blob/92cf3702fcfaadc84eb7bef59825a23e0cd84f56/src/MurmurHash2.cpp
static inline uint32_t MurmurHash2(const void *key, int len, uint32_t seed) {
    /* 'm' and 'r' are mixing constants generated offline.
       They're not really 'magic', they just happen to work well.  */

    const uint32_t m = 0x5bd1e995;
    const int r = 24;

    /* Initialize the hash to a 'random' value */

    uint32_t h = seed ^ len;

    /* Mix 4 bytes at a time into the hash */

    const unsigned char *data = (const unsigned char *)key;
    // MAX_STACK_DEPTH * 2 = 256 (because we hash 32 bits at a time).
//#pragma unroll
    for (int i = 0; i < 256; i++) {
        if (len < 4) {
            break;
        }
        uint32_t k = *(uint32_t *)data;

        k *= m;
        k ^= k >> r;
        k *= m;

        h *= m;
        h ^= k;

        data += 4;
        len -= 4;
    }

    /* Handle the last few bytes of the input array  */

    switch (len) {
        case 3:
            h ^= data[2] << 16;
        case 2:
            h ^= data[1] << 8;
        case 1:
            h ^= data[0];
            h *= m;
    };

    /* Do a few final mixes of the hash to ensure the last few
    // bytes are well-incorporated.  */

    h ^= h >> 13;
    h *= m;
    h ^= h >> 15;

    return h;
}

#endif