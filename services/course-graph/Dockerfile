FROM python:3.11-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    poppler-utils \
    libgomp1 \
    libgl1 \
    libglib2.0-0 \
    curl \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY course_graph/ course_graph/

ENV PORT=8000
EXPOSE 8000

HEALTHCHECK --interval=15s --timeout=5s --retries=5 \
    CMD curl -f http://localhost:8000/health || exit 1

CMD ["python", "-m", "course_graph.api"]
