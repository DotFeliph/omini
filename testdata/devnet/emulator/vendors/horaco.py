# ruff: noqa: E501 (the pages keep the firmware's long HTML lines)
"""Horaco HC-SWTGW218AS web interface (stock firmware V200.x, no SNMP).

The pages follow the firmware's own HTML (captured from a real unit, see
omini-plugin-horaco's tests/fixtures): CGI pages with tables whose `<th>` are
closed by `</td>`, 64-bit counters split as "hi-lo", the front panel drawn as
one `<div class="port" name="p<n>_l<link>_s<speed>_d<duplex>_f<fc>">` per port
and a paged MAC table turned with its form (`cmd=goto`).

Authentication: the login page posts `Response` = MD5(username + password),
which the browser also keeps in the `admin` cookie; every page checks that
cookie against the one session the switch keeps (one client at a time; it
ends after a reboot, a logout or a while without requests). A page asked
without a valid session answers a script sending the browser to /login.cgi
(HTTP 200), like the firmware.

Quirk: twice an hour (minutes 17 and 43) the embedded web server closes every
connection without answering for a few seconds; the plugin's retries and its
6 s pause ride it out.
"""

from __future__ import annotations

import hashlib
from typing import Any

from ..server import DROP, HttpService, Request, Response
from ..world import Snapshot

# Busy windows: minutes of each simulated hour, and their length in real seconds.
BUSY_MINUTES = (17, 43)
BUSY_S = 4.0
# The session ends after this many real seconds without a request.
IDLE_S = 600.0
# "Items per page" of the MAC table: option value → rows. The admin picked 30.
PER_PAGE = {"1": 10, "2": 20, "3": 30}
DEFAULT_PER_PAGE = "3"

# Speed codes of the front panel (index into the page's `spd` array).
SPEED_CODE = {10: 0, 100: 1, 1000: 2, 500: 3, 10000: 4, 2500: 5, 5000: 6}

REDIRECT = '<script type="text/javascript">window.top.location.replace("/login.cgi");</script>'

# Negotiation speeds of the Port Setting forms (option value, text): the RJ45
# ports' form, and the SFP+ port's form.
PORT_SPEEDS = (("10", "Auto"), ("0", "10M"), ("1", "100M"), ("2", "1000M"), ("5", "2500M"))
SFP_SPEEDS = (("10", "Auto"), ("1", "100M"), ("2", "1000M"), ("5", "2500M"), ("4", "10G"))


def port_form(name: str, prefix: str, ports: list[tuple[int, str]], speeds: tuple[tuple[str, str], ...]) -> str:
    """One Port Setting form, as the firmware draws it: a checkbox per port
    (port<index>, labelled with its name; options left unclosed)."""
    boxes = "".join(
        f'<input type="checkbox" id=port{i} name=port{i} class="{prefix}chkp" onclick={prefix}chkClick(this)>'
        f"<label id=lport{i} for=port{i}>{label}</label><br>"
        for i, label in ports
    )
    duplex = '<option value="2">Auto\n <option value="1">Full Duplex\n' + (
        "" if prefix else ' <option value="0">Half Duplex\n'
    )
    speed = "".join(f'<option value="{v}">{t}\n ' for v, t in speeds)
    return (
        f'<form method="post" name="{name}" action="port.cgi"><div class="tblcfgdiv"><table><tr><th style="width:150px;height:30px">Port</th>'
        '<th style="width:180px;">State</th><th style="width:180px;">Duplex Mode</th><th style="width:180px;">Negotiation Speed</th>'
        '<th style="width:180px;">Flow Control</th></tr><tr><td style="width:138px;text-align:left"><div class="port-container">'
        f'<input id="{prefix}ports" readonly value=Select Port><div class="arrow-icon"></div><div class="option-container">'
        f'<input type="checkbox" id="{prefix}all" name="{prefix}all" class="{prefix}allchkp"><label for="{prefix}all">Select All</label><br>'
        f"{boxes}</div></td>"
        '<td><select name="state"><option value="1">Enable\n <option value="0">Disable\n </select></td>'
        f'<td><select name="duplex" id="{prefix}duplex">{duplex} </select></td>'
        f'<td><select name="speed" id="{prefix}speed">{speed}</select></td>'
        '<td><select name="flowctrl"><option value="0">Off\n <option value="1">On\n </select></td></tr></table>'
        '<br style="line-height:50%"><input type="button" value="Apply"><input type="hidden" name="cmd" value="port"></div></form>'
    )


def credential(username: str, password: str) -> str:
    return hashlib.md5((username + password).encode()).hexdigest()


def hilo(value: int) -> str:
    value = max(int(value), 0) & 0xFFFFFFFFFFFFFFFF
    return f"{value >> 32}-{value & 0xFFFFFFFF}"


