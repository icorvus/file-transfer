package discovery

import (
	"net"

	"github.com/grandcat/zeroconf"
)

// Publish only on active non-loopback interfaces. The certificate fingerprint
// is deliberately not a trust source here: the QR code establishes trust.
func Publish(name, id string, port int) (*zeroconf.Server, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	active := []net.Interface{}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagRunning != 0 && iface.Flags&net.FlagMulticast != 0 && iface.Flags&net.FlagLoopback == 0 {
			active = append(active, iface)
		}
	}
	return zeroconf.Register(name+"-"+id[:6], "_neardrop._tcp", "local.", port, []string{"v=1", "id=" + id}, active)
}
