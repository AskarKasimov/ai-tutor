import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'

export const i18n = i18next.use(initReactI18next)

void i18n.init({
  fallbackLng: 'ru',
  initAsync: false,
  interpolation: { escapeValue: false },
  lng: 'ru',
  resources: {
    ru: {
      translation: {
        auth: {
          login: 'Войти', loginTab: 'Вход', register: 'Регистрация', logout: 'Выйти', close: 'Закрыть',
          loginTitle: 'Вход в AI Tutor', registerTitle: 'Создать аккаунт',
          subtitle: 'Войдите, чтобы озвучивать вопросы и распознавать свои ответы.',
          email: 'Email', password: 'Пароль', name: 'Имя (необязательно)',
          passwordHelp: 'От 15 до 128 символов.', create: 'Создать аккаунт', pending: 'Подождите…', checkingSession: 'Проверяем сессию…',
          networkError: 'Не удалось связаться с сервером. Попробуйте ещё раз.',
        },
        trainer: {
          title: 'Голосовой тренажёр',
          course: 'ПРИКЛАДНАЯ СТАТИСТИКА',
          question: 'Банк по данным клиента прогнозирует: «вернёт кредит в срок» или «не вернёт».',
          optionsTitle: 'Варианты ответа',
          options: { classification: 'Классификация', regression: 'Регрессия', clustering: 'Кластеризация', ranking: 'Ранжирование' },
          voiceInstruction: 'Назовите тип задачи и кратко объясните выбор.',
          playInstruction: 'Прослушать инструкцию', stopSpeech: 'Остановить',
          answerArea: 'Голосовой ответ на задание', answer: 'Ответьте голосом',
          answerHelp: 'Нажмите «Начать запись» и ответьте.\nПосле завершения ответ отправится на распознавание.',
          start: 'Начать запись', stopRecording: 'Завершить запись',
          recording: 'Идёт запись · {{time}}',
          recordingHelp: 'Говорите ответ в обычном темпе.\nЗавершите запись, когда будете готовы.',
          permission: 'Доступ к микрофону', permissionHelp: 'Разрешите использование микрофона\nв появившемся запросе браузера.', allow: 'Ожидаем разрешение…',
          processing: 'Обрабатываем ответ', processingHelp: 'Запись завершена.\nРаспознаём вашу речь.', processingHint: 'Распознавание может занять некоторое время', busy: 'Обработка…',
          grading: 'Оцениваем ответ', gradingHelp: 'Расшифровка готова.\nПроверяем ответ по заданию.', gradingHint: 'Оценка может занять некоторое время',
          yourAnswer: 'Ваш ответ', exampleTitle: 'Пример ответа', exampleAnswer: 'Классификация: целевая переменная имеет два класса.',
          voiceAnswer: 'Голосовой ответ', sideHelp: 'Прослушайте инструкцию.\nНазовите тип задачи и объясните выбор.',
          sideHint: 'После ответа здесь появятся\nбалл и объяснение.',
          score: 'Оценка', feedback: 'Обратная связь',
          exampleFeedback: 'Верно: банк прогнозирует одно из двух значений целевой переменной, поэтому это задача классификации.',
          exampleNote: 'Пример результата · не оценка вашей записи',
          finished: 'Демонстрационный результат готов', again: 'Попробовать ещё раз', preview: 'Посмотреть пример',
          listenRecording: 'Прослушать вашу запись',
          denied: 'Разрешите доступ к микрофону', deniedHelp: 'В настройках сайта разрешите микрофон,\nзатем попробуйте начать запись ещё раз.',
          unavailable: 'Микрофон недоступен', unavailableHelp: 'Проверьте подключение микрофона и откройте\nтренажёр через HTTPS или localhost.',
          captureHelp: 'Не удалось сохранить запись. Попробуйте ещё раз.',
          capture: 'Не удалось сохранить запись', retry: 'Попробовать снова',
          cancelSpeech: 'Отменить озвучивание',
          transcriptionFinished: 'Ответ распознан', transcriptionReady: 'Ответ получен',
          noAssessment: 'Ваша запись распознана.\nАвтоматическая оценка пока не подключена.',
          transcription: 'Не удалось распознать ответ',
          transcriptionHelp: 'Проверьте подключение к голосовому API\nи попробуйте записать ответ ещё раз.',
          assessment: 'Не удалось оценить ответ', assessmentHelp: 'Расшифровка сохранена. Попробуйте оценить ответ ещё раз.',
          assessmentErrorTitle: 'Оценка временно недоступна', assessmentErrorHelp: 'Ваш ответ распознан. Повторите проверку без новой записи.',
          retryAssessment: 'Повторить оценку',
          unauthorized: 'Требуется авторизация',
          unauthorizedHelp: 'Войдите в AI Tutor и попробуйте снова.\nДля обработки ответа нужна действующая сессия.',
          unauthorizedAssessmentHelp: 'Войдите в AI Tutor и повторите оценку сохранённого ответа.',
          speechUnauthorized: 'Для озвучивания инструкции войдите в AI Tutor.',
          speechError: 'Не удалось озвучить инструкцию. Прочитайте её и ответьте голосом.',
        },
        routes: {
          home: {
            title: 'AI Tutor',
            description: 'Здесь скоро появится AI-репетитор.',
          },
        },
      },
    },
  },
})
