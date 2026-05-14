from reportlab.lib.pagesizes import letter
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.lib.units import inch
from reportlab.lib import colors
from reportlab.platypus import (
    SimpleDocTemplate, Paragraph, Spacer, Table, TableStyle,
    HRFlowable, KeepTogether
)
from reportlab.lib.enums import TA_LEFT, TA_CENTER

OUTPUT = "wyndctl-docs.pdf"

# ── Colors ────────────────────────────────────────────────────────────────────
WYND_ORANGE  = colors.HexColor("#FF6B00")
WYND_DARK    = colors.HexColor("#1A1A1A")
STEP_BG      = colors.HexColor("#FFF4E8")
CODE_BG      = colors.HexColor("#F5F5F5")
DIVIDER      = colors.HexColor("#E0E0E0")
FLAG_HEADER  = colors.HexColor("#FF8C00")
STEP_BORDER  = colors.HexColor("#FF6B00")
WHITE        = colors.white

# ── Styles ────────────────────────────────────────────────────────────────────
base = getSampleStyleSheet()

def make_style(name, parent="Normal", **kwargs):
    return ParagraphStyle(name, parent=base[parent], **kwargs)

title_style = make_style("DocTitle", "Title",
    fontSize=28, textColor=WYND_DARK, spaceAfter=4, alignment=TA_CENTER)

subtitle_style = make_style("DocSubtitle", "Normal",
    fontSize=13, textColor=WYND_ORANGE, alignment=TA_CENTER, spaceAfter=24)

section_style = make_style("Section",
    fontSize=18, textColor=WYND_ORANGE, spaceBefore=20, spaceAfter=8,
    fontName="Helvetica-Bold")

sub_section_style = make_style("SubSection",
    fontSize=13, textColor=WYND_DARK, spaceBefore=14, spaceAfter=6,
    fontName="Helvetica-Bold")

body_style = make_style("Body",
    fontSize=10, textColor=WYND_DARK, spaceAfter=6, leading=16)

code_style = make_style("Code",
    fontSize=9, fontName="Courier", backColor=CODE_BG,
    textColor=colors.HexColor("#333333"), spaceAfter=8,
    leftIndent=10, rightIndent=10, leading=14,
    borderPad=6)

note_style = make_style("Note",
    fontSize=9, textColor=colors.HexColor("#555555"),
    spaceAfter=6, leading=14, leftIndent=12)

flag_name_style = make_style("FlagName",
    fontSize=9, fontName="Courier-Bold",
    textColor=colors.HexColor("#CC4400"))

flag_desc_style = make_style("FlagDesc",
    fontSize=9, textColor=WYND_DARK, leading=13)

step_title_style = make_style("StepTitle",
    fontSize=10, fontName="Helvetica-Bold",
    textColor=WYND_ORANGE, spaceAfter=2)

step_body_style = make_style("StepBody",
    fontSize=9, textColor=WYND_DARK, leading=13)

# ── Helpers ───────────────────────────────────────────────────────────────────
def hr():
    return HRFlowable(width="100%", thickness=1, color=DIVIDER, spaceAfter=10, spaceBefore=4)

def section(title):
    return [Paragraph(title, section_style), hr()]

def subsection(title):
    return Paragraph(title, sub_section_style)

def body(text):
    return Paragraph(text, body_style)

def code(text):
    return Paragraph(text.replace("\n", "<br/>").replace(" ", "&nbsp;"), code_style)

def note(text):
    return Paragraph(f"<i>{text}</i>", note_style)

def flags_table(rows):
    data = [[
        Paragraph("Flag", ParagraphStyle("FH", fontSize=9, fontName="Helvetica-Bold",
                                          textColor=WHITE)),
        Paragraph("Default", ParagraphStyle("FH2", fontSize=9, fontName="Helvetica-Bold",
                                             textColor=WHITE)),
        Paragraph("Description", ParagraphStyle("FH3", fontSize=9, fontName="Helvetica-Bold",
                                                  textColor=WHITE)),
    ]]
    for flag, default, desc in rows:
        data.append([
            Paragraph(flag, flag_name_style),
            Paragraph(default, flag_desc_style),
            Paragraph(desc, flag_desc_style),
        ])

    col_widths = [1.6*inch, 1.0*inch, 4.0*inch]
    t = Table(data, colWidths=col_widths, repeatRows=1)
    t.setStyle(TableStyle([
        ("BACKGROUND",  (0, 0), (-1, 0),  FLAG_HEADER),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [WHITE, CODE_BG]),
        ("GRID",        (0, 0), (-1, -1),  0.4, DIVIDER),
        ("TOPPADDING",  (0, 0), (-1, -1),  5),
        ("BOTTOMPADDING",(0,0), (-1, -1),  5),
        ("LEFTPADDING", (0, 0), (-1, -1),  6),
        ("RIGHTPADDING",(0, 0), (-1, -1),  6),
        ("VALIGN",      (0, 0), (-1, -1),  "TOP"),
    ]))
    return t

