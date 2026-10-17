import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { createAppDependencies } from '@/bootstrap/dependencies'
import { createQueryClient } from '@/bootstrap/providers'
import { MaterialUploadQueue } from '@/features/import-competency-map/ui/material-upload-queue'
import { createQueryWrapper } from '../../../support/query-wrapper'

it('preserves queue order through drag-and-drop, keyboard reordering and upload', async () => {
  const dependencies = createAppDependencies()
  dependencies.competencyMap.uploadCourseMaterials = vi
    .fn()
    .mockResolvedValue({ uploaded: [], failed: [] })
  render(<MaterialUploadQueue userId="user-1" subjectId="subject-1" />, {
    wrapper: createQueryWrapper(createQueryClient(), dependencies),
  })

  const input = screen.getByLabelText('Добавить PDF или ZIP')
  for (const name of ['Lecture 10.pdf', 'Lecture 2.pdf']) {
    fireEvent.change(input, {
      target: { files: [new File(['%PDF-1.7 content'], name)] },
    })
    await waitFor(() => expect(screen.getByText(name)).toBeInTheDocument())
  }

  let rows = screen.getAllByRole('listitem')
  expect(rows[0]).toHaveTextContent('Lecture 10.pdf')
  expect(rows[1]).toHaveTextContent('Lecture 2.pdf')

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
  expect(rows[0]).toHaveAttribute('draggable', 'true')
  const lectureTenDragHandle = screen.getByRole('button', {
    name: 'Перетащить Lecture 2.pdf',
  })
  expect(lectureTenDragHandle).toHaveAttribute('draggable', 'true')
  fireEvent.dragStart(lectureTenDragHandle, { dataTransfer: transfer })
  fireEvent.dragOver(rows[0], { dataTransfer: transfer })
  fireEvent.drop(rows[0], { dataTransfer: transfer })

  rows = screen.getAllByRole('listitem')
  expect(rows[0]).toHaveTextContent('Lecture 2.pdf')

  fireEvent.click(
    screen.getByRole('button', {
      name: 'Переместить Lecture 2.pdf ниже',
    }),
  )
  rows = screen.getAllByRole('listitem')
  expect(rows[0]).toHaveTextContent('Lecture 10.pdf')
  expect(rows[1]).toHaveTextContent('Lecture 2.pdf')

  fireEvent.click(screen.getByRole('button', { name: 'Отправить всё' }))
  const uploadMaterials = dependencies.competencyMap.uploadCourseMaterials
  await waitFor(() => expect(uploadMaterials).toHaveBeenCalled())
  const sentItems = vi.mocked(uploadMaterials).mock.calls[0][1]
  expect(sentItems.map((item) => item.name)).toEqual([
    'Lecture 10.pdf',
    'Lecture 2.pdf',
  ])
})