def uptime_text(seconds: int) -> str:
    s = max(int(seconds), 0)
    return f"{s // 86400}Day{s % 86400 // 3600}Hour{s % 3600 // 60}Minute{s % 60}Second"


def speed_text(mbps: int | None) -> str:
    if not mbps:
        return "Auto"
    return f"{mbps // 1000}G" if mbps >= 10000 else f"{mbps}M"


def login_page(error: bool = False) -> str:
    tip = "Username、Password error" if error else ""
    return f"""<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
<title>Login</title>
<link rel="stylesheet" type="text/css" href="/login.css">
<script type="text/javascript" src="/md5.js"></script>
<script type="text/javascript">
function loginSubmit()
{{
	var user = document.getElementById('inuser').value;
	var pwd = document.getElementById('inpwd').value;
	var response = hex_md5(user + pwd);
	document.getElementById('Response').value = response;
	document.cookie = "admin=" + response;
	document.login.submit();
}}
function keyLogin(e)
{{
	if (e.keyCode == 13) loginSubmit();
}}
</script>
</head>
<body onkeydown="keyLogin(event)">
<center>
<div class="logindiv">
<form method="post" name ="login" action="login.cgi">
<table class="logintbl">
	<tr>
		<td>Username</td>
		<td><input type="text" id="inuser" name="username" maxlength="16" value=""></td>
	</tr>
	<tr>
		<td>Password</td>
		<td><input type="password" id="inpwd" name="password" maxlength="16" value=""></td>
	</tr>
	<tr>
		<td>Language</td>
		<td><select name="language"><option value="EN" selected>English</option><option value="CN">中文</option></select></td>
	</tr>
</table>
<input type="hidden" id="Response" name="Response" value="">
<label id="tip" style="color:red">{tip}</label><br>
<input type="button" value="Login" class="loginbtn" onclick="loginSubmit()">
</form>
</div>
</center>
</body>
</html>
"""


INDEX = """<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
<title>Web Smart Switch</title>
</head>
<frameset rows="155,*" frameborder="0" border="0">
	<frame src="/panel.cgi" name="topFrame" scrolling="no" noresize>
	<frameset cols="210,*" frameborder="0" border="0">
		<frame src="/menu.cgi" name="leftFrame" scrolling="auto" noresize>
		<frame src="/info.cgi" name="mainFrame" scrolling="auto">
	</frameset>
</frameset>
</html>
"""

NOT_FOUND = "<html><head><title>404 Not Found</title></head><body><h1>404 Not Found</h1></body></html>"

MAC_SCRIPT = """<script type="text/javascript">
function btnClick(id)
{
	if (id=='clr') {
		document.getElementById('cmd').value = 'mactblclr';
	}
	else if (id=='next') {
		document.getElementById('cmd').value = 'nextpage';
	}
	else if (id=='prev') {
		document.getElementById('cmd').value = 'prevpage';
	}
	else if (id=='first') {
		document.getElementById('cmd').value = 'firstpage';
	}
	else if (id=='last') {
		document.getElementById('cmd').value = 'lastpage';
	}
	else if (id=='perpage') {
		document.getElementById('cmd').value = 'perpage';
	}
	else if (id=='goto') {
		var pageid=document.getElementById('pageidx').value;
		var totalpage=parseInt(document.getElementById('totalpage').innerHTML);
		var reg=/^[0-9]+/;
		if ((!reg.test(pageid)) || (pageid > totalpage) || (pageid < 1)){
			var adom1 = document.getElementById('tipbox');
			var adomtip = document.getElementById('tipinfo');
			adomtip.innerHTML='Invalid Page index';
			adom1.style.display = "block";
			return;
		}
		document.getElementById('cmd').value = 'goto';
	}
	document.fwd_tbl.submit();
}
function btnSearchClick(id)
{
	if (id=='search') {
		var mac=document.getElementById('mac_search').value;
		var reg=/^([a-fA-F0-9]{2}:){0,5}([a-fA-F0-9]{2}|[a-fA-F0-9]{2}:)$/;
		if (!reg.test(mac)){
			var adom1 = document.getElementById('tipbox');
			var adomtip = document.getElementById('tipinfo');
			adomtip.innerHTML='Invalid MAC Address format';
			adom1.style.display = "block";
			return;
		}
		document.getElementById('cmds').value = 'search';
	}
	else if (id=='clear') {
		document.getElementById('cmds').value = 'clear';
	}
	document.search_tbl.submit();
}
function tipboxbtnClick()
{
	var adom1 = document.getElementById('tipbox');
	adom1.style.display = "none";
}
</script>"""


