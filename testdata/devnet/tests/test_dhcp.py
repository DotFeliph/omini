"""The lab's DHCP server answers a DISCOVER with an offer, and nothing else."""

from __future__ import annotations

import struct

from emulator import dhcp, scan


def discover(msg_type: int = 1) -> bytes:
    b = bytearray(240)
    b[0:3] = bytes([1, 1, 6])
    b[4:8] = struct.pack(">I", 0xCAFE)
    b[10:12] = b"\x80\x00"
    b[28:34] = bytes.fromhex("02de71000001")
    b[236:240] = b"\x63\x82\x53\x63"
    return bytes(b + bytes([53, 1, msg_type, 255]))


def options(p: bytes) -> dict[int, bytes]:
    out, i = {}, 240
    while p[i] != 255:
        out[p[i]] = p[i + 2 : i + 2 + p[i + 1]]
        i += 2 + p[i + 1]
    return out


def test_offer_for_a_discover() -> None:
    o = dhcp.offer_for(discover(), "10.10.10.250")
    assert o is not None and o[0] == 2 and o[4:8] == struct.pack(">I", 0xCAFE)
    assert o[16:20] == bytes([10, 10, 10, 250]) and o[28:34] == bytes.fromhex("02de71000001")
    opts = options(o)
    assert opts[53] == b"\x02" and opts[54] == bytes([10, 10, 10, 1]) and opts[6] == bytes([10, 10, 10, 1])
    assert opts[15] == b"acme.local" and struct.unpack(">I", opts[51])[0] == dhcp.LEASE_S


def test_no_answer_to_anything_else() -> None:
    assert dhcp.offer_for(discover(3), "10.10.10.250") is None  # a REQUEST: never acknowledged
    assert dhcp.offer_for(b"junk", "10.10.10.250") is None


def test_the_gateway_resolves_the_check_name() -> None:
    assert scan.INTERNET["example.com"]