def step_box(number, title, detail):
    inner = [
        Paragraph(f"Step {number} — {title}", step_title_style),
        Paragraph(detail, step_body_style),
    ]
    data = [[inner]]
    t = Table(data, colWidths=[6.6*inch])
    t.setStyle(TableStyle([
        ("BACKGROUND",   (0, 0), (-1, -1), STEP_BG),
        ("LINEAFTER",    (0, 0), (0, -1),  2, STEP_BORDER),
        ("LINEBEFORE",   (0, 0), (0, -1),  3, STEP_BORDER),
        ("TOPPADDING",   (0, 0), (-1, -1), 8),
        ("BOTTOMPADDING",(0, 0), (-1, -1), 8),
        ("LEFTPADDING",  (0, 0), (-1, -1), 10),
        ("RIGHTPADDING", (0, 0), (-1, -1), 10),
    ]))
    return t

# ── Document ──────────────────────────────────────────────────────────────────
doc = SimpleDocTemplate(
    OUTPUT,
    pagesize=letter,
    leftMargin=0.85*inch, rightMargin=0.85*inch,
    topMargin=0.9*inch, bottomMargin=0.9*inch,
)

story = []

# Cover
story.append(Spacer(1, 0.5*inch))
story.append(Paragraph("wyndctl", title_style))
story.append(Paragraph("Command Reference &amp; Technical Guide", subtitle_style))
story.append(hr())
story.append(body(
    "wyndctl is a Go CLI for scanning and deploying Wynd Sentry IoT air-quality devices over USB serial. "
    "It exposes three commands — <b>scan</b>, <b>deploy</b>, and <b>delete-device</b> — and connects to a "
    "Postgres database to register and manage device assignments in the Wynd platform."
))
story.append(Spacer(1, 0.15*inch))

# ── Global Flags ──────────────────────────────────────────────────────────────
story += section("Global Flags")
story.append(body(
    "These flags are available on every command and are set once for the entire session."
))
story.append(Spacer(1, 0.08*inch))
story.append(flags_table([
    ("--config", "./wyndctl.yaml", "Path to the config file. Searched in current directory, home directory, and /etc/wynd."),
    ("--env",    "dev",            "Deployment environment: dev, staging, or prod. Controls which DB secret and IoT resources are used."),
    ("--log-level", "info",        "Log verbosity: debug, info, warn, error."),
    ("--log-format", "text",       "Log output format: text (human-readable) or json (structured, for log aggregators)."),
]))
story.append(Spacer(1, 0.1*inch))
story.append(note(
    "Tip: set env permanently by creating ~/wyndctl.yaml with `env: prod`, "
    "or export WYND_ENV=prod in your shell profile."
))

story.append(Spacer(1, 0.2*inch))

# ══════════════════════════════════════════════════════════════════════════════
# SCAN COMMAND
# ══════════════════════════════════════════════════════════════════════════════
story += section("scan — Discover Devices")

story.append(body(
    "Finds all Sentry devices connected via USB, prints their firmware versions, "
    "and lights their LED indicators. With <b>--label</b>, interactively maps each device "
    "to a bay number and writes <code>location-map.csv</code>."
))
story.append(Spacer(1, 0.08*inch))
story.append(code("wyndctl scan [--label] [--color R,G,B]"))
story.append(Spacer(1, 0.1*inch))

