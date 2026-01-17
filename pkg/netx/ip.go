package netx

import (
	"net"
)

func GetOutboundIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		panic("no valid IPv4 address found")
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String()
		}
	}

	panic("no valid IPv4 address found")
}
