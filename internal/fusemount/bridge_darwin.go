// Package fusemount montuje sejf przez macFUSE (libfuse3, backend FSKit,
// S004 ADR-003). Wymaga zainstalowanego macFUSE 5.5+.
package fusemount

/*
#cgo CFLAGS: -I/usr/local/include/fuse3 -D_FILE_OFFSET_BITS=64
#cgo LDFLAGS: -ldl
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"syscall"
	"time"
	"unsafe"

	"xordock/internal/fusefs"
	"xordock/internal/vault"
)

// cur to zamontowany system plików (jeden na proces).
var cur *fusefs.FS

func neg(e syscall.Errno) C.int { return -C.int(e) }

func gostr(p *C.char) string { return C.GoString(p) }

//export xdDestroy
func xdDestroy() {
	if cur != nil && !cur.IsProbe() {
		if err := cur.SyncAll(); err != nil {
			vault.Logf("xordock: zatwierdzenie przy odmontowaniu: %v", err)
		}
	}
}

//export xdGetattr
func xdGetattr(p *C.char, ino *C.uint64_t, dir *C.int, size *C.int64_t, ms *C.int64_t, mns *C.long) C.int {
	a, e := cur.Getattr(gostr(p))
	if e != 0 {
		return neg(e)
	}
	*ino = C.uint64_t(a.Ino)
	*dir = 0
	if a.Dir {
		*dir = 1
	}
	*size = C.int64_t(a.Size)
	*ms = C.int64_t(a.Mtime.Unix())
	*mns = C.long(a.Mtime.Nanosecond())
	return 0
}

//export xdReaddir
func xdReaddir(p *C.char, buf unsafe.Pointer, filler unsafe.Pointer) C.int {
	ents, e := cur.Readdir(gostr(p))
	if e != 0 {
		return neg(e)
	}
	for _, d := range ents {
		name := C.CString(d.Name)
		isdir := C.int(0)
		if d.Attr.Dir {
			isdir = 1
		}
		full := C.xd_fill(filler, buf, name, C.uint64_t(d.Attr.Ino), isdir, C.int64_t(d.Attr.Size),
			C.int64_t(d.Attr.Mtime.Unix()), C.long(d.Attr.Mtime.Nanosecond()))
		C.free(unsafe.Pointer(name))
		if full != 0 {
			break
		}
	}
	return 0
}

//export xdExists
func xdExists(p *C.char) C.int {
	_, e := cur.Getattr(gostr(p))
	return neg(e)
}

//export xdMkdir
func xdMkdir(p *C.char) C.int { return neg(cur.Mkdir(gostr(p))) }

//export xdUnlink
func xdUnlink(p *C.char) C.int { return neg(cur.Unlink(gostr(p))) }

//export xdRmdir
func xdRmdir(p *C.char) C.int { return neg(cur.Rmdir(gostr(p))) }

//export xdRename
func xdRename(a, b *C.char, flags C.uint) C.int {
	return neg(cur.Rename(gostr(a), gostr(b), uint(flags)))
}

//export xdOpen
func xdOpen(p *C.char, trunc C.int, fh *C.uint64_t) C.int {
	h, e := cur.Open(gostr(p), trunc != 0)
	if e != 0 {
		return neg(e)
	}
	*fh = C.uint64_t(h)
	return 0
}

//export xdCreate
func xdCreate(p *C.char, fh *C.uint64_t) C.int {
	h, e := cur.Create(gostr(p))
	if e != 0 {
		return neg(e)
	}
	*fh = C.uint64_t(h)
	return 0
}

//export xdRead
func xdRead(fh C.uint64_t, buf *C.char, size C.size_t, off C.off_t) C.int {
	data, e := cur.Read(uint64(fh), int64(off), int(size))
	if e != 0 {
		return neg(e)
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(size))
	n := copy(dst, data)
	clear(data)
	return C.int(n)
}

//export xdWrite
func xdWrite(fh C.uint64_t, buf *C.char, size C.size_t, off C.off_t) C.int {
	src := unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(size))
	n, e := cur.Write(uint64(fh), int64(off), src)
	if e != 0 {
		return neg(e)
	}
	return C.int(n)
}

//export xdTruncate
func xdTruncate(p *C.char, fh C.uint64_t, size C.off_t) C.int {
	return neg(cur.Truncate(gostr(p), uint64(fh), int64(size)))
}

//export xdFlush
func xdFlush(fh C.uint64_t) C.int { return neg(cur.Flush(uint64(fh))) }

//export xdRelease
func xdRelease(fh C.uint64_t) C.int { return neg(cur.Release(uint64(fh))) }

//export xdUtimens
func xdUtimens(p *C.char, now C.int, sec C.int64_t, nsec C.long) C.int {
	t := time.Now()
	if now == 0 {
		t = time.Unix(int64(sec), int64(nsec))
	}
	return neg(cur.Utimens(gostr(p), t))
}

//export xdStatfs
func xdStatfs(total, avail *C.uint64_t) {
	var st syscall.Statfs_t
	*total, *avail = 0, 0
	if cur == nil || syscall.Statfs(cur.BRoot(), &st) != nil {
		return
	}
	// Pojemność wyznacza karta: każdy bajt danych to udział na niej.
	*total = C.uint64_t(st.Blocks * uint64(st.Bsize))
	*avail = C.uint64_t(st.Bavail * uint64(st.Bsize))
}

// run montuje i obsługuje żądania do odmontowania (blokuje).
func run(fs *fusefs.FS, mountpoint string, opts string) error {
	cur = fs
	argv := []*C.char{C.CString("xordock"), C.CString("-o"), C.CString(opts)}
	defer func() {
		for _, a := range argv {
			C.free(unsafe.Pointer(a))
		}
	}()
	mp := C.CString(mountpoint)
	defer C.free(unsafe.Pointer(mp))
	switch r := C.xd_run(mp, C.int(len(argv)), &argv[0]); r {
	case 0:
		return nil
	case -1:
		return errFuseNew
	case -2:
		return errMount
	case -3:
		return ErrNoMacFUSE
	default:
		return syscall.Errno(-r)
	}
}

func exitLoop() { C.xd_exit() }

// Available: biblioteka libfuse3 z macFUSE daje się załadować (S011 W-02).
// CLI ładuje ją dopiero przy montowaniu (dlopen), więc bez macFUSE działa
// wszystko poza montowaniem.
func Available() bool { return C.xd_available() != 0 }