story.append(subsection("Flags"))
story.append(flags_table([
    ("--label",           "false",  "Interactive labeling mode. Lights each device's LED and prompts the operator to type a bay number. Writes bay, USB location, and device ID to location-map.csv."),
    ("--color R,G,B",     "255,165,0", "RGB color for the device LED indicator. Orange by default. Example: 255,0,0 for red."),
    ("-o / --output FMT", "text",   "Output format: text (human-readable) or json (structured). Defaults to json automatically when --log-format json is set."),
]))
story.append(Spacer(1, 0.15*inch))

story.append(subsection("What Happens Behind the Scenes"))
story.append(Spacer(1, 0.08*inch))

scan_steps = [
    ("USB Discovery",
     "The CLI calls transport.FindSerialPorts() which scans all system serial ports and filters by "
     "USB Vendor ID 0x2FE3 and Product ID 0x0100 — the hardware IDs burned into every Sentry device. "
     "Only matching ports are returned."),
    ("Channel Activation",
     "Each matched port is opened as a SerialChannel at 9600 baud. The COBS framing layer is "
     "initialised on top: all data is encoded so the 0x00 byte acts as a clean packet delimiter."),
    ("LED Reset",
     "Before printing any info, the CLI sends a CancelIndication RPC to every device, turning off "
     "any previously set LED state. A 1.5-second pause follows each reset to let the hardware settle."),
    ("GetDeviceInfo RPC",
     "For each channel, the CLI encodes a request using the hand-rolled protobuf-compatible encoder "
     "and calls RPC.UnaryCall(packageID=1, serviceID=1, methodID=1). The device responds with its "
     "AWS Thing name, firmware version, WiFi firmware version, and PM firmware version."),
    ("LED Indication",
     "After printing device info, the CLI calls SetIndicate with the --color value. In --label mode "
     "only one device LED is lit at a time so the operator can visually identify which physical "
     "device to assign a bay number to."),
    ("location-map.csv (--label only)",
     "Each operator-entered bay number is written to location-map.csv as a row of "
     "bay, USB port path, device ID. This file is later consumed by the deploy command "
     "to match physical USB positions to deployment settings."),
]

for i, (title, detail) in enumerate(scan_steps, 1):
    story.append(KeepTogether([step_box(i, title, detail), Spacer(1, 0.08*inch)]))

story.append(Spacer(1, 0.1*inch))
story.append(subsection("Output Files"))
story.append(flags_table([
    ("location-map.csv", "--label only", "Columns: bay, location (USB port path), device_id. Used by deploy to match ports to settings."),
]))

story.append(Spacer(1, 0.25*inch))

# ══════════════════════════════════════════════════════════════════════════════
# DEPLOY COMMAND
# ══════════════════════════════════════════════════════════════════════════════
story += section("deploy — Mass Provision Devices")

story.append(body(
    "Provisions multiple Sentry devices in parallel: configures WiFi credentials over USB, "
    "registers each device in AWS IoT, assigns it to a property and room in the Postgres database, "
    "and logs every result to <code>deployment-result.csv</code>."
))
story.append(Spacer(1, 0.08*inch))
story.append(code("wyndctl deploy [flags]"))
story.append(Spacer(1, 0.1*inch))

story.append(subsection("Flags"))
story.append(flags_table([
    ("--all",            "false",            "Deploy all found devices. Without this flag, devices already marked succeeded in deployment-result.csv are skipped — allowing clean retries of failed devices only."),
    ("--color R,G,B",    "255,0,255",         "RGB color for the LED while a device is actively being deployed. Defaults to magenta."),
    ("--iterative",      "false",            "One-by-one mode. Lights a device LED and waits for the operator to press Enter before deploying it. Useful for manual verification."),
    ("--workers N",      "runtime.NumCPU()", "Number of parallel goroutines. Defaults to the number of CPU cores on the machine running the CLI."),
    ("--timeout N",      "75",               "Per-device provisioning timeout in seconds. If the device does not reach ProvisionMQTTPublish status within this window, the attempt is counted as failed."),
]))
story.append(Spacer(1, 0.15*inch))

story.append(subsection("Required Input Files"))
story.append(flags_table([
    ("deployment-data.csv", "required", "Columns: bay, wifi_ssid, wifi_psk, account, lodging_id, room, room_type. wifi_ssid, account, lodging_id, and room_type inherit from the previous row if left blank."),
    ("location-map.csv",    "optional", "Created by scan --label. Maps USB port paths to bay numbers. If absent, devices are assigned bay numbers by discovery order."),
]))
story.append(Spacer(1, 0.15*inch))

