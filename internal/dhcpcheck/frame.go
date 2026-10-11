package dhcpcheck

import "encoding/binary"

// udpPayload returns the UDP data of an Ethernet frame carrying IPv4/UDP to
// port 68, and the frame's source MAC.
func udpPayload(f []byte) (payload, src []byte, ok bool) {
	if len(f) < 14+20 || binary.BigEndian.Uint16(f[12:]) != 0x0800 {
		return nil, nil, false
	}
	ip := f[14:]
	ihl := int(ip[0]&0x0f) * 4
	if ip[0]>>4 != 4 || ip[9] != 17 || len(ip) < ihl+8 || binary.BigEndian.Uint16(ip[ihl+2:]) != 68 {
		return nil, nil, false
	}
	end := len(ip)
	if total := int(binary.BigEndian.Uint16(ip[2:])); total >= ihl+8 && total < end {
		end = total // Ethernet padding
	}
	return ip[ihl+8 : end], f[6:12], true
}
