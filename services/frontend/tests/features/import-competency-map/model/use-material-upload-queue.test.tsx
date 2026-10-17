import { act, renderHook, waitFor } from '@testing-library/react'
import { zipSync, strToU8 } from 'fflate'
import { expect, it, vi } from 'vitest'
import { createAppDependencies } from '@/bootstrap/dependencies'
import { createQueryClient } from '@/bootstrap/providers'
import {
  useMaterialUploadQueue,
  useSubjectMaterialsQuery,
} from '@/features/import-competency-map'
import type {
  CourseMaterial,
  CourseMaterialUploadItem,
} from '@/entities/competency-map'
import { createQueryWrapper } from '../../../support/query-wrapper'

function saved(file: File): CourseMaterial {
  return {
    id: file.name,
    subjectId: 'subject-1',
    filename: file.name,
    contentType: file.type,
    fileSize: file.size,
    s3RawKey: 'raw',
    s3ParsedKey: '',
    parsedText: '',
    charCount: 0,
    status: 'pending',
    week: 1,
    kind: 'lecture',
    ocrWarnings: [],
    createdAt: 1,
    updatedAt: 1,
  }
}
it('sends a 49-file ZIP in batches within the master 48-file limit', async () => {
  const dependencies = createAppDependencies()
  const batches: string[][] = []
  dependencies.competencyMap.uploadCourseMaterials = vi.fn(
    async (_subjectId: string, items: CourseMaterialUploadItem[]) => {
      if (items.length > 48) throw new Error('Backend accepts at most 48 files')
      batches.push(items.map((item) => item.file.name))
      return {
        uploaded: items.map((item) => saved(item.file)),
        failed: [],
        results: items.map((item) => ({
          clientId: item.clientId,
          material: saved(item.file),
        })),
      }
    },
  )
  const { result } = renderHook(
    () =>
      useMaterialUploadQueue('user-1', 'subject-1', [
        { code: 'lecture', label: 'Лекция' },
      ]),
    { wrapper: createQueryWrapper(createQueryClient(), dependencies) },
  )
  const archive = zipSync(
    Object.fromEntries(
      Array.from({ length: 49 }, (_, i) => [
        `lecture-${i}.pdf`,
        strToU8('%PDF-1.4 content'),
      ]),
    ),
  )
  await act(() => result.current.addFile(new File([archive], 'course.zip')))
  expect(result.current.items).toHaveLength(49)
  await act(async () => {
    await result.current.sendAll()
  })
  await waitFor(() => expect(result.current.items).toHaveLength(0))
  expect(batches.map((batch) => batch.length)).toEqual([48, 1])
})

it('keeps failed files for retry after partial success', async () => {
  const dependencies = createAppDependencies()
  dependencies.competencyMap.uploadCourseMaterials = vi.fn(
    async (_subjectId: string, items: CourseMaterialUploadItem[]) => ({
      uploaded: [saved(items[0].file)],
      results: [
        { clientId: items[0].clientId, material: saved(items[0].file) },
        {
          clientId: items[1].clientId,
          error: {
            filename: items[1].file.name,
            code: 'UPLOAD_FAILED',
            message: 'Ошибка загрузки',
          },
        },
      ],
      failed: [
        {
          filename: items[1].file.name,
          code: 'UPLOAD_FAILED',
          message: 'Ошибка загрузки',
        },
      ],
    }),
  )
  const { result } = renderHook(
    () =>
      useMaterialUploadQueue('user-1', 'subject-1', [
        { code: 'lecture', label: 'Лекция' },
      ]),
    { wrapper: createQueryWrapper(createQueryClient(), dependencies) },
  )
  await act(() => result.current.addFile(new File(['%PDF-1.4 a'], 'a.pdf')))
  await act(() => result.current.addFile(new File(['%PDF-1.4 b'], 'b.pdf')))
  await act(async () => {
    await result.current.sendAll()
  })
  await waitFor(() => expect(result.current.items).toHaveLength(1))
  expect(result.current.items[0]).toMatchObject({
    file: { name: 'b.pdf' },
    error: 'Ошибка загрузки',
  })
})

it('does not start another batch when the queue unmounts during invalidation', async () => {
  const dependencies = createAppDependencies()
  let releaseReload!: () => void
  const reloadGate = new Promise<void>((resolve) => {
    releaseReload = resolve
  })
  let uploadCount = 0
  let reloadCount = 0
  dependencies.competencyMap.readSubjectMaterials = vi.fn(async () => {
    reloadCount += 1
    if (reloadCount > 1) await reloadGate
    return []
  })
  dependencies.competencyMap.uploadCourseMaterials = vi.fn(
    async (_subjectId: string, items: CourseMaterialUploadItem[]) => {
      uploadCount += 1
      return {
        uploaded: items.map((item) => saved(item.file)),
        failed: [],
        results: items.map((item) => ({
          clientId: item.clientId,
          material: saved(item.file),
        })),
      }
    },
  )
  const { result, unmount } = renderHook(
    () => {
      const queue = useMaterialUploadQueue('user-1', 'subject-1', [
        { code: 'lecture', label: 'Лекция' },
      ])
      useSubjectMaterialsQuery('user-1', 'subject-1')
      return queue
    },
    { wrapper: createQueryWrapper(createQueryClient(), dependencies) },
  )
  await waitFor(() => expect(reloadCount).toBe(1))
  const archive = zipSync(
    Object.fromEntries(
      Array.from({ length: 49 }, (_, i) => [
        `lecture-${i}.pdf`,
        strToU8('%PDF-1.4 content'),
      ]),
    ),
  )
  await act(() => result.current.addFile(new File([archive], 'course.zip')))
  let sending!: Promise<void>
  act(() => {
    sending = result.current.sendAll()
  })
  await waitFor(() => expect(reloadCount).toBe(2))
  unmount()
  releaseReload()
  await sending
  expect(uploadCount).toBe(1)
})
