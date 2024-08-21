//go:build wasip1

package wasip1

// This file contains the definition of host imports compatible with the socket
// extensions from wasmedge v0.12+.

import (
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const (
	AF_INET = iota
	AF_INET6
	AF_UNIX
)

const (
	SOCK_ANY = iota
	SOCK_DGRAM
	SOCK_STREAM
)

const (
	SOL_SOCKET = iota
)

const (
	SO_REUSEADDR = iota
	_
	SO_ERROR
	_
	SO_BROADCAST
)

const (
	AI_PASSIVE = 1 << iota
	_
	AI_NUMERICHOST
	AI_NUMERICSERV
)

const (
	IPPROTO_IP = iota
	IPPROTO_TCP
	IPPROTO_UDP
)

type sockaddr interface {
	sockaddr() (unsafe.Pointer, error)
	sockport() int
}

type sockaddrInet4 struct {
	kind uint32
	addr [4]byte
	port uint32
	raw  addressBuffer
}

func (s *sockaddrInet4) sockaddr() (unsafe.Pointer, error) {
	s.raw.bufLen = 4
	s.raw.buf = uintptr32(uintptr(unsafe.Pointer(&s.kind)))
	return unsafe.Pointer(s), nil
}

func (s *sockaddrInet4) sockport() int {
	return int(s.port)
}

type sockaddrInet6 struct {
	kind uint32
	addr [16]byte
	port uint32
	zone uint32
	raw  addressBuffer
}

func (s *sockaddrInet6) sockaddr() (unsafe.Pointer, error) {
	if s.zone != 0 {
		return nil, syscall.ENOTSUP
	}
	s.raw.bufLen = 16
	s.raw.buf = uintptr32(uintptr(unsafe.Pointer(&s.kind)))
	return unsafe.Pointer(&s.raw), nil
}

func (s *sockaddrInet6) sockport() int {
	return int(s.port)
}

type sockaddrUnix struct {
	name string

	raw rawSockaddrAny
	buf addressBuffer
}

func (s *sockaddrUnix) sockaddr() (unsafe.Pointer, error) {
	s.raw.family = AF_UNIX
	if len(s.name) >= len(s.raw.addr)-1 {
		return nil, syscall.EINVAL
	}
	copy(s.raw.addr[:], s.name)
	s.raw.addr[len(s.name)] = 0
	s.buf.bufLen = 128
	s.buf.buf = uintptr32(uintptr(unsafe.Pointer(&s.raw)))
	return unsafe.Pointer(&s.buf), nil
}

func (s *sockaddrUnix) sockport() int {
	return 0
}

type uintptr32 = uint32
type size = uint32

type addressBuffer struct {
	buf    uintptr32
	bufLen size
}

type rawSockaddrAny struct {
	family uint16
	addr   [126]byte
}

// poolfd is unused for now
//
//go:wasmimport wasi_snapshot_preview1 sock_open
//go:noescape
func sock_open(poolfd int32, af int32, socktype int32, fd unsafe.Pointer) syscall.Errno

//go:wasmimport wasi_snapshot_preview1 sock_bind
//go:noescape
func sock_bind(fd int32, addr unsafe.Pointer) syscall.Errno

//go:wasmimport wasi_snapshot_preview1 sock_listen
//go:noescape
func sock_listen(fd int32, backlog int32) syscall.Errno

//go:wasmimport wasi_snapshot_preview1 sock_connect
//go:noescape
func sock_connect(fd int32, addr unsafe.Pointer) syscall.Errno

//go:wasmimport wasi_snapshot_preview1 sock_set_reuse_addr
//go:noescape
func sock_set_reuse_addr(fd int32, isEnabled int32) syscall.Errno

//go:wasmimport wasi_snapshot_preview1 sock_set_broadcast
//go:noescape
func sock_set_broadcast(fd int32, isEnabled int32) syscall.Errno

//go:wasmimport wasi_snapshot_preview1 sock_addr_local
//go:noescape
func sock_addr_local(fd int32, addr unsafe.Pointer) syscall.Errno

//go:wasmimport wasi_snapshot_preview1 sock_addr_remote
//go:noescape
func sock_addr_remote(fd int32, addr unsafe.Pointer) syscall.Errno

//go:wasmimport wasi_snapshot_preview1 sock_recv_from
//go:noescape
func sock_recv_from(
	fd int32,
	iovs unsafe.Pointer,
	iovsCount int32,
	iflags int32,
	addr unsafe.Pointer,
	nread unsafe.Pointer,
) syscall.Errno

//go:wasmimport wasi_snapshot_preview1 sock_send_to
//go:noescape
func sock_send_to(
	fd int32,
	iovs unsafe.Pointer,
	iovsCount int32,
	addr unsafe.Pointer,
	port int32,
	flags int32,
	nwritten unsafe.Pointer,
) syscall.Errno

//go:wasmimport wasi_snapshot_preview1 sock_addr_resolve
//go:noescape
func sock_addr_resolve(
	node unsafe.Pointer,
	service unsafe.Pointer,
	hints unsafe.Pointer,
	res unsafe.Pointer,
	maxResLen uint32,
	resLen unsafe.Pointer,
) syscall.Errno

//go:wasmimport wasi_snapshot_preview1 sock_shutdown
func sock_shutdown(fd, how int32) syscall.Errno

func socket(proto, sotype, unused int) (fd int, err error) {
	var newfd int32
	errno := sock_open(0, int32(proto), int32(sotype), unsafe.Pointer(&newfd))
	if errno != 0 {
		return -1, errno
	}
	return int(newfd), nil
}

func bind(fd int, sa sockaddr) error {
	rawaddr, err := sa.sockaddr()
	if err != nil {
		return err
	}
	errno := sock_bind(int32(fd), rawaddr)
	runtime.KeepAlive(sa)
	if errno != 0 {
		return errno
	}
	return nil
}

func listen(fd int, backlog int) error {
	if errno := sock_listen(int32(fd), int32(backlog)); errno != 0 {
		return errno
	}
	return nil
}

func connect(fd int, sa sockaddr) error {
	rawaddr, err := sa.sockaddr()
	if err != nil {
		return err
	}
	errno := sock_connect(int32(fd), rawaddr)
	runtime.KeepAlive(sa)
	if errno != 0 {
		return errno
	}
	return nil
}

type iovec struct {
	ptr uintptr32
	len uint32
}

func recvfrom(fd int, iovs [][]byte, flags int32) (n int, addr rawSockaddrAny, port, oflags int32, err error) {
	iovsBuf := make([]iovec, 0, 8)
	for _, iov := range iovs {
		iovsBuf = append(iovsBuf, iovec{
			ptr: uintptr32(uintptr(unsafe.Pointer(unsafe.SliceData(iov)))),
			len: uint32(len(iov)),
		})
	}
	addrBuf := addressBuffer{
		buf:    uintptr32(uintptr(unsafe.Pointer(&addr))),
		bufLen: uint32(unsafe.Sizeof(addr)),
	}
	nread := int32(0)
	errno := sock_recv_from(
		int32(fd),
		unsafe.Pointer(unsafe.SliceData(iovsBuf)),
		int32(len(iovsBuf)),
		flags,
		unsafe.Pointer(&addrBuf),
		unsafe.Pointer(&nread),
	)
	if errno != 0 {
		return int(nread), addr, port, oflags, errno
	}
	runtime.KeepAlive(addrBuf)
	runtime.KeepAlive(iovsBuf)
	runtime.KeepAlive(iovs)
	return int(nread), addr, port, oflags, nil
}

func sendto(fd int, iovs [][]byte, addr rawSockaddrAny, port, flags int32) (int, error) {
	iovsBuf := make([]iovec, 0, 8)
	for _, iov := range iovs {
		iovsBuf = append(iovsBuf, iovec{
			ptr: uintptr32(uintptr(unsafe.Pointer(unsafe.SliceData(iov)))),
			len: uint32(len(iov)),
		})
	}
	addrBuf := addressBuffer{
		buf:    uintptr32(uintptr(unsafe.Pointer(&addr))),
		bufLen: uint32(unsafe.Sizeof(addr)),
	}
	nwritten := int32(0)
	errno := sock_send_to(
		int32(fd),
		unsafe.Pointer(unsafe.SliceData(iovsBuf)),
		int32(len(iovsBuf)),
		unsafe.Pointer(&addrBuf),
		port,
		flags,
		unsafe.Pointer(&nwritten),
	)
	if errno != 0 {
		return int(nwritten), errno
	}
	runtime.KeepAlive(addrBuf)
	runtime.KeepAlive(iovsBuf)
	runtime.KeepAlive(iovs)
	return int(nwritten), nil
}

func shutdown(fd, how int) error {
	if errno := sock_shutdown(int32(fd), int32(how)); errno != 0 {
		return errno
	}
	return nil
}

func sockopt_set_broadcast(fd int, value int) error {
	errno := sock_set_broadcast(int32(fd), int32(value))
	if errno != 0 {
		return errno
	}
	return nil
}

func sockopt_set_reuse_addr(fd int, value int) error {
	errno := sock_set_reuse_addr(int32(fd), int32(value))
	if errno != 0 {
		return errno
	}
	return nil
}

func getsockname(fd int) (sa sockaddr, err error) {
	var rsa rawSockaddrAny
	buf := addressBuffer{
		buf:    uintptr32(uintptr(unsafe.Pointer(&rsa))),
		bufLen: uint32(unsafe.Sizeof(rsa)),
	}
	var port uint32
	errno := sock_addr_local(int32(fd), unsafe.Pointer(&buf))
	if errno != 0 {
		return nil, errno
	}
	return anyToSockaddr(&rsa, port)
}

func getpeername(fd int) (sockaddr, error) {
	var rsa rawSockaddrAny
	buf := addressBuffer{
		buf:    uintptr32(uintptr(unsafe.Pointer(&rsa))),
		bufLen: uint32(unsafe.Sizeof(rsa)),
	}
	var port uint32
	errno := sock_addr_remote(int32(fd), unsafe.Pointer(&buf))
	if errno != 0 {
		return nil, errno
	}
	return anyToSockaddr(&rsa, port)
}

func anyToSockaddr(rsa *rawSockaddrAny, port uint32) (sockaddr, error) {
	switch rsa.family {
	case AF_INET:
		addr := sockaddrInet4{kind: AF_INET, port: port}
		copy(addr.addr[:], rsa.addr[:])
		return &addr, nil
	case AF_INET6:
		addr := sockaddrInet6{kind: AF_INET6, port: port}
		copy(addr.addr[:], rsa.addr[:])
		return &addr, nil
	case AF_UNIX:
		addr := sockaddrUnix{}
		addr.name = string(rsa.addr[:strlen(rsa.addr[:])])
		return &addr, nil
	default:
		return nil, syscall.ENOTSUP
	}
}

func strlen(b []byte) (n int) {
	for n < len(b) && b[n] != 0 {
		n++
	}
	return n
}

type wasiAddrIP6Port struct {
	IPBuf [16]byte
	port  uint16
}

type wasiAddrIP4Port struct {
	IPBuf [4]byte
	port  uint16
}

type wasiAddrInfo struct {
	sockkind uint32
	addrBuf  [18]byte // sizeof wasiAddrIP6Port
	socktype uint32
}

type wasiAddrInfoHints struct {
	socktype     uint32
	family       uint32
	hintsEnabled uint8
}

func getaddrinfo(name, service string, hints *wasiAddrInfoHints, results []wasiAddrInfo) (int, error) {
	resPtr := unsafe.Pointer(unsafe.SliceData(results))
	// For compatibility with WasmEdge, make sure strings are null-terminated.
	namePtr, _ := nullTerminatedString(name)
	servPtr, _ := nullTerminatedString(service)

	var n uint32 = 0

	errno := sock_addr_resolve(
		unsafe.Pointer(namePtr),
		unsafe.Pointer(servPtr),
		unsafe.Pointer(hints),
		resPtr,
		uint32(len(results)),
		unsafe.Pointer(&n),
	)
	if errno != 0 {
		return 0, errno
	}

	return int(n), nil
}

func nullTerminatedString(s string) (*byte, int) {
	if n := strings.IndexByte(s, 0); n >= 0 {
		s = s[:n+1]
		return unsafe.StringData(s), len(s)
	} else {
		b := append([]byte(s), 0)
		return unsafe.SliceData(b), len(b)
	}
}
