#define FUSE_USE_VERSION 31
#define FUSE_DARWIN_ENABLE_EXTENSIONS 0
#include <fuse.h>
#include <stdint.h>

int xd_fill(void *filler, void *buf, char *name, uint64_t ino, int isdir, int64_t size, int64_t ms, long mns);
int xd_run(char *mountpoint, int argc, char **argv);
void xd_exit(void);
int xd_available(void);