class Horaco(HttpService):
    def __init__(self, *a: Any, **kw: Any) -> None:
        super().__init__(*a, **kw)
        self.user = self.secrets.get("username", "admin")
        self.password = self.secrets.get("password", "")
        # The one session: (client IP, switch boot time, last request in simulated time).
        self.session: tuple[str, float, float] | None = None
        self.per_page = DEFAULT_PER_PAGE
        self.mac_page = 1  # the MAC table's current page (the firmware keeps it between requests)
        self.mac_search: str | None = None
        self.saves = 0  # Save Configuration posts
        self.added_vlans: dict[int, str] = {}  # created or renamed with Add VLAN

    # ------------------------------------------------------------------ helpers

    def busy(self, snap: Snapshot) -> bool:
        within = snap.t % 3600
        span = min(BUSY_S * snap.speed, 600.0)
        return any(m * 60 <= within < m * 60 + span for m in BUSY_MINUTES)

    def signed_in(self, req: Request, snap: Snapshot) -> bool:
        if self.session is None:
            return False
        ip, boot, last = self.session
        if boot != snap.boot(self.node) or snap.t - last > IDLE_S * snap.speed:
            self.session = None
            return False
        if ip != req.client[0] or req.cookies.get("admin") != credential(self.user, self.password):
            return False
        self.session = (ip, boot, snap.t)
        return True

    @staticmethod
    def page(body: str) -> Response:
        return Response.text(body, ctype="text/html")

    # ------------------------------------------------------------------ routing

    def handle(self, req: Request, snap: Snapshot) -> Response:
        if self.busy(snap):
            return DROP
        path = req.path
        if path in ("/", "/index.html", "/index.cgi"):
            if self.signed_in(req, snap):
                return self.page(INDEX)
            return self.page(login_page())
        if path == "/login.cgi":
            if req.method == "POST":
                return self.login(req, snap)
            return self.page(login_page())
        if path == "/logout.cgi":
            self.session = None
            return self.page(REDIRECT)
        if path.endswith(".css"):
            return Response.text("body{font-family:Arial;font-size:12px}", ctype="text/css")
        if path.endswith(".js"):
            return Response.text("", ctype="application/javascript")
        pages = {
            "/info.cgi": self.info,
            "/port.cgi": self.port,
            "/save.cgi": self.save,
            "/vlan.cgi": self.vlan,
            "/panel.cgi": self.panel,
            "/mac.cgi": self.mac,
        }
        fn = pages.get(path)
        if fn is None:
            return Response.text(NOT_FOUND, 404, "text/html")
        if not self.signed_in(req, snap):
            return self.page(REDIRECT)
        return self.page(fn(req, snap))

    def login(self, req: Request, snap: Snapshot) -> Response:
        form = req.form()
        want = credential(self.user, self.password)
        if form.get("username") != self.user or form.get("Response") != want:
            return self.page(login_page(error=True))
        # The switch keeps one session: signing in takes it over.
        self.session = (req.client[0], snap.boot(self.node), snap.t)
        self.mac_page = 1
        return self.page('<script type="text/javascript">window.top.location.replace("/index.cgi");</script>')

    # ------------------------------------------------------------------ data

    def ports(self) -> list[str]:
        n = self.ctx.world.d.node(self.node)
        return sorted(n.ports, key=lambda p: n.ports[p].index or 0)

    # ------------------------------------------------------------------ pages

    def info(self, req: Request, snap: Snapshot) -> str:
        n = snap.node(self.node)
        fw = snap.firmware(self.node)["current"] or n.version
        rows = []
        for name in self.ports():
            ps = snap.port(self.node, name)
            if ps.up:
                rows.append(
                    f"\t<tr>\n\t\t<td>{name}</td>\n\t\t<td>Link Up</td>\n\t\t<td>Full Duplex</td>\n"
                    f"\t\t<td>{speed_text(ps.speed)}</td>\n\t\t<td>Off</td>\n\t</tr>"
                )
            else:
                rows.append(
                    f"\t<tr>\n\t\t<td>{name}</td>\n\t\t<td>Link Down</td>\n\t\t<td>Auto</td>\n"
                    "\t\t<td>Auto</td>\n\t\t<td>Off</td>\n\t</tr>"
                )
        dev_name = n.hostname or n.model
        return f"""<html>
<head>
<title>System Information</title>
<link rel="stylesheet" type="text/css" href="/style.css">
<style type="text/css">
#infotbl{{width:830px}}
</style>
</head>

<body>
<center>

<div><p>Device Info</p><form method="post" name="info" id="infofrm" action="/info.cgi">
<table class="infotbl" id="infotbl">
	<tr>
		<th style="text-align:center">Device Name:</td>
		<td><input type="text" size="18" name="devName" value="{dev_name}" maxlength="32">                            <input type="submit" value="Modify" id="infosub"></td>
		<th style="width:138px;text-align:center">Sys Uptime:</td>
		<td>{uptime_text(snap.uptime(self.node))}</td>
	</tr>
	<tr>
		<th id='deviceType' style="text-align:center">Device Model:</td>
		<td>{n.model}</td>
		<th id='firewareVer' style="text-align:center">Firmware Version:</td>
		<td>{fw}</td>
	</tr>
	<tr>
		<th style="text-align:center">IP Address:</td>
		<td>{n.ip}</td>
		<th style="text-align:center">Netmask:</td>
		<td>255.255.255.0</td>
	</tr>
	<tr>
		<th style="text-align:center">MAC Address:</td>
		<td>{n.mac.upper()}</td>
	</tr>
</table>
</form>
<p>Port Status</p><table>
	<tr>
		<th width="108">Port</th>
		<th width="128">Link Status</th>
		<th width="182">Duplex Status</th>
		<th width="188">Nego Speed Status</th>
		<th width="162">Flow Control</th>
	</tr>
{chr(10).join(rows)}
</table>
</div></center>
</body>
</html>
"""

    def port(self, req: Request, snap: Snapshot) -> str:
        if req.arg("page") == "stats":
            return self.stats(snap)
        n = self.ctx.world.d.node(self.node)
        if req.method == "POST" and req.form().get("cmd") == "port":
            # Port Setting: the checked ports take the posted state (duplex,
            # speed and flow control stay Auto/Auto/Off here, as on every port).
            form = req.form()
            for i, name in enumerate(self.ports()):
                if form.get(f"port{i}") == "on" and form.get("state") in ("0", "1"):
                    n.ports[name].admin_up = form["state"] == "1"
        rj45 = [(i, name) for i, name in enumerate(self.ports()) if n.ports[name].connector != "sfp"]
        sfp = [(i, name) for i, name in enumerate(self.ports()) if n.ports[name].connector == "sfp"]
        rows = []
        for name in self.ports():
            ps = snap.port(self.node, name)
            up = ps.up and n.ports[name].admin_up
            rows.append(
                f"<tr><td>{name}</td><td>{'Enable' if n.ports[name].admin_up else 'Disable'}</td>"
                f"<td>Auto</td><td>{'Full Duplex' if up else 'Half Duplex'}</td>"
                f"<td>Auto</td><td>{speed_text(ps.speed) if up else '10M'}</td><td>Off</td><td>Off</td></tr>"
            )
        return f"""<html><head><title>Port Setting</title><link rel="stylesheet" type="text/css" href="/style.css"></head><body><h3>Port Setting</h3>{port_form("portcfg", "", rj45, PORT_SPEEDS)}{port_form("extportcfg", "ext", sfp, SFP_SPEEDS) if sfp else ""}<hr><h3>Port List</h3><br><div class="tblcfgdiv"><table><tr><th rowspan="2" style="width:90px;height:80px">Port</th><th rowspan="2" width="90">State</th><th colspan="2" style="width:280px;height:40px">Duplex Mode</th><th colspan="2" width="280">Negotiation Speed</th><th colspan="2" width="130">Flow Control</th></tr><tr><th width="140">Config Attribute</th><th width="140">Actual Status</th><th width="140">Config Attribute</th><th width="140">Actual Status</th><th width="140">Config Attribute</th><th width="140">Actual Status</th></tr>{"".join(rows)}</table></div></body></html>
"""

    # ------------------------------------------------------------------ 802.1Q

    def switch_vlans(self) -> dict[int, str]:
        """The switch's VLANs: VLAN 1, its ports' and those added (14-character names)."""
        n = self.ctx.world.d.node(self.node)
        ids = {1}
        for p in n.ports.values():
            ids.update(v for v in (p.untagged, *p.tagged) if v)
        out = {}
        for vid in sorted(ids):
            v = self.ctx.world.d.vlans.get((n.site, vid))
            out[vid] = (v.name if v and vid != 1 else "")[:14]
        return {**out, **self.added_vlans}

    def vlan(self, req: Request, snap: Snapshot) -> str:
        n = self.ctx.world.d.node(self.node)
        if req.arg("page") == "static":
            if req.method == "POST" and req.form().get("vid", "").isdigit():
                vid = int(req.form()["vid"])
                if 1 <= vid <= 4094:
                    self.added_vlans[vid] = req.form().get("name", "")[:14]
            return vlan_static_page(sorted(self.switch_vlans().items()))
        if req.method == "POST" and "vlan_type" in req.form():
            form = req.form()
            for i, name in enumerate(self.ports()):
                if form.get(f"port{i}") != "on":
                    continue
                p = n.ports[name]
                if form["vlan_type"] == "0":
                    p.untagged, p.tagged = int(form["vlan_access"]), ()
                elif form["vlan_type"] == "1":
                    native = int(form["vlan_native"])
                    allowed = sorted(int(k[4:]) for k in form if k[4:].isdigit() and k.startswith("VLAN"))
                    p.untagged, p.tagged = native, tuple(v for v in allowed if v != native)
        rows = []
        for name in self.ports():
            p = n.ports[name]
            if p.tagged:
                native = p.untagged or 1
                trunk = ",".join(str(v) for v in sorted({native, *p.tagged}))
                rows.append((name, "Trunk", native, native, trunk))
            else:
                rows.append((name, "Access", p.untagged or 1, "-", "-"))
        return vlan_port_page(self.ports(), sorted(self.switch_vlans()), rows)

    def save(self, req: Request, snap: Snapshot) -> str:
        if req.method == "POST" and req.form().get("cmd") == "save":
            self.saves += 1
        return '<html><head><title>Save Configuration</title></head><body><form method="post" name="save" action="/save.cgi"><input type="submit" value="Save"><input type="hidden" name="cmd" value="save"></form></body></html>'

    def stats(self, snap: Snapshot) -> str:
        n = snap.node(self.node)
        names = self.ports()
        script = []
        rows = []
        for i, name in enumerate(names):
            for col in ("txgood", "rxgood", "txgoodByte", "rxgoodByte"):
                script.append(
                    f"\tstatArr = document.getElementById('port{i}-{col}').innerHTML.split('-');\n"
                    f"\tdocument.getElementById('port{i}-{col}').innerHTML="
                    "parseInt(statArr[0]*4294967296)+parseInt(statArr[1]);"
                )
            ps = snap.port(self.node, name)
            rows.append(
                f"\t<tr>\n\t\t<td>{name}</td>\n\t\t<td>{'Enable' if n.ports[name].admin_up else 'Disable'}</td>\n"
                f"\t\t<td>{'Link Up' if ps.up else 'Link Down'}</td>\n"
                f"\t\t<td id=port{i}-txgood>{hilo(ps.tx_packets)}</td>\n"
                f"\t\t<td id=port{i}-rxgood>{hilo(ps.rx_packets)}</td>\n"
                f"\t\t<td id=port{i}-txgoodByte>{hilo(ps.tx_bytes)}</td>\n"
                f"\t\t<td id=port{i}-rxgoodByte>{hilo(ps.rx_bytes)}</td>\n\t</tr>"
            )
        return f"""<html>

<head>
<title>Port Stistics</title>
<link rel="stylesheet" type="text/css" href="/style.css">
<script type="text/javascript">
function loadstats()
{{
	var statArr;
{chr(10).join(script)}
}}
</script>
</head>

<body onload="loadstats()">

<h3>Port Statistics</h3><form method="post" action="/port.cgi?page=stats">
<div class="tblcfgdiv"><table>
	<tr>
		<th style="width:100px;">Port</th>
		<th style="width:118px;">State</th>
		<th style="width:118px;">Link Status</th>
		<th style="width:128px;">TxGoodPkt</th>
		<th style="width:128px;">RxGoodPkt</th>
		<th style="width:128px;">TxGoodBytes</th>
		<th style="width:128px;">RxGoodBytes</th>
	</tr>
{chr(10).join(rows)}
</table>
<br style="line-height:50%">
<input type="submit" name="submit" value="Clear">
<input type="hidden" name="cmd" value="stats">
</div></form>
</body>
</html>"""

    def panel(self, req: Request, snap: Snapshot) -> str:
        n = snap.node(self.node)
        names = self.ports()
        heads, cells = [], []
        for k, name in enumerate(names, start=1):
            p = n.ports[name]
            ps = snap.port(self.node, name)
            fiber = p.connector == "sfp" or p.kind == "sfp"
            link = 2 if not p.admin_up else (1 if ps.up else 0)
            spd = SPEED_CODE.get(ps.speed or 0, 7) if ps.up else 7
            if fiber:
                cls = "portfiberlnkup" if ps.up else "portfiber"
            elif not ps.up:
                cls = "portlnkdown"
            elif (ps.speed or 0) >= 1000:
                cls = "port2p5"
            else:
                cls = "port100m"
            heads.append(f"\t<td>{k}</td>")
            cells.append(
                f'\t<td>\n\t\t<div class="port" name="p{k}_l{link}_s{spd}_d{1 if ps.up else 0}_f0" '
                'onmouseover="showPortStatusTip(this)" onmouseout="hiddenPortStatusTip(this)">\n'
                f'\t\t\t<div class="{cls}"></div>\n\t\t</div>\n\t</td>'
            )
        return PANEL.replace("{heads}", "\n".join(heads)).replace("{cells}", "\n".join(cells))

    def mac(self, req: Request, snap: Snapshot) -> str:
        page = req.arg("page") or "fwd_tbl"
        form = req.form() if req.method == "POST" else {}
        entries = sorted(snap.fdb(self.node), key=lambda e: hashlib.md5(e[0].encode()).digest())
        if page == "search":
            cmds = form.get("cmds")
            if cmds == "search":
                self.mac_search = (form.get("mac_search") or "").lower() or None
            elif cmds == "clear":
                self.mac_search = None
            self.mac_page = 1
        elif req.method == "GET":
            self.mac_page = 1
        if self.mac_search:
            entries = [e for e in entries if e[0].startswith(self.mac_search)]
        cmd = form.get("cmd", "")
        if cmd == "perpage" or (cmd == "goto" and form.get("perpage") in PER_PAGE):
            self.per_page = form.get("perpage") if form.get("perpage") in PER_PAGE else self.per_page
        size = PER_PAGE[self.per_page]
        total_pages = max(1, -(-len(entries) // size))
        if cmd == "goto":
            try:
                self.mac_page = int(form.get("pageidx", "1"))
            except ValueError:
                self.mac_page = 1
        elif cmd == "nextpage":
            self.mac_page += 1
        elif cmd == "prevpage":
            self.mac_page -= 1
        elif cmd in ("firstpage", "perpage", "mactblclr"):
            # Clearing the table changes nothing visible here: the entries are learned again at once.
            self.mac_page = 1
        elif cmd == "lastpage":
            self.mac_page = total_pages
        self.mac_page = min(max(self.mac_page, 1), total_pages)
        cur = self.mac_page
        shown = entries[(cur - 1) * size : cur * size]
        rows = "".join(
            f"<tr>\n\t<td >{m.upper()}</td>\n\t<td >dynamic</td>\n\t<td >{port.split()[-1]}</td>\n\t<td >{vlan}</td>\n</tr>\n"
            for m, port, vlan in shown
        )
        first = (cur - 1) * size + 1 if shown else 0
        last = (cur - 1) * size + len(shown)
        search = (self.mac_search or "").upper()

        def nav(label_id: str, text: str, active: bool) -> str:
            color = "#000" if active else "#c3c3c3"
            style = f"width:32px;border:0px;font-size:16px;color:{color};transform:scaleY(1.5)"
            click = ' onclick="btnClick(this.id)" style="cursor:pointer;"' if active else ""
            return f'\t<td style="{style}"><label id={label_id}{click}>{text}</label></td>'

        options = "\n".join(
            f'\t\t\t<option value="{v}" {"selected" if v == self.per_page else ""}>{rows_}</option>'
            for v, rows_ in PER_PAGE.items()
        )
        return f"""<html>

<head>
<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
<title>MAC Address Information</title>
<link rel="stylesheet" type="text/css" href="/style.css">
{MAC_SCRIPT}
</head>

<body>
<h3>MAC Address Table</h3>
<div class="tipdiv" id="tipbox" style="margin-left:500px;display:none"><p style="margin-left:20px;color:red">Error</p><br>
<p id='tipinfo' style="width:238;margin-left:60px"></p><br>
<input type="button" value="Confirm" onclick="tipboxbtnClick()" style="margin-left:120px">
</div>
<form method="post" name = "search_tbl" action="/mac.cgi?page=search">
<div class="tblcfgdiv"><table class="infotbl" style="width:820px">
	<tr>
		<td style="padding-left:0px;width:252px;"><input type="text" name="mac_search" value="{search}" id='mac_search' maxlength="18"></td>
		<td><input type="button" value="Search" id=search onclick="btnSearchClick(this.id)"></td>
		<td><input type="button" value="Clear" id=clear onclick="btnSearchClick(this.id)"></td>
		<td style="width:230px;"></td>
		<td style="width:230px;"></td>
	</tr>
</table>
<input type="hidden" id=cmds name="cmds" value="">
</div>
</form>
<br>
<form method="post" name = "fwd_tbl" action="/mac.cgi?page=fwd_tbl">
<div class="tblcfgdiv"><table>
	<tr>
		<th style="width:230px;">MAC Address</th>
		<th style="width:120px;">Type</th>
		<th style="width:250px;">Port</th>
		<th style="width:210px;">VLAN ID</th>
	</tr>
{rows}</table>
<br style="line-height:50%">
<table style="border-width:0px">
	<td style="width:100px;border:0px"><label>Total {len(entries)} Items,</label></td>
	<td style="width:145px;border:0px"><label>Current {first}-{last} Items,</label></td>
	<td style="width:150px;border:0px">
		<label>PerPage</label>
		<select name='perpage' id='perpage' style="width:50px" onchange="btnClick(this.id)">
{options}
		</select>
		<label>Items</label>
	</td>
{nav("first", "<<", cur > 1)}
{nav("prev", "<", cur > 1)}
	<td style="width:18px;border:0px;">
		<div style="display:flex;justify-content:center;align-items:center;width:20px;height:20px;background-color:rgb(62,140,213);color:#fff">{cur}</div>
	</td>
{nav("next", ">", cur < total_pages)}
{nav("last", ">>", cur < total_pages)}
	<td style="width:95px;border:0px"><input type='text' name='pageidx' id='pageidx' maxlength='3' style="width:30px" value="{cur}"> /<label id='totalpage'>{total_pages} </label>Pages</td>
	<td style="width:55px;border:0px"><input type='button' value="Goto" id=goto onclick="btnClick(this.id)" style="width:50px;height:25px"></td>
	<td style="width:115px;border:0px"></td>
</table>
<input type="hidden" id=cmd name="cmd" value="">
</div>
</form>
</body>
</html>
"""


PANEL = """<html>
<head>
<title>Top Frame</title>
<link rel="stylesheet" type="text/css" href="/panel.css">
<script type="text/javascript">
function changeLanguage()
{
	var adom1 = document.getElementById('langsel');
	adom1.submit();
}
function showPortStatusTip(obj)
{
	var rect = obj.getBoundingClientRect();
	var str = obj.getAttribute('name');
	var spd = ["10Mbps","100Mbps","1000Mbps","500Mbps","10Gbps","2.5Gbps","5Gbps","-"];
	var dup = ["Half", "Full"];
	var fc  = ["Off", "On"];
	var port = "Port "+str[1];
	var link1 = (str[4] == '2')?"Disable":((str[4] == '0') ? "Down":"Link Up");
	var link = "Link： "+link1;
	var speed = "Speed： "+spd[parseInt(str[7])];
	var duplex = "Duplex Mode： "+dup[parseInt(str[10])];
	var flowctrl = "Flow Control： "+fc[parseInt(str[13])];
	document.getElementById('tipspanPort').innerHTML = port;
	document.getElementById('tipspanLink').innerHTML = link;
	document.getElementById('tipspanSpeed').innerHTML = speed;
	document.getElementById('tipspanDuplex').innerHTML = duplex;
	document.getElementById('tipspanFc').innerHTML = flowctrl;
	document.getElementById('tipbox').style.left = rect.left-37;
	document.getElementById('tipbox').style.visibility = "visible";
}
function hiddenPortStatusTip(obj)
{
document.getElementById('tipbox').style.visibility = "hidden";
}
</script>
</head>
<body>
<script>
function doreload() {
  timer++;
  if (timer>=10)window.location.replace("panel.cgi");
}
function reloadAll() {
  setInterval("doreload();", 1000);
}
  var timer = 0;
  reloadAll();
</script>
<center>
<div class="topdiv">
<div class="paneldiv">
<table class="paneltable">
	<tr>
{heads}
	</tr>
	<tr>
{cells}
	</tr>
</table>
<div class="tipbox" id="tipbox">
	<span id="tipspanPort"></span>
	<span id="tipspanLink"></span>
	<span id="tipspanSpeed"></span>
	<span id="tipspanDuplex"></span>
	<span id="tipspanFc"></span>
</div>
</div>
<div class="sysdiv">
<form method="post" action="/savecfg.cgi" style="display:inline-block">
	<input type="submit" name="savecfg" value="Save configuration" class="sysbutton">
</form>
<input type="button" name="logout" value="Logout" onclick='location.href=("logout.cgi")'>
</div>
</div>
<br>
<div style="width:708px;margin-left:-300px"><table>
	<tr>
		<td>
			<div class="indicon" style="background-color:rgb(72,125,233);"></div>
		</td>
		<td style="width:60px;text-align:left">
			<label>10G</label>
		</td>
		<td>
			<div class="indicon" style="background-color:rgb(82,151,199);"></div>
		</td>
		<td style="width:60px;text-align:left">
			<label>2.5G/1G</label>
		</td>
		<td>
			<div class="indicon" style="background-color:rgb(106,185,196);"></div>
		</td>
		<td style="width:280px;text-align:left">
			<label>100M/10M</label>
		</td>
		<td>
			<div class="arrow" style="background-color:rgb(105,105,105);"></div>
		</td>
		<td>
			<div class="port" style="width:15px;transform:scale(0.4,0.4)">
				<div class="portlnkdown"></div>
			</div>
		</td>
		<td style="width:35px;text-align:left;padding-left:5px">
			<label>Coper</label>
		</td>
		<td>
			<div class="port" style="width:15px;transform:scale(0.4,0.4)">
				<div class="portfiber"></div>
			</div>
		</td>
		<td>
			<label>Fiber</label>
		</td>
  </tr>
	<tr>
		<td>
			<div class="indicon" style="background-color:rgb(126,135,146);"></div>
		</td>
		<td style="width:50px;text-align:left">
			<label>Down</label>
		</td>
		<td>
			<div class="indicon" style="background-color:rgb(190,190,190);"></div>
		</td>
		<td style="width:60px;text-align:left">
			<label>Disable</label>
		</td>
	</tr>
</table>
</div>
<br>
<hr style="height:5px;border:0;background-color:#dfdfdf">
</body>
</html>
"""


def vlan_static_page(vlans: list[tuple[int, str]]) -> str:
    """Add VLAN: the VLAN Manage switch, the form, and the list (VLAN 1 cannot be deleted)."""
    box = '<input type="checkbox" class="vlanchk">'
    rows = "".join(
        f"<tr><td>{'' if vid == 1 else box}</td><td>{i}</td><td>{vid}</td><td>{name}</td></tr>"
        for i, (vid, name) in enumerate(vlans, start=1)
    )
    return (
        '<html><head><title>VLAN Static</title></head><body><form method="post" name="vlaneblfrm" '
        'action="/vlan.cgi?page=state"><table><tr><td>VLAN Manage:</td><td><input type="checkbox" '
        'name="enable_vlan" checked></td></tr></table></form><form method="post" name="formVlanStatic" '
        'id="formVlanStatic" action="/vlan.cgi?page=static"><table><tr><td>VLAN ID:</td><td><input type="text" '
        'name="vid" id="vlanID"></td></tr><tr><td>VLAN Name:</td><td><input type="text" name="name"></td></tr>'
        '</table><input type="button" value="Apply" onclick="addVlanClick()"></form><form method="post" '
        'name="formVlanStatic" id="formVlanDelStatic" action="/vlan.cgi?page=getRmvVlanEntry"><table><tr>'
        f"<th>Select</th><th>No.</th><th>VLAN ID</th><th>VLAN Name</th></tr>{rows}"
        '</table><input type="button" value="Select All"></form></body></html>'
    )


def vlan_port_page(ports: list[str], vlans: list[int], rows: list[tuple[str, str, int, int | str, str]]) -> str:
    """802.1Q VLAN Setting: the port form (checkboxes port<index>, unclosed options) and the list."""
    boxes = "".join(
        f'<input type="checkbox" id=port{i} name=port{i} class="chkp"><label for=port{i}>{name}</label><br>'
        for i, name in enumerate(ports)
    )
    opts = "".join(f'<option value="{v}">VLAN {v}\n ' for v in vlans)
    trunk = "".join(
        f'<input type="checkbox" id=VLAN{v} name=VLAN{v} class="chkvlan"><label for=VLAN{v}>VLAN {v}</label><br>'
        for v in vlans
    )
    listed = "".join(
        f"<tr><td>{p}</td><td>{mode}</td><td>{access}</td><td>{native}</td><td>{allowed}</td></tr>"
        for p, mode, access, native, allowed in rows
    )
    return (
        "<html><head><title>VLAN Port Information</title></head><body><h3>802.1Q VLAN Setting</h3>"
        '<form method="post" id="portVlan" action="/vlan.cgi?page=port_based"><table><tr><th>Port</th>'
        "<th>Port vlan type</th><th>Access VLAN</th><th>Native VLAN</th><th>Trunk vlan</th></tr><tr><td>"
        f'<input type="checkbox" id="all" name="all"><label for="all">Select All</label><br>{boxes}</td>'
        '<td><select name="vlan_type" id="vlan_type"><option value="0">Access\n <option value="1">Trunk\n '
        '<option value="2">Hybrid\n </select><select name="accept_type" id="accept_type"><option value="0">All\n '
        '<option value="1">Tag Only\n <option value="2">Untag Only\n </select></td>'
        f'<td><select name="vlan_access" id="vlanAccess">{opts}</select></td>'
        f'<td><select name="vlan_native" id="vlanTrunk" disabled>{opts}</select></td><td>{trunk}</td></tr>'
        '</table><input type="button" value="Apply" onclick="chkVlanEnable()"></form><hr><table><tr><th>Port</th>'
        f"<th>Port vlan type</th><th>Access VLAN</th><th>Native VLAN</th><th>Trunk vlan</th></tr>{listed}"
        "</table></body></html>"
    )
