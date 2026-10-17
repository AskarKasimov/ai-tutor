ARG PYTHON_BASE_IMAGE=python:3.11-slim
FROM ${PYTHON_BASE_IMAGE}

WORKDIR /app

RUN if ! dpkg-query -W poppler-utils libgomp1 libgl1 libglib2.0-0 curl >/dev/null 2>&1; then \
    apt-get update && apt-get install -y --no-install-recommends \
    poppler-utils \
    libgomp1 \
    libgl1 \
    libglib2.0-0 \
    curl \
    && rm -rf /var/lib/apt/lists/*; fi

COPY --from=ghcr.io/astral-sh/uv:latest /uv /uvx /bin/

COPY services/course-graph/requirements.txt services/course-graph/pyproject.toml ./
ARG INSTALL_DOCLING=true
# Models run on CPU only: CPU wheels of torch skip ~4 GB of CUDA and triton.
# The uv cache lives in a BuildKit cache mount, so rebuilds reuse downloads.
ENV UV_TORCH_BACKEND=cpu UV_LINK_MODE=copy
RUN --mount=type=cache,target=/root/.cache/uv \
    if [ "$INSTALL_DOCLING" = "true" ]; then \
      uv pip install --system -r requirements.txt; \
    elif [ "$INSTALL_DOCLING" = "false" ]; then \
      sed '/^docling[<=>]/d' requirements.txt > /tmp/requirements-light.txt && \
      uv pip install --system -r /tmp/requirements-light.txt; \
    else exit 1; fi
ENV COURSE_GRAPH_WITH_DOCLING=${INSTALL_DOCLING}
ENV HF_HUB_DISABLE_XET=1
ENV HF_HUB_DOWNLOAD_TIMEOUT=30

COPY services/course-graph/course_graph/ course_graph/
# Stubs of llmgateway.v1, generated in services/shared (import llmgateway.v1).
COPY services/shared/gen/python/ ./

ENV PORT=8000
EXPOSE 8000

HEALTHCHECK --interval=15s --timeout=5s --retries=5 \
    CMD curl -f http://localhost:8000/health || exit 1

CMD ["python", "-m", "course_graph.api"]
