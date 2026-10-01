# Frontend AI Tutor

React 19, TypeScript, Vite 8,
TanStack Router (файловые маршруты), TanStack Query, i18next / react-i18next,
SCSS Modules, Vitest и Testing Library. Для UI используется Radix Themes.

## Запуск

Требуется Node.js 22.12+ и npm. Из этой директории:

```bash
npm ci
npm run dev
```

Откройте http://localhost:5173. На `/` находится единственная страница-заглушка.
Обращений к backend пока нет; переменные окружения не требуются.

## Структура

- `src/app/` — запуск, провайдеры Radix/Query и глобальные стили.
- `src/routes/` — файловые маршруты и их SCSS Modules.
- `src/i18n/` — локализация; пользовательский текст хранится в переводах.
- `src/shared/` — место для общих компонентов, типов и утилит.
- `src/data/` — место для контрактов данных и query/mutation hooks.
- `src/mocks/` — место для тестовых источников данных и фикстур.
- `src/platform/` — место для адаптеров браузерных API.
- `src/test/` — настройка Testing Library и jsdom.

`src/routeTree.gen.ts` генерируется плагином Router при запуске Vite/Vitest
или сборке. После добавления маршрутов сначала запустите `npm run dev`
либо `npm run build`, затем typecheck. Сгенерированный файл хранится в Git.
Правила разработки находятся в [AGENTS.md](AGENTS.md).

## Проверки

```bash
npm run lint
npm run typecheck
npm test -- --run
npm run build
```

Эти же проверки выполняются в Frontend CI. `npm run preview` запускает
предпросмотр готовой сборки.

## Docker

Из корня монорепозитория:

```bash
docker build -t ai-tutor-frontend services/frontend
docker run --rm -p 8080:80 ai-tutor-frontend
```

Сборка обслуживается nginx на http://localhost:8080 с fallback для SPA-маршрутов.
Фронтенд включён в общий Docker Compose. Для запуска всего приложения
выполните `./run.sh -d` из корня монорепозитория после настройки `.env`
по корневому README. Приложение доступно на https://localhost:8443; Caddy
направляет `/api/v1` и `/api/v1/*` в backend, удаляя префикс `/api/v1`.
`/health` также обслуживает backend; остальные пути идут во frontend.
Порт nginx доступен только внутри Docker-сети.
