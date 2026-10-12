import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { TrainingWorkspace } from '@/pages/trainer/ui/training-workspace'
import '@/shared/i18n'

afterEach(() => vi.unstubAllGlobals())

function renderWorkspace() {
  render(
    <TrainingWorkspace chat={<p>Диалог</p>}>
      <p>Задание</p>
    </TrainingWorkspace>,
  )
  return screen.getByRole('separator', { name: 'Изменить ширину панели чата' })
}

it('places the conversation in a named complementary panel', () => {
  renderWorkspace()
  const panel = screen.getByRole('complementary', { name: 'Репетитор' })
  expect(panel).toContainElement(screen.getByText('Диалог'))
  expect(panel).not.toContainElement(screen.getByText('Задание'))
})

it('resizes with the keyboard, respects limits and resets with a double click', () => {
  const handle = renderWorkspace()
  expect(handle).toHaveAttribute('aria-valuenow', '420')
  fireEvent.keyDown(handle, { key: 'ArrowLeft' })
  expect(handle).toHaveAttribute('aria-valuenow', '452')
  fireEvent.keyDown(handle, { key: 'ArrowRight', shiftKey: true })
  expect(handle).toHaveAttribute('aria-valuenow', '388')
  fireEvent.keyDown(handle, { key: 'Home' })
  fireEvent.keyDown(handle, { key: 'ArrowRight' })
  expect(handle).toHaveAttribute('aria-valuenow', '320')
  fireEvent.keyDown(handle, { key: 'End' })
  fireEvent.keyDown(handle, { key: 'ArrowLeft' })
  expect(handle).toHaveAttribute(
    'aria-valuenow',
    handle.getAttribute('aria-valuemax'),
  )
  fireEvent.doubleClick(handle)
  expect(handle).toHaveAttribute('aria-valuenow', '420')
})

it('drags the boundary and stops resizing after releasing or cancelling the pointer', () => {
  class TestPointerEvent extends MouseEvent {
    pointerId: number
    constructor(type: string, init: PointerEventInit) {
      super(type, init)
      this.pointerId = init.pointerId ?? 1
    }
  }
  vi.stubGlobal('PointerEvent', TestPointerEvent)
  const handle = renderWorkspace()
  Object.assign(handle, {
    setPointerCapture: vi.fn(),
    releasePointerCapture: vi.fn(),
  })
  vi.spyOn(screen.getByRole('main'), 'getBoundingClientRect').mockReturnValue({
    width: 1000,
  } as DOMRect)
  fireEvent.pointerDown(handle, { pointerId: 1, button: 0, clientX: 600 })
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 500 })
  expect(handle).toHaveAttribute('aria-valuenow', '520')
  fireEvent.pointerUp(handle, { pointerId: 1 })
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 400 })
  expect(handle).toHaveAttribute('aria-valuenow', '520')
  fireEvent.pointerDown(handle, { pointerId: 2, button: 0, clientX: 500 })
  fireEvent.pointerMove(handle, { pointerId: 2, clientX: 1000 })
  expect(handle).toHaveAttribute('aria-valuenow', '320')
  fireEvent.pointerCancel(handle, { pointerId: 2 })
  fireEvent.pointerMove(handle, { pointerId: 2, clientX: 100 })
  expect(handle).toHaveAttribute('aria-valuenow', '320')
})
