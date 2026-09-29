FROM nvcr.io/nvidia/tritonserver:24.10-py3
COPY --from=ghcr.io/astral-sh/uv:0.12.17 /uv /uvx /bin/

RUN uv pip install --system --no-cache \
    "torch>=2.6,<2.11" \
    "torchaudio>=2.6,<2.11" \
    hydra-core==1.3.* \
    soundfile==0.13.* \
    omegaconf==2.3.* \
    sentencepiece \
    onnxruntime-gpu \
    tqdm
