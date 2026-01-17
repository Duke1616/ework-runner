package netx

import (
	"fmt"
	"net"
)

// GetOutboundIP 获得对外发送消息的 IP 地址
func GetOutboundIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String(), nil
		}
	}

	return "", fmt.Errorf("no valid IPv4 address found")
}
