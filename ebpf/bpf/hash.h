

// murmurhash2 from
// https://github.com/aappleby/smhasher/blob/92cf3702fcfaadc84eb7bef59825a23e0cd84f56/src/MurmurHash2.cpp/*  */
// https://github.com/parca-dev/parca-agent/blob/main/bpf/unwinders/hash.h

// Hash limit in bytes, set to size of python stack
#define HASH_LIMIT 32 * 3 * 4
// len should be multiple of 4
static __always_inline uint64_t MurmurHash64A ( const void * key, uint64_t len, uint64_t seed )
{
    const uint64_t m = 0xc6a4a7935bd1e995ULL;
    const int r = 47;

    uint64_t h = seed ^ (len * m);

    const uint64_t * data = key;
    int i = 0;
    for (; i < len/8 && i < HASH_LIMIT/8; i++)
    {
        uint64_t k = data[i];

        k *= m;
        k ^= k >> r;
        k *= m;

        h ^= k;
        h *= m;
    }


    const unsigned char * data2 = (const unsigned char*)&data[i];
    if(len & 7)
    {
        h ^= (uint64_t)(((uint32_t*)data2)[0]);
        h *= m;
    };

    h ^= h >> r;
    h *= m;
    h ^= h >> r;

    return h;
}