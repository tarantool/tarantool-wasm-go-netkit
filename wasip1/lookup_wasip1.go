//go:build wasip1

package wasip1

import (
	"context"
	"net"
	"os"
	"unsafe"
)

func lookupAddr(ctx context.Context, op, network, address string) ([]net.Addr, error) {
	var hints wasiAddrInfoHints

	switch network {
	case "tcp", "tcp4", "tcp6":
		hints.socktype = SOCK_STREAM
	case "udp", "udp4", "udp6":
		hints.socktype = SOCK_DGRAM
	case "unix", "unixgram":
		return []net.Addr{&net.UnixAddr{Name: address, Net: network}}, nil
	default:
		return nil, net.UnknownNetworkError(network)
	}

	switch network {
	case "tcp", "udp":
		hints.family = AF_INET
	case "tcp4", "udp4":
		hints.family = AF_INET
	case "tcp6", "udp6":
		hints.family = AF_INET6
	}

	hostname, service, err := net.SplitHostPort(address)
	results := make([]wasiAddrInfo, 8)
	n, err := getaddrinfo(hostname, service, &hints, results)
	if err != nil {
		addr := &netAddr{network, address}
		return nil, newOpError(op, addr, os.NewSyscallError("getaddrinfo", err))
	}

	addrs := make([]net.Addr, 0, n)
	for _, r := range results[:n] {
		var ip net.IP
		var port int
		addrPtr := unsafe.Pointer(unsafe.SliceData(r.addrBuf[:]))
		switch r.sockkind {
		case AF_INET:
			addrIP4 := *(*wasiAddrIP4Port)(addrPtr)
			ip = addrIP4.IPBuf[:]
			port = int(addrIP4.port)
		case AF_INET6:
			addrIP6 := *(*wasiAddrIP6Port)(addrPtr)
			ip = addrIP6.IPBuf[:]
			port = int(addrIP6.port)
		}

		switch network {
		case "tcp", "tcp4", "tcp6":
			addrs = append(addrs, &net.TCPAddr{IP: ip, Port: port})
		case "udp", "udp4", "udp6":
			addrs = append(addrs, &net.UDPAddr{IP: ip, Port: port})
		}
	}
	if len(addrs) != 0 {
		return addrs, nil
	}

	return nil, &net.DNSError{
		Err:        "lookup failed",
		Name:       hostname,
		IsNotFound: true,
	}
}
