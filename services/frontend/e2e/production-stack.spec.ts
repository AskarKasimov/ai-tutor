import { expect, test } from '@playwright/test'

test.skip(!process.env.E2E_STACK_URL, 'requires the unified production stack')

test('real production stack keeps student and admin sessions isolated', async ({
  page,
}) => {
  const requests: Array<{ path: string; portal: string | undefined }> = []
  page.on('request', (request) => {
    const url = new URL(request.url())
    if (!url.pathname.startsWith('/api/v1/')) return
    requests.push({
      path: url.pathname.replace('/api/v1', ''),
      portal: request.headers()['x-ai-tutor-portal'],
    })
  })

  await page.goto('/')
  await expect(page.getByLabel('Пароль')).toBeVisible()
  await page.getByRole('button', { name: 'Регистрация' }).click()
  await page.getByLabel('Имя').fill('Проверка стека')
  await page.getByLabel('Email').fill(`stack-${Date.now()}@example.invalid`)
  await page.getByLabel('Пароль').fill('stack-smoke-test-password')
  await page.getByRole('button', { name: 'Создать аккаунт' }).click()
  await expect(
    page.getByRole('heading', { name: 'Выберите режим' }),
  ).toBeVisible()

  await page.getByRole('link', { name: /Обучение/ }).click()
  await expect(page).toHaveURL(/\/learn$/)
  await expect(page.getByRole('heading', { name: 'Курсы' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Курсы' })).toBeVisible()
  await page.getByRole('button', { name: /Меню профиля/ }).click()
  await page.getByRole('menuitem', { name: 'К выбору режима' }).click()
  await page.getByRole('link', { name: /Симуляция экзамена/ }).click()
  await expect(page).toHaveURL(/\/exam$/)
  await expect(page.getByRole('heading', { name: 'Курсы' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Курсы' })).toBeVisible()

  await page.goto('/admin/learn/competency-map')
  await expect(page).toHaveURL(/\/admin\/learn\/competency-map$/)
  await expect(
    page.getByRole('heading', { name: 'Карта компетенций' }),
  ).toBeVisible()
  await expect
    .poll(() =>
      requests.some(
        ({ path, portal }) => path === '/auth/teacher' && portal === 'admin',
      ),
    )
    .toBe(true)

  await page.goto('/learn')
  await expect(page).toHaveURL(/\/learn$/)
  await page.getByRole('button', { name: /Меню профиля/ }).click()
  await page.getByRole('menuitem', { name: 'Выйти' }).click()
  await expect(page.getByLabel('Пароль')).toBeVisible()

  await page.goto('/admin/courses')
  await expect(page).toHaveURL(/\/admin\/courses$/)
  await expect(page.getByRole('heading', { name: 'Курсы' })).toBeVisible()
  await expect
    .poll(() =>
      requests.some(
        ({ path, portal }) => path === '/auth/me' && portal === 'admin',
      ),
    )
    .toBe(true)
})
