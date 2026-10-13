import { act, renderHook } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { createAppDependencies } from '@/bootstrap/dependencies'
import { createQueryClient } from '@/bootstrap/providers'
import { useAnswerPreview } from '@/features/learning-plan'
import { createQueryWrapper } from '../../support/query-wrapper'

afterEach(() => vi.restoreAllMocks())

it('aborts recognition and ignores a late transcript after changing questions', async () => {
  const dependencies = createAppDependencies()
  let finish!: (value: { id: string; text: string }) => void
  let oldSignal!: AbortSignal
  const preview = vi.spyOn(dependencies.learningPlan, 'previewAnswer')
  preview.mockImplementationOnce((_blob, signal) => {
    oldSignal = signal
    return new Promise((resolve) => {
      finish = resolve
    })
  })
  preview.mockResolvedValue({ id: 'new', text: 'Новый ответ' })
  const view = renderHook(({ identity }) => useAnswerPreview(identity), {
    initialProps: { identity: 'p:0:old' },
    wrapper: createQueryWrapper(createQueryClient(), dependencies),
  })
  let pending!: Promise<void>
  act(() => {
    pending = view.result.current.recognize(new Blob(['old']))
  })
  view.rerender({ identity: 'p:0:new' })
  expect(oldSignal.aborted).toBe(true)
  await act(async () => {
    await view.result.current.recognize(new Blob(['new']))
  })
  await act(async () => {
    finish({ id: 'old', text: 'Старый ответ' })
    await pending
  })
  expect(view.result.current.data).toEqual({ id: 'new', text: 'Новый ответ' })
  expect(view.result.current.busy).toBe(false)
})

it('allows retrying STT without another recording and clears the failed draft on rerecord', async () => {
  const dependencies = createAppDependencies()
  const preview = vi
    .spyOn(dependencies.learningPlan, 'previewAnswer')
    .mockRejectedValueOnce(new Error('offline'))
    .mockResolvedValue({ id: 'ready', text: 'Ответ' })
  const view = renderHook(() => useAnswerPreview('p:0:q'), {
    wrapper: createQueryWrapper(createQueryClient(), dependencies),
  })
  const blob = new Blob(['answer'])
  await act(async () => {
    await view.result.current.recognize(blob)
  })
  expect(view.result.current.data).toBeNull()
  expect(view.result.current.error).toBeDefined()
  await act(async () => {
    await view.result.current.retry()
  })
  expect(preview.mock.calls[0][0]).toBe(preview.mock.calls[1][0])
  expect(view.result.current.data?.text).toBe('Ответ')
  act(() => view.result.current.clear())
  expect(view.result.current.data).toBeNull()
  expect(view.result.current.error).toBeUndefined()
})
