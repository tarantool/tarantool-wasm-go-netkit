//go:build wasip1

package wasip1

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

func init() {
	net.DefaultResolver.Dial = DialContext
}

// Dialer is a type similar to net.Dialer but it uses the dial functions defined
// in this package instead of those from the standard library.
//
// For details about the configuration, see: https://pkg.go.dev/net#Dialer
//
// Note that depending on the WebAssembly runtime being employed, certain
// functionalities of the Dialer may not be available.
type Dialer struct {
	Timeout        time.Duration
	Deadline       time.Time
	LocalAddr      net.Addr
	DualStack      bool
	FallbackDelay  time.Duration
	Resolver       *net.Resolver   // ignored
	Cancel         <-chan struct{} // ignored
	Control        func(network, address string, c syscall.RawConn) error
	ControlContext func(ctx context.Context, network, address string, c syscall.RawConn) error
}

func (d *Dialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	timeout := d.Timeout
	if !d.Deadline.IsZero() {
		deadline := time.Until(d.Deadline)
		if timeout == 0 || deadline < timeout {
			timeout = deadline
		}
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if d.LocalAddr != nil {
		println("wasip1.Dialer: LocalAddr not yet supported on GOOS=wasip1")
	}
	if d.Resolver != nil {
		println("wasip1.Dialer: Resolver ignored because it is not supported on GOOS=wasip1")
	}
	if d.Cancel != nil {
		println("wasip1.Dialer: Cancel channel not implemented on GOOS=wasip1")
	}
	if d.Control != nil {
		println("wasip1.Dialer: Control function not yet supported on GOOS=wasip1")
	}
	if d.ControlContext != nil {
		println("wasip1.Dialer: ControlContext function not yet supported on GOOS=wasip1")
	}
	// TOOD:
	// - use LocalAddr to bind to a socket prior to establishing the connection
	// - use DualStack and FallbackDelay
	// - use Control and ControlContext functions
	// - emulate the Cancel channel with context.Context
	return DialContext(ctx, network, address)
}

// DialTimeout is not present in net.Dialer but this type provides it because it
// is useful to implement interfaces in popular network libraries such as the
// lib/pq Postgres client.
func (d *Dialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return d.DialContext(ctx, network, address)
}

// Dial connects to the address on the named network.
func Dial(network, address string) (net.Conn, error) {
	return DialContext(context.Background(), network, address)
}

// DialContext is a variant of Dial that accepts a context.
func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	addrs, err := LookupAddr(ctx, "dial", network, address)
	if err != nil {
		addr := &netAddr{network, address}
		return nil, dialErr(addr, err)
	}
	var addr net.Addr
	var conn net.Conn
	for _, addr = range addrs {
		conn, err = dialAddr(ctx, addr)
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, dialErr(addr, err)
}

func dialErr(addr net.Addr, err error) error {
	return newOpError("dial", addr, err)
}

func dialAddr(ctx context.Context, addr net.Addr) (net.Conn, error) {
	proto := family(addr)
	sotype, err := socketType(addr)
	if err != nil {
		return nil, os.NewSyscallError("socket", err)
	}
	fd, err := socket(proto, sotype, 0)
	if err != nil {
		return nil, os.NewSyscallError("socket", err)
	}
	defer func() {
		if fd >= 0 {
			syscall.Close(fd)
		}
	}()

	if err := setNonBlock(fd); err != nil {
		return nil, err
	}
	if sotype == SOCK_DGRAM && proto != AF_UNIX {
		if err := sockopt_set_broadcast(fd, 1); err != nil {
			// If the system does not support broadcast we should still be able
			// to use the datagram socket.
			switch {
			case errors.Is(err, syscall.EINVAL):
			case errors.Is(err, syscall.ENOPROTOOPT):
			default:
				return nil, os.NewSyscallError("setsockopt", err)
			}
		}
	}

	connectAddr, err := socketAddress(addr)
	if err != nil {
		return nil, os.NewSyscallError("sockaddr", err)
	}
	var inProgress bool
	switch err := connect(fd, connectAddr); err {
	case nil:
	case syscall.EINPROGRESS:
		inProgress = true
	default:
		return nil, os.NewSyscallError("connect", err)
	}

	if sotype == SOCK_DGRAM {
		name, err := getsockname(fd)
		if err != nil {
			return nil, err
		}
		peer, err := getpeername(fd)
		if err != nil {
			return nil, err
		}
		f := os.NewFile(uintptr(fd), "")
		fd = -1
		return makePacketConn(f, name, peer), nil
	}

	f := os.NewFile(uintptr(fd), "")
	fd = -1 // now the *os.File owns the file descriptor
	defer f.Close()

	if inProgress {
		rawConn, err := f.SyscallConn()
		if err != nil {
			return nil, fmt.Errorf("os.(*File).SyscallConn: %w", err)
		}

		errch := make(chan error)
		go func() {
			var err error
			rawConnErr := rawConn.Write(func(fd uintptr) bool {
				var value int
				value, err = syscall.Write(int(fd), make([]byte, 0))
				if err != nil {
					return true // done
				}
				switch syscall.Errno(value) {
				case syscall.EINPROGRESS, syscall.EINTR:
					return false // continue
				case syscall.EISCONN:
					err = nil
					return true
				case syscall.Errno(0):
					// The net poller can wake up spuriously. Check that we are
					// are really connected.
					_, err := getpeername(int(fd))
					return err == nil
				default:
					err = syscall.Errno(value)
					return true
				}
			})
			if err == nil {
				err = rawConnErr
			}
			errch <- err
		}()

		select {
		case err := <-errch:
			if err != nil {
				return nil, os.NewSyscallError("connect", err)
			}
		case <-ctx.Done():
			// This should interrupt the async connect operation handled by the
			// goroutine.
			f.Close()
			// Wait for the goroutine to complete, we can safely discard the
			// error here because we don't care about the socket anymore.
			<-errch
			return nil, context.Cause(ctx)
		}
	}

	c, err := net.FileConn(f)
	if err != nil {
		return nil, fmt.Errorf("net.FileConn: %w", err)
	}
	return makeConn(c)
}
