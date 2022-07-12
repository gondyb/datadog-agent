#ifndef _CONTAINER_H_
#define _CONTAINER_H_

static __attribute__((always_inline)) void copy_container_id(const char src[CONTAINER_ID_LEN], char dst[CONTAINER_ID_LEN]) {
    __builtin_memmove(dst, src, CONTAINER_ID_LEN);
}

#endif
