"""Opt-in smoke test of the real Caddyfile using an isolated local container.

RUN_CADDY_SMOKE=1 python3 -m unittest discover -s scripts/tests -p test_voxcpm_proxy.py
Requires the existing caddy:2-alpine image, Docker and openssl; never pulls images.
"""

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import os
from pathlib import Path
import ssl
import socketserver
import subprocess
import tempfile
import threading
import time
import unittest
import urllib.request


@unittest.skipUnless(os.getenv("RUN_CADDY_SMOKE") == "1", "set RUN_CADDY_SMOKE=1 for local Docker proxy smoke")
class ProxySmoke(unittest.TestCase):
    def test_pcm_arrives_before_upstream_finishes_and_disconnect_reaches_upstream(self):
        received_pcm = threading.Event()
        cancelled = threading.Event()
        paths = []

        class Upstream(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *args):
                pass

            def do_GET(self):
                self.close_connection = True
                paths.append(self.path)
                self.send_response(200)
                self.send_header("Content-Type", "application/x-ndjson")
                self.send_header("Cache-Control", "no-store")
                self.send_header("Transfer-Encoding", "chunked")
                self.end_headers()

                def send(data):
                    self.wfile.write(f"{len(data):x}\r\n".encode() + data + b"\r\n")
                    self.wfile.flush()

                try:
                    send(b'{"type":"start","version":1,"sample_rate":48000,"channels":1,"encoding":"pcm_s16le"}\n')
                    send(b'{"type":"audio","seq":0,"data":"AAA="}\n')
                    if not received_pcm.wait(8):
                        return  # Test client cannot unblock this if the proxy buffers.
                    # Keep writing until the client disconnect reaches the proxy's upstream socket.
                    for _ in range(200):
                        send(b'{"type":"audio","seq":1,"data":"AAA="}\n')
                        time.sleep(.01)
                except (BrokenPipeError, ConnectionResetError):
                    cancelled.set()

        class LocalServer(ThreadingHTTPServer):
            def server_bind(self):
                # HTTPServer's getfqdn(0.0.0.0) can block on unrelated external DNS.
                socketserver.TCPServer.server_bind(self)
                self.server_name = "localhost"
                self.server_port = self.server_address[1]

        upstream = LocalServer(("0.0.0.0", 0), Upstream)
        thread = threading.Thread(target=upstream.serve_forever, daemon=True)
        thread.start()
        container = None
        try:
            with tempfile.TemporaryDirectory() as directory:
                temp = Path(directory)
                subprocess.run(["docker", "image", "inspect", "caddy:2-alpine"], check=True, stdout=subprocess.DEVNULL)
                subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
                    "-subj", "/CN=localhost", "-keyout", str(temp / "server-key.pem"), "-out", str(temp / "server.pem")],
                    check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                source = Path(__file__).parents[2] / "services/caddy/Caddyfile"
                config = source.read_text().replace("api:8002", f"host.docker.internal:{upstream.server_port}")
                (temp / "Caddyfile").write_text(config)
                container = subprocess.check_output(["docker", "run", "--rm", "-d", "--pull=never", "-p", "127.0.0.1::443",
                    "-v", f"{temp}:/certs:ro", "-v", f"{temp / 'Caddyfile'}:/etc/caddy/Caddyfile:ro", "caddy:2-alpine"], text=True).strip()
                port = subprocess.check_output(["docker", "port", container, "443/tcp"], text=True).strip().rsplit(":", 1)[1]
                url = f"https://127.0.0.1:{port}/api/v1/diagnostic-sessions/s/turns/t/audio/stream?part=question"
                context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
                context.check_hostname = False
                context.verify_mode = ssl.CERT_NONE  # Only a freshly generated local test certificate.
                response = None
                for _ in range(50):
                    try:
                        response = urllib.request.urlopen(url, context=context, timeout=5)
                        break
                    except OSError:
                        time.sleep(.1)
                self.assertIsNotNone(response, "isolated Caddy did not start")
                with response:
                    self.assertEqual(response.headers.get("Cache-Control"), "no-store")
                    self.assertIsNone(response.headers.get("Content-Length"))
                    self.assertIsNone(response.headers.get("Content-Encoding"))
                    self.assertIn(b'"type":"start"', response.readline())
                    self.assertIn(b'"type":"audio"', response.readline())
                    received_pcm.set()
                self.assertTrue(cancelled.wait(5), "downstream disconnect did not reach upstream")
                self.assertEqual(paths, ["/diagnostic-sessions/s/turns/t/audio/stream?part=question"])
        finally:
            received_pcm.set()
            if container:
                subprocess.run(["docker", "rm", "-f", container], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            upstream.shutdown()
            upstream.server_close()


if __name__ == "__main__":
    unittest.main()