story.append(subsection("What Happens Behind the Scenes"))
story.append(Spacer(1, 0.08*inch))

deploy_steps = [
    ("Load CSV Settings",
     "deployment-data.csv is parsed row by row. Blank cells for wifi_ssid, account, lodging_id, "
     "and room_type inherit the value from the previous row, so a block of rooms in the same property "
     "only needs those fields set once. wifi_psk never inherits — it must be explicit."),
    ("Load Location Map",
     "If location-map.csv exists, it is loaded into a map of USB port path to bay number. "
     "If the file is absent, devices are numbered in USB discovery order with a warning logged."),
    ("USB Scan",
     "Identical to the scan command: FindSerialPorts filters by VID/PID, each port is opened "
     "as a SerialChannel at 9600 baud, and COBS framing is initialised on every channel."),
    ("Operator Confirmation",
     "The CLI prompts Deploy N device(s) on <env>? [y/N] to stderr. The operator must type y or yes. "
     "Any other input cancels the run. This prompt cannot be bypassed."),
    ("Database Connection",
     "The CLI connects to Postgres (credentials resolved from AWS Secrets Manager for dev/prod). "
     "This is optional — if the database is unavailable, room assignment is skipped with a warning "
     "and the rest of the deployment continues."),
    ("Parallel Deployment (default)",
     "One goroutine is spawned per device, bounded by a semaphore of size --workers. Each goroutine "
     "runs the full provisioning sequence independently. With --iterative, a single goroutine runs "
     "each device in sequence after the operator presses Enter."),
    ("Unprovision",
     "The CLI sends an Unprovision RPC to clear any existing WiFi and MQTT credentials from "
     "the device's flash storage. This ensures a clean state before writing new credentials. "
     "A 1-second sleep follows to let the device settle."),
    ("Disable BLE Advertising",
     "SetAdvertising(false) is called to turn off Bluetooth advertising. This reduces RF interference "
     "during the WiFi provisioning handshake and is standard procedure before every deployment."),
    ("SetProvision — WiFi + MQTT",
     "The CLI sends the WiFi SSID, PSK, and MQTT broker URL to the device via the SetProvision RPC. "
     "It then polls GetStatus every few seconds until the device reaches ProvisionMQTTPublish state "
     "(state 7), confirming the device connected to WiFi, resolved DNS, connected to the MQTT broker, "
     "and published its first message. If the --timeout window expires without reaching this state, "
     "the attempt is marked failed. Up to 5 retries are attempted with a device reboot between each."),
    ("Database — Room Assignment",
     "The CLI calls GetUserByEmail to verify the account exists, GetLodging to confirm the lodging "
     "belongs to that user, then FindOrCreateZone to get or create the room record. Finally "
     "AssignDeviceToZone upserts the device row with the zone ID. If the user or lodging is not "
     "found, the deployment for that device is marked failed."),
    ("LED Feedback + Result Log",
     "On success the LED is set to the --color value. On failure it is set to red (255,0,0). "
     "A row is appended to deployment-result.csv with bay, device_id, mac_addr, succeeded, "
     "room_id, room_name, and a failure reason if applicable."),
]

for i, (title, detail) in enumerate(deploy_steps, 1):
    story.append(KeepTogether([step_box(i, title, detail), Spacer(1, 0.08*inch)]))

story.append(Spacer(1, 0.1*inch))
story.append(subsection("Output Files"))
story.append(flags_table([
    ("deployment-result.csv", "always written", "Columns: bay, device_id, mac_addr, succeeded, room_id, room_name, reason. Re-running without --all uses this file to skip already-succeeded devices."),
]))

story.append(Spacer(1, 0.2*inch))

# ══════════════════════════════════════════════════════════════════════════════
# DELETE-DEVICE COMMAND
# ══════════════════════════════════════════════════════════════════════════════
story += section("delete-device — Remove Device Assignments")

story.append(body(
    "Removes device-to-room assignments from the database. Device rows and AWS IoT "
    "certificates are preserved so devices can be redeployed at any time. "
    "Pass <b>--delete-room</b> to also remove the associated zone records."
))
story.append(Spacer(1, 0.08*inch))
story.append(code("wyndctl delete-device --account EMAIL [--lodging-id N] [--device-id ID] [--delete-room]"))
story.append(Spacer(1, 0.1*inch))

