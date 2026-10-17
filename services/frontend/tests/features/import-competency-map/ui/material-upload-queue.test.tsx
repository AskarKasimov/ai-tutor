import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { createAppDependencies } from '@/bootstrap/dependencies'
import { createQueryClient } from '@/bootstrap/providers'
import { MaterialUploadQueue } from '@/features/import-competency-map/ui/material-upload-queue'
import { createQueryWrapper } from '../../../support/query-wrapper'

it('sorts by week and supports drag-and-drop and keyboard reordering', async () => {
  const dependencies = createAppDependencies()
  dependencies.competencyMap.uploadCourseMaterials = vi
    .fn()
    .mockResolvedValue({ results: [] })
  render(
    <MaterialUploadQueue
      userId="user-1"
      subjectId="subject-1"
      kinds={[{ code: 'lecture', label: 'Лекция' }]}
    />,
    { wrapper: createQueryWrapper(createQueryClient(), dependencies) },
  )

  const input = screen.getByLabelText('Добавить PDF или ZIP')
  for (const name of ['Lecture 10.pdf', 'Lecture 2.pdf']) {
    fireEvent.change(input, {
      target: { files: [new File(['%PDF-1.7 content'], name)] },
    })
    await waitFor(() => expect(screen.getByText(name)).toBeInTheDocument())
  }

  const weeks = screen.getAllByRole('spinbutton')
  fireEvent.change(weeks[0], { target: { value: '10' } })
  fireEvent.change(weeks[1], { target: { value: '2' } })

  let rows = screen.getAllByRole('listitem')
  expect(rows[0]).toHaveTextContent('Lecture 2.pdf')
  expect(rows[1]).toHaveTextContent('Lecture 10.pdf')

  const transfer = {
    data: {} as Record<string, string>,
    effectAllowed: 'none',
    dropEffect: 'none',
    setData(type: string, value: string) {
      this.data[type] = value
    },
    getData(type: string) {
      return this.data[type] ?? ''
    },
  }
  expect(rows[1]).toHaveAttribute('draggable', 'true')
  fireEvent.dragStart(rows[1], { dataTransfer: transfer })
  fireEvent.dragOver(rows[0], { dataTransfer: transfer })
  fireEvent.drop(rows[0], { dataTransfer: transfer })

  rows = screen.getAllByRole('listitem')
  expect(rows[0]).toHaveTextContent('Lecture 10.pdf')

  const reorderedWeeks = screen.getAllByRole('spinbutton')
  fireEvent.change(reorderedWeeks[1], { target: { value: '1' } })
  rows = screen.getAllByRole('listitem')
  expect(rows[0]).toHaveTextContent('Lecture 2.pdf')

  const lectureTwoHandle = screen.getByRole('button', {
    name: 'Переместить Lecture 2.pdf. Используйте стрелки вверх и вниз или перетащите.',
  })
  fireEvent.keyDown(lectureTwoHandle, { key: 'ArrowDown' })
  rows = screen.getAllByRole('listitem')
  expect(rows[0]).toHaveTextContent('Lecture 10.pdf')
  expect(rows[1]).toHaveTextContent('Lecture 2.pdf')

  fireEvent.click(screen.getByRole('button', { name: 'Отправить всё' }))
  const uploadMaterials = dependencies.competencyMap.uploadCourseMaterials
  await waitFor(() => expect(uploadMaterials).toHaveBeenCalled())
  const sentItems = vi.mocked(uploadMaterials).mock.calls[0][1]
  expect(sentItems.map((item) => item.file.name)).toEqual([
    'Lecture 10.pdf',
    'Lecture 2.pdf',
  ])
})
