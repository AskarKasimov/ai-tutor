import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { createAppDependencies } from '@/bootstrap/dependencies'
import { createQueryClient } from '@/bootstrap/providers'
import { MaterialUploadQueue } from '@/features/import-competency-map/ui/material-upload-queue'
import { createQueryWrapper } from '../../../support/query-wrapper'

it('shows file errors in a dismissible dialog and reopens for the same error', async () => {
  render(
    <MaterialUploadQueue
      userId="user-1"
      subjectId="subject-1"
      kinds={[{ code: 'lecture', label: 'Лекция' }]}
    />,
    {
      wrapper: createQueryWrapper(createQueryClient(), createAppDependencies()),
    },
  )
  const input = screen.getByLabelText('Добавить PDF или ZIP')
  const selectInvalidFile = () =>
    fireEvent.change(input, {
      target: { files: [new File(['text'], 'notes.txt')] },
    })
  selectInvalidFile()
  const dialog = await screen.findByRole('dialog', {
    name: 'Не удалось загрузить материалы',
  })
  expect(dialog).toHaveAccessibleDescription('Выберите PDF или ZIP.: notes.txt')
  fireEvent.click(screen.getByRole('button', { name: 'Понятно' }))
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
  )
  selectInvalidFile()
  await screen.findByRole('dialog')
  fireEvent.keyDown(screen.getByRole('dialog'), {
    key: 'Escape',
    code: 'Escape',
    keyCode: 27,
  })
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
  )
})

it('shows an upload error and preserves the file for another attempt', async () => {
  const dependencies = createAppDependencies()
  dependencies.competencyMap.uploadCourseMaterials = vi
    .fn()
    .mockRejectedValue(new Error('Сервер недоступен'))
  render(
    <MaterialUploadQueue
      userId="user-1"
      subjectId="subject-1"
      kinds={[{ code: 'lecture', label: 'Лекция' }]}
    />,
    { wrapper: createQueryWrapper(createQueryClient(), dependencies) },
  )
  fireEvent.change(screen.getByLabelText('Добавить PDF или ZIP'), {
    target: { files: [new File(['%PDF-1.4 data'], 'lecture.pdf')] },
  })
  fireEvent.click(await screen.findByRole('button', { name: 'Отправить всё' }))
  expect(await screen.findByRole('dialog')).toHaveAccessibleDescription(
    'Сервер недоступен',
  )
  fireEvent.click(screen.getByRole('button', { name: 'Закрыть уведомление' }))
  expect(screen.getByText('lecture.pdf')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Отправить всё' })).toBeEnabled()
})