story.append(subsection("Flags"))
story.append(flags_table([
    ("--account EMAIL",  "required", "Email address of the user whose devices will be cleared."),
    ("--lodging-id N",   "0 (all)",  "Scope deletion to a specific lodging ID. If omitted, all lodgings owned by the account are affected."),
    ("--device-id ID",   "(all)",    "Scope deletion to a single device by AWS Thing name. If omitted, all devices under the account (and lodging, if specified) are affected."),
    ("--delete-room",    "false",    "Also delete the associated zone/room records from the database. Without this flag only the device-to-zone link is cleared."),
]))
story.append(Spacer(1, 0.15*inch))

story.append(subsection("What Happens Behind the Scenes"))
story.append(Spacer(1, 0.08*inch))

delete_steps = [
    ("Resolve User",
     "The CLI looks up the user by --account email. If no matching user is found the command "
     "exits with an error immediately."),
    ("Query Devices",
     "Devices are queried by joining device → zone → lodging, filtered by owner_id. "
     "Optionally narrowed by --lodging-id or --device-id. If no devices are found the command "
     "prints a message and exits cleanly."),
    ("Clear Zone Assignment",
     "For each matching device, zone_id is set to NULL in the database. The device row itself "
     "is kept intact — the device can be redeployed to a new room at any time without any "
     "hardware or IoT reconfiguration."),
    ("Delete Room (--delete-room only)",
     "If --delete-room is passed, the zone record associated with each device is also deleted "
     "from the database. Use with care — if multiple devices share a zone, the zone is deleted "
     "when the first device in it is processed."),
    ("Result",
     "The CLI prints the count of cleared assignments and exits. No AWS IoT changes are made — "
     "certificates and policies are never modified."),
]

for i, (title, detail) in enumerate(delete_steps, 1):
    story.append(KeepTogether([step_box(i, title, detail), Spacer(1, 0.08*inch)]))

story.append(Spacer(1, 0.2*inch))

# ── Retry / Resume ────────────────────────────────────────────────────────────
story += section("Retry and Resume")
story.append(body(
    "If some devices fail, re-run <b>wyndctl deploy</b> without <b>--all</b>. "
    "The CLI reads the existing <code>deployment-result.csv</code> and skips any bay already "
    "marked <code>succeeded=true</code>, so only the failed devices are retried. "
    "Use <b>--all</b> to force a full re-deployment of every connected device regardless of "
    "prior results."
))
story.append(Spacer(1, 0.08*inch))
story.append(code(
    "# First run — some devices fail\n"
    "wyndctl deploy\n\n"
    "# Retry only failures\n"
    "wyndctl deploy\n\n"
    "# Force re-deploy everything\n"
    "wyndctl deploy --all"
))

story.append(Spacer(1, 0.2*inch))

# ── Provisioning State Machine ────────────────────────────────────────────────
story += section("Provisioning State Machine")
story.append(body(
    "After SetProvision is called, the device moves through these states. "
    "The CLI polls until state 7 (ProvisionMQTTPublish) or the timeout expires."
))
story.append(Spacer(1, 0.08*inch))

states = [
    ("0", "ProvisionOff",           "Device is off / not started"),
    ("1", "ProvisionUnprovisioned", "Credentials cleared, ready to receive new config"),
    ("2", "ProvisionWiFiWait",      "Credentials received, attempting WiFi association"),
    ("3", "ProvisionWiFiDisconnect","WiFi association failed, retrying"),
    ("4", "ProvisionWiFiConnect",   "WiFi associated successfully"),
    ("5", "ProvisionWiFiPing",      "DNS/connectivity check in progress"),
    ("6", "ProvisionMQTTConnect",   "Connecting to MQTT broker (AWS IoT endpoint)"),
    ("7", "ProvisionMQTTPublish",   "Connected and published — provisioning complete"),
    ("8", "ProvisionMQTTWait",      "Waiting for MQTT acknowledgement"),
]

