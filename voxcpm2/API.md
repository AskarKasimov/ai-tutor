# VoxCPM2 HTTP API

Этот сервис использует исходный код [OpenBMB/VoxCPM](https://github.com/OpenBMB/VoxCPM)
из коммита `f772e498a45fbb5fb8e13fbf9b9c48be9fe33e69`. Образ использует
CPU-версию PyTorch; сборка проверена на Linux ARM64.

Из корня монорепозитория:

```bash
docker compose up -d --build voxcpm2
curl http://localhost:8001/health
curl -X POST http://localhost:8001/synthesize \
  -H 'Content-Type: application/json' \
  -d '{"text":"Привет, мир!"}' --output speech.wav
```

`POST /synthesize` принимает JSON с полями `text` (обязательно), `cfg_value`
(по умолчанию 2.0), `inference_timesteps` (10) и `seed` (необязательно).
Ответ — `audio/wav`. Веса `openbmb/VoxCPM2` загружаются при первом запросе и
сохраняются в Docker volume `voxcpm2-cache`. На CPU синтез может занимать
значительное время.

Для локальной проверки API без загрузки весов:

```bash
cd voxcpm2
SETUPTOOLS_SCM_PRETEND_VERSION=2.0.3 uv sync --extra api --extra api-test
uv run --no-sync python -m pytest -q tests/test_api.py
```
