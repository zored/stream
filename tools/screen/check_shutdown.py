from pathlib import Path
import socket
import subprocess
import tempfile
import signal
root=Path(__file__).resolve().parent/'Sources'
with tempfile.TemporaryDirectory(prefix='screen-shutdown-') as d:
    tmp=Path(d)
    server=socket.socket();server.bind(('127.0.0.1',0));server.listen();server.settimeout(10)
    port=server.getsockname()[1]
    paths=[]
    for f in list((root/'ScreenCore').glob('*.swift'))+list((root/'screen').glob('*.swift')):
        text=f.read_text().replace('import ScreenCore\n','')
        if f.name=='SatelliteConnection.swift':
            assert 'port: 16622' in text
            assert '            socket.cancel()\n            done()\n        })' in text
            text=text.replace('port: 16622','port: '+str(port))
            text=text.replace('        socket.send(content: Data(Satellite.removeDevice.utf8),', '        DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {\n        socket.send(content: Data(Satellite.removeDevice.utf8),')
            text=text.replace('            socket.cancel()\n            done()\n        })', '            socket.cancel()\n            done()\n        })\n        }')
        dest=tmp/f.name;dest.write_text(text);paths.append(str(dest))
    subprocess.run(['swiftc','-swift-version','5','-module-cache-path','/private/tmp/stream-clang-cache',*paths,'-o',str(tmp/'screen')],check=True)
    proc=subprocess.Popen([str(tmp/'screen'),'--probe'],stdout=subprocess.DEVNULL,stderr=subprocess.PIPE)
    try:
        conn,_=server.accept();conn.settimeout(5)
        conn.sendall(b'BEGIN CompanionVersion=5 ApiVersion=1.14.0\n')
        stream=conn.makefile('rb')
        assert stream.readline().startswith(b'ADD-DEVICE ')
        proc.send_signal(signal.SIGTERM)
        lines=stream.read()
        code=proc.wait(timeout=5)
        assert b'REMOVE-DEVICE DEVICEID=zored-screen\n' in lines,(code,lines,proc.stderr.read())
        assert code==0,code
        print('shutdown ok: REMOVE-DEVICE before EOF, exit 0')
    finally:
        if proc.poll() is None:proc.kill();proc.wait()
        server.close()
