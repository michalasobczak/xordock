// Most libfuse3 (macFUSE) → Go. Tylko tłumaczenie wywołań; logika jest
// w internal/fusefs, kryptografia w internal/vault.
#include "bridge.h"
#include <errno.h>
#include <string.h>
#include <unistd.h>
#include "_cgo_export.h"

static void fill_stat(struct stat *st, uint64_t ino, int isdir, int64_t size, int64_t ms, long mns) {
	memset(st, 0, sizeof *st);
	st->st_ino = ino;
	st->st_mode = isdir ? (S_IFDIR | 0700) : (S_IFREG | 0600);
	st->st_nlink = isdir ? 2 : 1;
	st->st_uid = getuid();
	st->st_gid = getgid();
	st->st_size = isdir ? 0 : size;
	st->st_blocks = (st->st_size + 511) / 512;
	st->st_blksize = 65536;
	struct timespec t = {.tv_sec = ms, .tv_nsec = mns};
	st->st_mtimespec = t;
	st->st_ctimespec = t;
	st->st_atimespec = t;
	st->st_birthtimespec = t;
}

int xd_fill(void *filler, void *buf, char *name, uint64_t ino, int isdir, int64_t size, int64_t ms, long mns) {
	struct stat st;
	fill_stat(&st, ino, isdir, size, ms, mns);
	return ((fuse_fill_dir_t)filler)(buf, name, &st, 0, 0);
}

static void *xd_init(struct fuse_conn_info *conn, struct fuse_config *cfg) {
	(void)conn;
	cfg->use_ino = 1;
	cfg->hard_remove = 1;
	cfg->attr_timeout = 1;
	cfg->entry_timeout = 1;
	cfg->negative_timeout = 0;
	cfg->kernel_cache = 0;
	return NULL;
}

static void xd_destroy(void *p) { (void)p; xdDestroy(); }

static int xd_getattr(const char *p, struct stat *st, struct fuse_file_info *fi) {
	(void)fi;
	uint64_t ino; int d; int64_t sz, ms; long mns;
	int r = xdGetattr((char *)p, &ino, &d, &sz, &ms, &mns);
	if (r) return r;
	fill_stat(st, ino, d, sz, ms, mns);
	return 0;
}

static int xd_readdir(const char *p, void *buf, fuse_fill_dir_t filler, off_t off,
		      struct fuse_file_info *fi, enum fuse_readdir_flags fl) {
	(void)off; (void)fi; (void)fl;
	filler(buf, ".", NULL, 0, 0);
	filler(buf, "..", NULL, 0, 0);
	return xdReaddir((char *)p, buf, (void *)filler);
}

static int xd_mkdir(const char *p, mode_t m) { (void)m; return xdMkdir((char *)p); }
static int xd_unlink(const char *p) { return xdUnlink((char *)p); }
static int xd_rmdir(const char *p) { return xdRmdir((char *)p); }
static int xd_rename(const char *a, const char *b, unsigned int fl) { return xdRename((char *)a, (char *)b, fl); }

static int xd_open(const char *p, struct fuse_file_info *fi) {
	uint64_t fh;
	int r = xdOpen((char *)p, (fi->flags & O_TRUNC) ? 1 : 0, &fh);
	if (r) return r;
	fi->fh = fh;
	return 0;
}

static int xd_create(const char *p, mode_t m, struct fuse_file_info *fi) {
	(void)m;
	uint64_t fh;
	int r = xdCreate((char *)p, &fh);
	if (r) return r;
	fi->fh = fh;
	return 0;
}

static int xd_read(const char *p, char *buf, size_t size, off_t off, struct fuse_file_info *fi) {
	(void)p;
	return xdRead(fi->fh, buf, size, off);
}

static int xd_write(const char *p, const char *buf, size_t size, off_t off, struct fuse_file_info *fi) {
	(void)p;
	return xdWrite(fi->fh, (char *)buf, size, off);
}

static int xd_truncate(const char *p, off_t size, struct fuse_file_info *fi) {
	return xdTruncate((char *)p, fi ? fi->fh : 0, size);
}

static int xd_flush(const char *p, struct fuse_file_info *fi) { (void)p; return xdFlush(fi->fh); }
static int xd_fsync(const char *p, int ds, struct fuse_file_info *fi) { (void)p; (void)ds; return xdFlush(fi->fh); }
static int xd_release(const char *p, struct fuse_file_info *fi) { (void)p; return xdRelease(fi->fh); }

static int xd_utimens(const char *p, const struct timespec tv[2], struct fuse_file_info *fi) {
	(void)fi;
	if (tv == NULL) return xdUtimens((char *)p, 1, 0, 0);
	if (tv[1].tv_nsec == UTIME_OMIT) return 0;
	if (tv[1].tv_nsec == UTIME_NOW) return xdUtimens((char *)p, 1, 0, 0);
	return xdUtimens((char *)p, 0, tv[1].tv_sec, tv[1].tv_nsec);
}

