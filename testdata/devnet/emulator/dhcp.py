"""The HQ gateway's DHCP server, for Omini's DHCP check: it answers a
DHCPDISCOVER with an offer (never more: Omini sends no REQUEST), broadcast
to port 68 like a real server answering a client without an address."""

from __future__ import annotations

import asyncio
import ipaddress
import logging
import socket
import struct
from typing import Any

log = logging.getLogger("devnet")

GATEWAY = "10.10.10.1"  # HQ's CARP VIP on MGMT, where Unbound and the DHCP server run
DOMAIN = b"acme.local"
LEASE_S = 86400


def offer_for(discover: bytes, offered: str) -> bytes | None:
    """The DHCPOFFER for a DISCOVER (None for anything else)."""
    if len(discover) < 240 or discover[0] != 1 or discover[236:240] != b"\x63\x82\x53\x63":
        return None
    i, msg_type = 240, 0
    while i + 1 < len(discover) and discover[i] != 255:
        if discover[i] == 0:
            i += 1
            continue
        if discover[i] == 53 and discover[i + 1] == 1:
            msg_type = discover[i + 2]
        i += 2 + discover[i + 1]
    if msg_type != 1:  # only a DISCOVER
        return None
    gw = ipaddress.IPv4Address(GATEWAY).packed
    out = bytearray(240)
    out[0:4] = bytes([2, 1, 6, 0])
    out[4:8] = discover[4:8]  # xid
    out[10:12] = discover[10:12]  # flags
    out[16:20] = ipaddress.IPv4Address(offered).packed  # yiaddr
    out[20:24] = gw  # siaddr
    out[28:44] = discover[28:44]  # chaddr
    out[236:240] = b"\x63\x82\x53\x63"
    out += bytes([53, 1, 2, 54, 4, *gw, 1, 4, 255, 255, 255, 0, 3, 4, *gw, 6, 4, *gw])
    out += bytes([15, len(DOMAIN), *DOMAIN, 51, 4, *struct.pack(">I", LEASE_S), 255])
    return bytes(out)


class _Dhcp(asyncio.DatagramProtocol):
    def __init__(self, offered: str) -> None:
        self.offered = offered
        self.transport: Any = None

    def connection_made(self, transport: Any) -> None:
        self.transport = transport

    def datagram_received(self, data: bytes, addr: tuple[str, int]) -> None:
        reply = offer_for(data, self.offered)
        if reply is not None:
            self.transport.sendto(reply, ("255.255.255.255", 68))


async def start(offered: str) -> Any:
    """Listens on port 67 for broadcasts (the lab's own namespace)."""
    loop = asyncio.get_running_loop()
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
    sock.bind(("0.0.0.0", 67))
    transport, _ = await loop.create_datagram_endpoint(lambda: _Dhcp(offered), sock=sock)
    log.info("dhcp: offering %s", offered)
    return transport
