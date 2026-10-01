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
