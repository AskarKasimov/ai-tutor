import os
import grpc
from course_graph.llm import llm_pb2, llm_pb2_grpc


class ModelClient:
    def __init__(self):
        self.channel = grpc.aio.insecure_channel(
            os.environ.get("COURSE_GRAPH_LLM_GRPC_TARGET", "api:8003"),
            options=[("grpc.max_receive_message_length", 128 * 1024)],
        )
        self.stub = llm_pb2_grpc.LLMStub(self.channel)

    async def complete(self, system: str, text: str) -> str:
        response = await self.stub.Complete(
            llm_pb2.CompleteRequest(system=system, input=text), timeout=120,
        )
        return response.content

    async def close(self):
        await self.channel.close()
