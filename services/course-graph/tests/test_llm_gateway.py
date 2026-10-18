"""The real Python→gateway boundary: a gRPC server with the llmgateway.v1
contract from services/shared, no stubbed complete()."""
import asyncio
import json

import grpc
import pytest
from llmgateway.v1 import gateway_pb2, gateway_pb2_grpc

from course_graph.llm.client import GatewayError, ModelClient


class Gateway(gateway_pb2_grpc.GatewayServicer):
    def __init__(self, content='{"remove":[]}', finish_reason="stop"):
        self.requests = []
        self.content, self.finish_reason = content, finish_reason

    async def Complete(self, request, context):
        self.requests.append(request)
        completion = {"choices": [{"index": 0, "message": {"role": "assistant", "content": self.content},
                                   "finish_reason": self.finish_reason}]}
        return gateway_pb2.CompleteResponse(request_id="req-1", completion_json=json.dumps(completion).encode())


async def serve(gateway):
    server = grpc.aio.server()
    gateway_pb2_grpc.add_GatewayServicer_to_server(gateway, server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    return server, f"127.0.0.1:{port}"


def call(gateway, complexity="high", reasoning=False):
    async def run():
        server, address = await serve(gateway)
        client = ModelClient(address, timeout=5)
        try:
            return await client.completer(complexity, reasoning)("Системный промпт", '{"blocks":[]}')
        finally:
            await client.close()
            await server.stop(None)
    return asyncio.run(run())


def test_client_calls_gateway_contract():
    gateway = Gateway()
    assert call(gateway, "low") == '{"remove":[]}'
    request = gateway.requests[0]
    assert request.complexity == gateway_pb2.COMPLEXITY_LOW
    assert [(m.role, m.content) for m in request.messages] == [
        (gateway_pb2.ROLE_SYSTEM, "Системный промпт"), (gateway_pb2.ROLE_USER, '{"blocks":[]}')]
    assert request.parameters.response_format == gateway_pb2.RESPONSE_FORMAT_JSON_OBJECT
    assert request.parameters.reasoning_effort == gateway_pb2.REASONING_EFFORT_LOW


def test_a_reasoning_stage_leaves_the_effort_to_the_model():
    gateway = Gateway()
    call(gateway, "low", reasoning=True)
    assert not gateway.requests[0].parameters.HasField("reasoning_effort")


def test_client_rejects_cut_answer():
    with pytest.raises(GatewayError, match="token limit"):
        call(Gateway(finish_reason="length"))


def test_client_rejects_unknown_complexity():
    async def run():
        client = ModelClient("127.0.0.1:1")
        try:
            with pytest.raises(ValueError):
                client.completer("extreme")
        finally:
            await client.close()
    asyncio.run(run())


def test_clean_endpoint_reaches_gateway(monkeypatch):
    from fastapi.testclient import TestClient
    import course_graph.api as api

    async def start():
        return await serve(gateway)
    gateway = Gateway()
    loop = asyncio.new_event_loop()
    server, address = loop.run_until_complete(start())
    # The server runs on its own loop in a thread while TestClient drives the app.
    import threading
    thread = threading.Thread(target=loop.run_forever, daemon=True)
    thread.start()
    try:
        monkeypatch.setenv("COURSE_GRAPH_LLM_GATEWAY_ADDR", address)
        response = TestClient(api.app).post("/clean", json={"markdown": "# Теория\n\nМатрица — таблица чисел."})
        assert response.status_code == 200
        assert response.json()["markdown"] == "# Теория\n\nМатрица — таблица чисел."
        assert gateway.requests and all(r.complexity == gateway_pb2.COMPLEXITY_LOW for r in gateway.requests)
    finally:
        asyncio.run_coroutine_threadsafe(server.stop(None), loop).result(5)
        loop.call_soon_threadsafe(loop.stop)
        thread.join(5)
