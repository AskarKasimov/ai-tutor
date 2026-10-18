"""Client of the LLM gateway (llmgateway.v1.Gateway/Complete over gRPC).

The service never names a model: every call states only its complexity, and the
gateway picks the pool. Generated stubs come from services/shared/gen/python.
"""
import json
import logging
import os

import grpc
from llmgateway.v1 import gateway_pb2, gateway_pb2_grpc

COMPLEXITY = {
    "low": gateway_pb2.COMPLEXITY_LOW,
    "medium": gateway_pb2.COMPLEXITY_MEDIUM,
    "high": gateway_pb2.COMPLEXITY_HIGH,
}


class GatewayError(RuntimeError):
    pass


# A timed-out or unavailable call is repeated once: the gateway puts the
# instance that failed after healthy ones, so the repeat usually goes to
# another model. One slow call used to fail a whole graph build after
# 15 minutes of finished passes.
RETRIED = {grpc.StatusCode.DEADLINE_EXCEEDED, grpc.StatusCode.UNAVAILABLE}


class ModelClient:
    def __init__(self, address: str | None = None, timeout: float | None = None):
        self.address = address or os.environ.get("COURSE_GRAPH_LLM_GATEWAY_ADDR", "llm-gateway:8011")
        self.timeout = timeout if timeout is not None else float(os.environ.get("COURSE_GRAPH_LLM_TIMEOUT", "300"))
        self.channel = grpc.aio.insecure_channel(
            self.address, options=[("grpc.max_receive_message_length", 40 * 1024 * 1024)],
        )
        self.stub = gateway_pb2_grpc.GatewayStub(self.channel)
        # Repeated calls; stage counters see one logical call per complete, so
        # the endpoints report repeats next to them as retried_requests.
        self.retries = 0

    async def complete(self, system: str, text: str, complexity: str = "high", reasoning: bool = False) -> str:
        # Without reasoning a call asks for low effort, which keeps thinking
        # models fast; with it the model reasons as it does by default.
        parameters = gateway_pb2.GenerationParameters(
            temperature=0.2, response_format=gateway_pb2.RESPONSE_FORMAT_JSON_OBJECT,
        )
        if not reasoning:
            parameters.reasoning_effort = gateway_pb2.REASONING_EFFORT_LOW
        request = gateway_pb2.CompleteRequest(
            messages=[
                gateway_pb2.Message(role=gateway_pb2.ROLE_SYSTEM, content=system),
                gateway_pb2.Message(role=gateway_pb2.ROLE_USER, content=text),
            ],
            complexity=COMPLEXITY[complexity],
            parameters=parameters,
        )
        try:
            response = await self.stub.Complete(request, timeout=self.timeout)
        except grpc.aio.AioRpcError as error:
            if error.code() not in RETRIED:
                raise
            self.retries += 1
            logging.warning("LLM gateway call ended with %s; repeating it once", error.code().name)
            response = await self.stub.Complete(request, timeout=self.timeout)
        try:
            choice = json.loads(response.completion_json)["choices"][0]
            content = choice["message"]["content"]
        except (ValueError, KeyError, IndexError, TypeError) as error:
            raise GatewayError(f"LLM gateway returned no choices (request {response.request_id})") from error
        if choice.get("finish_reason") == "length":
            raise GatewayError(f"LLM answer was cut by the token limit (request {response.request_id})")
        return content or ""

    def completer(self, complexity: str, reasoning: bool = False):
        """complete bound to one complexity, in the (system, text) shape the stages call."""
        if complexity not in COMPLEXITY:
            raise ValueError(f"unknown complexity {complexity!r}")

        async def complete(system: str, text: str) -> str:
            return await self.complete(system, text, complexity, reasoning)
        return complete

    async def close(self):
        await self.channel.close()
