import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { CosmicProgress } from '@/shared/ui'

it('reports a whole, clamped percent to assistive technology', () => {
  const { rerender } = render(
    <CosmicProgress value={42.6} label="Диагностика" valueText="43%" />,
  )
  const bar = screen.getByRole('progressbar', { name: 'Диагностика' })
  expect(bar).toHaveAttribute('aria-valuenow', '43')
  expect(bar).toHaveAttribute('aria-valuetext', '43%')
  rerender(<CosmicProgress value={140} label="Диагностика" />)
  expect(bar).toHaveAttribute('aria-valuenow', '100')
})

it('starts empty at zero and sweeps without a value when progress is unknown', () => {
  const { rerender } = render(<CosmicProgress value={0} label="Сборка" />)
  const bar = screen.getByRole('progressbar', { name: 'Сборка' })
  expect(bar).toHaveAttribute('aria-valuenow', '0')
  expect(bar).not.toHaveAttribute('aria-busy')
  rerender(<CosmicProgress value={null} label="Сборка" />)
  expect(bar).not.toHaveAttribute('aria-valuenow')
  expect(bar).toHaveAttribute('aria-busy', 'true')
})
