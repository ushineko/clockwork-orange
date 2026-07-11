"""Tests for the single-instance signaling helper.

When a second launch is blocked by the instance lock, it should reach the
running instance over a local socket and ask it to surface its window instead
of exiting silently. These tests exercise the protocol without a GUI display
(QtNetwork only, QCoreApplication event loop).

The full cross-process behaviour (second launch restores a tray-minimized
window) is verified end-to-end manually; a single in-process event loop cannot
faithfully reproduce the client-in-one-process / server-in-another timing that
``_notify_existing_instance`` is built for.
"""
import pytest
from PyQt6.QtCore import QCoreApplication
from PyQt6.QtNetwork import QLocalServer, QLocalSocket

from gui.main_window import _notify_existing_instance


@pytest.fixture(scope="module")
def qapp():
    app = QCoreApplication.instance() or QCoreApplication([])
    yield app


def test_notify_returns_false_when_no_instance_listening(qapp):
    # Nothing is listening on this name: there is no running instance to reach,
    # so the lock-held branch must fall through cleanly (no hang, no exception).
    assert _notify_existing_instance("clockwork_orange_test_absent_server") is False


def test_server_receives_show_payload_from_client(qapp):
    # Contract test: a server started the way the primary instance starts one
    # receives the "SHOW" payload a secondary instance sends. The client socket
    # is held alive here so a single shared event loop can accept and read it.
    name = "clockwork_orange_test_protocol_server"
    QLocalServer.removeServer(name)
    server = QLocalServer()
    received = {}
    conns = []  # keep accepted connections alive past the slot

    def on_connection():
        conn = server.nextPendingConnection()
        conns.append(conn)
        conn.readyRead.connect(
            lambda c=conn: received.setdefault("data", bytes(c.readAll()))
        )

    server.newConnection.connect(on_connection)
    assert server.listen(name)

    client = QLocalSocket()
    try:
        client.connectToServer(name)
        assert client.waitForConnected(1000)
        client.write(b"SHOW")
        client.flush()
        for _ in range(200):
            if received:
                break
            server.waitForNewConnection(10)
            qapp.processEvents()
        assert received.get("data") == b"SHOW"
    finally:
        client.abort()
        server.close()
        QLocalServer.removeServer(name)
