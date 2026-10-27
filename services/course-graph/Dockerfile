ARG PYTHON_BASE_IMAGE=python:3.11-slim
FROM ${PYTHON_BASE_IMAGE} AS build

WORKDIR /app

RUN if ! dpkg-query -W poppler-utils libgomp1 libgl1 libglib2.0-0 curl >/dev/null 2>&1; then \
    apt-get update && apt-get install -y --no-install-recommends \
    poppler-utils \
    libgomp1 \
    libgl1 \
    libglib2.0-0 \
    curl \
    && rm -rf /var/lib/apt/lists/*; fi

COPY --from=ghcr.io/astral-sh/uv:0.12.17 /uv /uvx /bin/

COPY services/course-graph/pyproject.toml services/course-graph/uv.lock services/course-graph/.python-version ./
ARG INSTALL_DOCLING=true
# cpu: CPU wheels of torch skip ~4 GB of CUDA and triton. cu128: CUDA 12.8 wheels
# for COURSE_GRAPH_DEVICE=gpu; 12.8 is the first that supports RTX 50xx (sm_120).
# The uv cache lives in a BuildKit cache mount, so rebuilds reuse downloads.
ARG TORCH_BACKEND=cpu
ENV UV_LINK_MODE=copy UV_PYTHON_DOWNLOADS=never
ENV PATH="/app/.venv/bin:$PATH"
# A private environment also isolates dependencies from reused OCR base images.
RUN --mount=type=cache,target=/root/.cache/uv \
    case "$TORCH_BACKEND" in cpu) torch_extra=cpu ;; cu128) torch_extra=gpu ;; *) exit 1 ;; esac && \
    case "$INSTALL_DOCLING" in true) ocr_extra="--extra ocr" ;; false) ocr_extra="" ;; *) exit 1 ;; esac && \
    uv sync --frozen --no-dev --no-install-project --extra "$torch_extra" $ocr_extra && \
    python -c "import sys, torch; print('torch', torch.__version__); sys.exit((torch.version.cuda is None) != ('$TORCH_BACKEND' == 'cpu'))"
ENV COURSE_GRAPH_WITH_DOCLING=${INSTALL_DOCLING}
ENV HF_HUB_DISABLE_XET=1
ENV HF_HUB_DOWNLOAD_TIMEOUT=30

COPY services/course-graph/course_graph/ course_graph/
# Stubs of llmgateway.v1, generated in services/shared (import llmgateway.v1).
COPY services/shared/gen/python/ ./

EXPOSE 8000

HEALTHCHECK --interval=15s --timeout=5s --retries=5 \
    CMD curl -f http://localhost:8000/health || exit 1

FROM build AS runtime
CMD ["uv", "run", "--no-sync", "python", "-m", "course_graph.api"]

FROM runtime AS dev
ARG INSTALL_DOCLING=true
ARG TORCH_BACKEND=cpu
RUN case "$TORCH_BACKEND" in cpu) torch_extra=cpu ;; cu128) torch_extra=gpu ;; *) exit 1 ;; esac && \
    case "$INSTALL_DOCLING" in true) ocr_extra="--extra ocr" ;; false) ocr_extra="" ;; *) exit 1 ;; esac && \
    uv sync --frozen --group dev --extra "$torch_extra" $ocr_extra
ENV WATCHFILES_FORCE_POLLING=true
CMD ["watchfiles", "python -m course_graph.api", "course_graph", "config"]
