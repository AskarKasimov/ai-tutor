# ai-tutor

Монорепозиторий сервисов для работы с речью.

## GigaAM

Исходный код модели, HTTP API, Dockerfile и `uv.lock` находятся в
[`gigaam`](gigaam/README_ru.md). Все команды `uv` для GigaAM
запускайте из этого каталога.

Запуск HTTP API из корня монорепозитория:

```bash
docker compose up -d --build gigaam
```

Проверка: `curl http://localhost:8000/health`.

По умолчанию выбран `v3_e2e_rnnt`: он возвращает текст с пунктуацией и
нормализацией, что удобно для диалога с учеником. Если важнее минимальная
ошибка распознавания без оформления текста, в [оценке авторов](gigaam/evaluation.md)
`v3_rnnt` показывает средний WER 8,3% против 11,2% у `v3_e2e_rnnt`.
Модель меняется через `MODEL_NAME` в Compose. Исходный код GigaAM взят из
[официального репозитория](https://github.com/salute-developers/GigaAM), коммит
`7447938d791c4f3e643386ee22c33777004293a5`.

## VoxCPM2

Официальный исходный код [OpenBMB/VoxCPM](https://github.com/OpenBMB/VoxCPM)
с HTTP API находится в [`voxcpm2`](voxcpm2/API.md). Используется исходный
коммит `f772e498a45fbb5fb8e13fbf9b9c48be9fe33e69`.

```bash
docker compose up -d --build voxcpm2
curl http://localhost:8001/health
curl -X POST http://localhost:8001/synthesize \
  -H 'Content-Type: application/json' \
  -d '{"text":"Привет, мир!"}' --output speech.wav
```

Первый запрос скачает веса модели в Docker volume. В Docker Desktop на macOS
сервис работает на CPU; синтез может быть медленным.

`VoxCPM2` — актуальная модель в линейке VoxCPM с поддержкой русского языка.
Она выбрана ради качества и возможностей синтеза, а не скорости CPU-инференса.
