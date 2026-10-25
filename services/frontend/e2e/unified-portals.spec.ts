import { expect, test } from '@playwright/test'

test('one browser context switches modes while student and admin sessions stay separate', async ({
  page,
}, testInfo) => {
  let studentSession = false
  let adminSession = false
  const calls: Array<{
    path: string
    method: string
    portal: string | undefined
  }> = []
  const subject = {
    id: 'subject-1',
    name: 'Введение в искусственный интеллект',
    ready: true,
    learning_ready: true,
    exam_ready: true,
  }

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace('/api/v1', '')
    const portal = request.headers()['x-ai-tutor-portal']
    calls.push({ path, method: request.method(), portal })

    if (path === '/auth/me') {
      const user = portal === 'admin' ? adminSession : studentSession
      if (!user) {
        await route.fulfill({ status: 401, body: '' })
        return
      }
      await route.fulfill({
        json: {
          id: portal === 'admin' ? 'teacher-1' : 'student-1',
          email:
            portal === 'admin'
              ? 'teacher@example.invalid'
              : 'student@example.invalid',
          display_name: portal === 'admin' ? 'Учитель' : 'Студент',
          role: portal === 'admin' ? 'admin' : 'student',
        },
      })
      return
    }
    if (path === '/auth/refresh') {
      await route.fulfill({ status: 401, body: '' })
      return
    }
    if (path === '/auth/register' && request.method() === 'POST') {
      studentSession = true
      await route.fulfill({
        json: {
          user: {
            id: 'student-1',
            email: 'student@example.invalid',
            display_name: 'Студент',
            role: 'student',
          },
        },
      })
      return
    }
    if (path === '/auth/teacher' && request.method() === 'POST') {
      adminSession = true
      await route.fulfill({
        json: {
          user: {
            id: 'teacher-1',
            email: 'teacher@example.invalid',
            display_name: 'Учитель',
            role: 'admin',
          },
        },
      })
      return
    }
    if (path === '/auth/logout' && request.method() === 'POST') {
      if (portal === 'admin') adminSession = false
      else studentSession = false
      await route.fulfill({ status: 204, body: '' })
      return
    }
    if (path.endsWith('/subjects')) {
      await route.fulfill({ json: [subject] })
      return
    }
    if (path.endsWith('/learning-state')) {
      await route.fulfill({
        json: {
          subject_id: subject.id,
          subject_name: subject.name,
          diagnostic_status: 'not_started',
          diagnostic_completed: false,
          training_available: false,
        },
      })
      return
    }
    if (path.endsWith('/material-kinds')) {
      await route.fulfill({ json: [] })
      return
    }
    if (path === '/exam/diagnostic-sessions' && request.method() === 'GET') {
      await route.fulfill({
        status: 503,
        json: { code: 'PROCESSING_UNAVAILABLE' },
      })
      return
    }
    await route.fulfill({ status: 404, json: { code: 'NOT_FOUND' } })
  })

  await page.goto('/')
  await expect(page.getByLabel('Пароль')).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('start-login.png') })
  await page.getByRole('button', { name: 'Регистрация' }).click()
  await page.getByLabel('Имя').fill('Студент')
  await page.getByLabel('Email').fill('student@example.invalid')
  await page.getByLabel('Пароль').fill('test-password')
  await page.getByRole('button', { name: 'Создать аккаунт' }).click()
  await expect(
    page.getByRole('heading', { name: 'Выберите режим' }),
  ).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('start-choice.png') })

  await page.getByRole('link', { name: /Обучение/ }).click()
  await expect(page).toHaveURL(/\/learn$/)
  await expect(page.getByRole('heading', { name: 'Курсы' })).toBeVisible()
  await expect(
    page.getByRole('navigation', { name: 'Направления' }),
  ).toContainText('Обучение')
  await expect(
    page.getByRole('navigation', { name: 'Направления' }),
  ).toContainText('Симуляция экзамена')
  await page.reload()
  await expect(
    page
      .getByRole('navigation', { name: 'Направления' })
      .getByRole('link', { name: 'Обучение' }),
  ).toHaveAttribute('aria-current', 'page')
  await page.screenshot({ path: testInfo.outputPath('learn-home.png') })
  await page.getByRole('button', { name: /Меню профиля/ }).click()
  await page.getByRole('menuitem', { name: 'К выбору режима' }).click()
  await page.getByRole('link', { name: /Симуляция экзамена/ }).click()
  await expect(page).toHaveURL(/\/exam$/)
  await expect(page.getByRole('heading', { name: 'Курсы' })).toBeVisible()
  await expect(
    page
      .getByRole('navigation', { name: 'Направления' })
      .getByRole('link', { name: 'Симуляция экзамена' }),
  ).toHaveAttribute('aria-current', 'page')
  await page.reload()
  await expect(
    page
      .getByRole('navigation', { name: 'Направления' })
      .getByRole('link', { name: 'Симуляция экзамена' }),
  ).toHaveAttribute('aria-current', 'page')
  await page.screenshot({ path: testInfo.outputPath('exam-home.png') })

  await page.goto('/admin/learn/competency-map')
  await expect(page).toHaveURL(/\/admin\/learn\/competency-map$/)
  await expect(
    page.getByRole('heading', { name: 'Материалы для обучения' }),
  ).toBeVisible()
  const learnNav = page.getByRole('navigation', {
    name: 'Разделы администрирования',
  })
  await expect(
    learnNav.getByRole('link', { name: 'Материалы для обучения' }),
  ).toHaveAttribute('aria-current', 'page')
  await expect(
    learnNav.getByRole('link', { name: 'Материалы для экзамена' }),
  ).toHaveAttribute('href', '/admin/courses')
  await expect(page.getByLabel('Файл карты компетенций')).toBeVisible()
  await expect(page.getByRole('heading', { name: subject.name })).toBeVisible()
  await expect(page.getByText('Материалы готовы')).toBeVisible()
  await expect(
    page.getByRole('button', {
      name: /Создать курс|Добавление нового предмета/,
    }),
  ).toHaveCount(0)
  expect(
    calls.some(
      (call) =>
        call.method === 'POST' &&
        ['/learn/admin/subjects', '/exam/admin/subjects'].includes(call.path),
    ),
  ).toBe(false)
  await page.goto(`/admin/learn/competency-map/${subject.id}`)
  await expect(page.getByRole('heading', { name: subject.name })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: subject.name })).toBeVisible()
  await expect(page.getByLabel('Добавить PDF или ZIP')).toHaveCount(0)
  await expect(
    page
      .getByRole('navigation', { name: 'Разделы администрирования' })
      .getByRole('link', { name: 'Материалы для обучения' }),
  ).toHaveAttribute('aria-current', 'page')
  await page.screenshot({ path: testInfo.outputPath('learn-admin.png') })
  await expect
    .poll(() =>
      calls.some(
        (call) => call.path === '/auth/teacher' && call.portal === 'admin',
      ),
    )
    .toBe(true)
  await page.goto('/learn')
  await expect(page).toHaveURL(/\/learn$/)
  await expect
    .poll(() =>
      calls.some(
        (call) => call.path === '/auth/me' && call.portal === undefined,
      ),
    )
    .toBe(true)

  await page.getByRole('button', { name: /Меню профиля/ }).click()
  await page.getByRole('menuitem', { name: 'Выйти' }).click()
  await expect(page.getByLabel('Пароль')).toBeVisible()

  await page.goto('/admin/courses')
  await expect(page).toHaveURL(/\/admin\/courses$/)
  await expect(
    page.getByRole('heading', { name: 'Материалы для экзамена' }),
  ).toBeVisible()
  await expect(page.getByText('Экзамен: готов')).toBeVisible()
  await expect(page.getByText('Обучение: готово')).toBeVisible()
  const examNav = page.getByRole('navigation', {
    name: 'Разделы администрирования',
  })
  await expect(
    examNav.getByRole('link', { name: 'Материалы для экзамена' }),
  ).toHaveAttribute('aria-current', 'page')
  await expect(
    examNav.getByRole('link', { name: 'Материалы для обучения' }),
  ).toHaveAttribute('href', '/admin/learn/competency-map')
  await page.getByRole('link', { name: subject.name }).click()
  await expect(page).toHaveURL(/\/admin\/competency-map\/subject-1$/)
  await expect(
    page.getByRole('heading', { name: 'Материалы для экзамена' }),
  ).toBeVisible()
  await expect(page.getByLabel('Добавить PDF или ZIP')).toBeVisible()
  await expect(page.getByLabel('Файл карты компетенций')).toHaveCount(0)
  await page.reload()
  await expect(page.getByLabel('Добавить PDF или ZIP')).toBeVisible()
  await expect(
    page
      .getByRole('navigation', { name: 'Разделы администрирования' })
      .getByRole('link', { name: 'Материалы для экзамена' }),
  ).toHaveAttribute('aria-current', 'page')
  await page.screenshot({ path: testInfo.outputPath('exam-admin.png') })
  await expect
    .poll(() =>
      calls.some((call) => call.path === '/auth/me' && call.portal === 'admin'),
    )
    .toBe(true)
})