state_data = [[
    Paragraph("State", ParagraphStyle("SH", fontSize=9, fontName="Helvetica-Bold", textColor=WHITE)),
    Paragraph("Name",  ParagraphStyle("SH", fontSize=9, fontName="Helvetica-Bold", textColor=WHITE)),
    Paragraph("Meaning", ParagraphStyle("SH", fontSize=9, fontName="Helvetica-Bold", textColor=WHITE)),
]]
for num, name, meaning in states:
    bg = STEP_BG if num == "7" else WHITE
    state_data.append([
        Paragraph(num,     ParagraphStyle("SN", fontSize=9, fontName="Courier-Bold", textColor=WYND_ORANGE)),
        Paragraph(name,    ParagraphStyle("SN2", fontSize=9, fontName="Courier", textColor=WYND_DARK)),
        Paragraph(meaning, flag_desc_style),
    ])

st = Table(state_data, colWidths=[0.5*inch, 2.1*inch, 4.0*inch], repeatRows=1)
st.setStyle(TableStyle([
    ("BACKGROUND",    (0, 0), (-1, 0),  FLAG_HEADER),
    ("BACKGROUND",    (0, 8), (-1, 8),  STEP_BG),
    ("ROWBACKGROUNDS",(0, 1), (-1, 7),  [WHITE, CODE_BG]),
    ("GRID",          (0, 0), (-1, -1), 0.4, DIVIDER),
    ("TOPPADDING",    (0, 0), (-1, -1), 5),
    ("BOTTOMPADDING", (0, 0), (-1, -1), 5),
    ("LEFTPADDING",   (0, 0), (-1, -1), 6),
    ("RIGHTPADDING",  (0, 0), (-1, -1), 6),
    ("VALIGN",        (0, 0), (-1, -1), "MIDDLE"),
]))
story.append(st)

story.append(Spacer(1, 0.2*inch))

# ── Communication Stack ───────────────────────────────────────────────────────
story += section("Communication Stack")
story.append(body(
    "Every device interaction travels through four layers before reaching the physical USB port."
))
story.append(Spacer(1, 0.08*inch))

stack_data = [
    ["Layer", "File", "What it does"],
    ["COBS Framing",      "transport/cobs.go",           "Encodes packets so 0x00 is always the frame delimiter. Guarantees clean boundaries on the serial stream."],
    ["Packet",            "transport/packet.go",         "Wraps encoded data in typed packets (ServerRequest, ClientResponse, etc.) with a CRC16 integrity checksum."],
    ["RPC",               "transport/rpc.go",            "Multiplexes unary calls identified by (packageID, serviceID, methodID, invocationID) over a single serial channel."],
    ["Encoding",          "device/encoding.go + proto.go","Hand-rolled protobuf-compatible wire encoding. Field IDs are hardcoded constants — no .proto files or code generation."],
]

sd = Table(
    [[Paragraph(c, ParagraphStyle("TH", fontSize=9, fontName="Helvetica-Bold",
                                   textColor=WHITE if i == 0 else WYND_DARK))
      for c in row] for i, row in enumerate(stack_data)],
    colWidths=[1.4*inch, 1.8*inch, 3.4*inch],
    repeatRows=1,
)
sd.setStyle(TableStyle([
    ("BACKGROUND",    (0, 0), (-1, 0),  FLAG_HEADER),
    ("ROWBACKGROUNDS",(0, 1), (-1, -1), [WHITE, CODE_BG]),
    ("GRID",          (0, 0), (-1, -1), 0.4, DIVIDER),
    ("TOPPADDING",    (0, 0), (-1, -1), 5),
    ("BOTTOMPADDING", (0, 0), (-1, -1), 5),
    ("LEFTPADDING",   (0, 0), (-1, -1), 6),
    ("RIGHTPADDING",  (0, 0), (-1, -1), 6),
    ("VALIGN",        (0, 0), (-1, -1), "TOP"),
    ("FONTNAME",      (0, 1), (1, -1),  "Courier"),
    ("FONTSIZE",      (0, 1), (1, -1),  8),
]))
story.append(sd)

story.append(Spacer(1, 0.2*inch))
story.append(hr())
story.append(Paragraph(
    "wyndctl — internal tool, Wynd Inc.",
    ParagraphStyle("Footer", fontSize=8, textColor=colors.HexColor("#AAAAAA"), alignment=TA_CENTER)
))

doc.build(story)
print(f"Generated: {OUTPUT}")
