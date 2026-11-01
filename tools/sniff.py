import os, sys, termios, time

port, seconds = sys.argv[1], float(sys.argv[2])
fd = os.open(port, os.O_RDWR | os.O_NOCTTY)
attrs = termios.tcgetattr(fd)
attrs[0] = 0
attrs[1] = 0
attrs[2] = termios.CS8 | termios.CREAD | termios.CLOCAL
attrs[3] = 0
attrs[4] = attrs[5] = termios.B9600
attrs[6][termios.VMIN] = 0
attrs[6][termios.VTIME] = 1
termios.tcsetattr(fd, termios.TCSANOW, attrs)
termios.tcflush(fd, termios.TCIOFLUSH)

os.write(fd, b"GET /index.html\nHTTP/1.1\nConnection: Upgrade\nUpgrade: websocket\nSec-WebSocket-Key: 123abc\n\n")

buf = b""
deadline = time.time() + 5
while b"\r\n\r\n" not in buf:
    if time.time() > deadline:
        sys.exit(f"no handshake reply: {buf!r}")
    buf += os.read(fd, 4096)
head, buf = buf.split(b"\r\n\r\n", 1)
print("handshake:", head.split(b"\r\n")[0].decode(), flush=True)

start = time.time()
while time.time() - start < seconds:
    buf += os.read(fd, 4096)
    while len(buf) >= 2:
        if buf[0] != 0x82:
            print(f"{time.time()-start:7.2f} junk {buf[:1].hex()}", flush=True)
            buf = buf[1:]
            continue
        n = buf[1] & 0x7F
        if len(buf) < 2 + n:
            break
        payload = buf[2:2 + n]
        buf = buf[2 + n:]
        length, cmd, tx = payload[0], payload[1], payload[2]
        print(f"{time.time()-start:7.2f} len={length} framelen={n} cmd=0x{cmd:02x} tx={tx} data={payload[3:].hex()}", flush=True)
os.close(fd)