// Uprawnień i właściciela nie przechowujemy: każdy element ma 0600/0700
// i właściciela montującego. FSKit ustawia tryb przy każdym tworzeniu
// elementu, więc błąd zablokowałby mkdir i create. Odstępstwo od F-03
// opisane w S004 §9 (A-06). Element musi istnieć.
static int xd_chmod(const char *p, mode_t m, struct fuse_file_info *fi) { (void)m; (void)fi; return xdExists((char *)p); }
static int xd_chown(const char *p, uid_t u, gid_t g, struct fuse_file_info *fi) { (void)u; (void)g; (void)fi; return xdExists((char *)p); }

static int xd_statfs(const char *p, struct statvfs *s) {
	(void)p;
	uint64_t total, avail;
	xdStatfs(&total, &avail);
	memset(s, 0, sizeof *s);
	s->f_bsize = 4096;
	s->f_frsize = 4096;
	s->f_blocks = total / 4096;
	s->f_bfree = avail / 4096;
	s->f_bavail = avail / 4096;
	s->f_files = 1000000;
	s->f_ffree = 1000000;
	s->f_namemax = 255;
	return 0;
}

static const struct fuse_operations xd_ops = {
	.init = xd_init,
	.destroy = xd_destroy,
	.getattr = xd_getattr,
	.readdir = xd_readdir,
	.mkdir = xd_mkdir,
	.unlink = xd_unlink,
	.rmdir = xd_rmdir,
	.rename = xd_rename,
	.open = xd_open,
	.create = xd_create,
	.read = xd_read,
	.write = xd_write,
	.truncate = xd_truncate,
	.flush = xd_flush,
	.fsync = xd_fsync,
	.release = xd_release,
	.utimens = xd_utimens,
	.chmod = xd_chmod,
	.chown = xd_chown,
	.statfs = xd_statfs,
};

static struct fuse *xd_fuse;

/* S011 W-02: libfuse3 z macFUSE ładowana przy pierwszym użyciu (dlopen),
 * a nie przez loader. Bez macFUSE CLI startuje i działa poza montowaniem.
 * Ścieżka jest stała (instalacja macFUSE), bez przeszukiwania DYLD_*. */
#include <dlfcn.h>

static int xd_loaded = -1;
static struct fuse *(*p_new_31)(struct fuse_args *, const struct fuse_operations *, size_t,
                                struct libfuse_version *, void *);
static int (*p_mount)(struct fuse *, const char *);
static int (*p_loop)(struct fuse *);
static void (*p_unmount)(struct fuse *);
static void (*p_destroy)(struct fuse *);
static void (*p_exit)(struct fuse *);

int xd_available(void) {
	if (xd_loaded >= 0) return xd_loaded;
	xd_loaded = 0;
	void *h = dlopen("/usr/local/lib/libfuse3.4.dylib", RTLD_NOW | RTLD_LOCAL);
	if (h == NULL) return 0;
	p_new_31 = dlsym(h, "_fuse_new_31");
	p_mount = dlsym(h, "fuse_mount");
	p_loop = dlsym(h, "fuse_loop");
	p_unmount = dlsym(h, "fuse_unmount");
	p_destroy = dlsym(h, "fuse_destroy");
	p_exit = dlsym(h, "fuse_exit");
	if (p_new_31 && p_mount && p_loop && p_unmount && p_destroy && p_exit) xd_loaded = 1;
	return xd_loaded;
}

int xd_run(char *mountpoint, int argc, char **argv) {
	if (!xd_available()) return -3;
	struct fuse_args args = FUSE_ARGS_INIT(argc, argv);
	struct libfuse_version version = {
		.major = FUSE_MAJOR_VERSION,
		.minor = FUSE_MINOR_VERSION,
		.hotfix = FUSE_HOTFIX_VERSION,
		.darwin_extensions_enabled = FUSE_DARWIN_ENABLE_EXTENSIONS,
		.padding = 0,
	};
	xd_fuse = p_new_31(&args, &xd_ops, sizeof(xd_ops), &version, NULL);
	if (xd_fuse == NULL) return -1;
	if (p_mount(xd_fuse, mountpoint) != 0) {
		p_destroy(xd_fuse);
		xd_fuse = NULL;
		return -2;
	}
	int r = p_loop(xd_fuse);
	p_unmount(xd_fuse);
	p_destroy(xd_fuse);
	xd_fuse = NULL;
	return r;
}

void xd_exit(void) {
	if (xd_fuse && p_exit) p_exit(xd_fuse);
}
