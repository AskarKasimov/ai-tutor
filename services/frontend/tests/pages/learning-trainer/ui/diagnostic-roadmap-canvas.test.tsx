import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import '@/shared/i18n/i18n'
import DiagnosticRoadmapCanvas from '@/pages/learning-trainer/ui/diagnostic-roadmap-canvas'

describe('DiagnosticRoadmapCanvas', () => {
  const sampleOutline = {
    competencies: [
      {
        id: 'c1',
        name: 'Линейные модели',
        constituents: [
          {
            id: 't1',
            name: 'Метод наименьших квадратов',
            outcomes: [
              {
                id: 'o1',
                name: 'Оценивать коэффициенты МНК',
                role: 'main' as const,
              },
              {
                id: 'o2',
                name: 'Проверять гипотезы о значимости',
                role: 'basic' as const,
              },
            ],
          },
        ],
      },
      {
        id: 'c2',
        name: 'Деревья решений',
        constituents: [
          {
            id: 't2',
            name: 'Критерии ветвления',
            outcomes: [
              {
                id: 'o3',
                name: 'Вычислять критерий Джини',
                role: 'main' as const,
              },
            ],
          },
        ],
      },
    ],
  }

  it('renders central subject node and branches for competencies and constituents', async () => {
    render(
      <DiagnosticRoadmapCanvas
        outline={sampleOutline}
        subjectTitle="Машинное обучение"
      />,
    )

    // Central Subject node
    expect(
      await screen.findByLabelText('Машинное обучение'),
    ).toBeInTheDocument()
    expect(screen.getByText('Предмет')).toBeInTheDocument()

    // Competencies
    expect(await screen.findByLabelText('Линейные модели')).toBeInTheDocument()
    expect(await screen.findByLabelText('Деревья решений')).toBeInTheDocument()

    // Constituents
    expect(
      await screen.findByLabelText('Метод наименьших квадратов'),
    ).toBeInTheDocument()
    expect(
      await screen.findByLabelText('Критерии ветвления'),
    ).toBeInTheDocument()

    // Outcomes are not shown before selection
    expect(
      screen.queryByText('Оценивать коэффициенты МНК'),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText('Вычислять критерий Джини'),
    ).not.toBeInTheDocument()
  })

  it('shows educational outcomes in details inspector when constituent is clicked', async () => {
    render(
      <DiagnosticRoadmapCanvas
        outline={sampleOutline}
        subjectTitle="Машинное обучение"
      />,
    )

    fireEvent.click(await screen.findByLabelText('Метод наименьших квадратов'))

    // Details panel appears
    const inspector = await screen.findByRole('complementary', {
      name: 'Метод наименьших квадратов',
    })
    expect(inspector).toBeVisible()
    expect(screen.getByText('Оценивать коэффициенты МНК')).toBeVisible()
    expect(screen.getByText('Проверять гипотезы о значимости')).toBeVisible()
    expect(screen.getByText('Основной')).toBeVisible()
    expect(screen.getByText('Базовый')).toBeVisible()

    // Other topic outcomes are still not in document
    expect(
      screen.queryByText('Вычислять критерий Джини'),
    ).not.toBeInTheDocument()

    // Close inspector
    fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
    expect(
      screen.queryByRole('complementary', {
        name: 'Метод наименьших квадратов',
      }),
    ).not.toBeInTheDocument()
  })

  it('toggles fullscreen mode on fullscreen button click', async () => {
    const { container } = render(
      <DiagnosticRoadmapCanvas
        outline={sampleOutline}
        subjectTitle="Машинное обучение"
      />,
    )

    const wrapper =
      container.querySelector('[data-fullscreen]') ||
      container.firstElementChild
    expect(wrapper).toHaveAttribute('data-fullscreen', 'false')

    fireEvent.click(
      await screen.findByRole('button', { name: 'Во весь экран' }),
    )
    expect(wrapper).toHaveAttribute('data-fullscreen', 'true')

    fireEvent.click(screen.getByRole('button', { name: 'Свернуть' }))
    expect(wrapper).toHaveAttribute('data-fullscreen', 'false')
  })
})
