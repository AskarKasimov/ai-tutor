import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import '@/shared/i18n/i18n'
import DiagnosticRoadmapCanvas from '@/pages/learning-trainer/ui/diagnostic-roadmap-canvas'

describe('DiagnosticRoadmapCanvas', () => {
  const sampleOutline = {
    competencies: [
      {
        id: 'c1',
        name: 'К1: Линейные модели',
        constituents: [
          {
            id: 't1',
            name: '1.1. Метод наименьших квадратов',
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
        name: 'Компетенция 2: Деревья решений',
        constituents: [
          {
            id: 't2',
            name: '2.1. Критерии ветвления',
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

  it('renders central subject node and branches with clean titles only (no К1, no ОР or count badges)', async () => {
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

    // Competencies stripped of "К1:" and "Компетенция 2:"
    expect(await screen.findByLabelText('Линейные модели')).toBeInTheDocument()
    expect(await screen.findByLabelText('Деревья решений')).toBeInTheDocument()
    expect(screen.queryByText('К1:')).not.toBeInTheDocument()

    // Constituents stripped of "1.1." and "2.1."
    expect(
      await screen.findByLabelText('Метод наименьших квадратов'),
    ).toBeInTheDocument()
    expect(
      await screen.findByLabelText('Критерии ветвления'),
    ).toBeInTheDocument()

    // No badges for "Компетенция", "Тем: X" or "X ОР" inside nodes
    expect(screen.queryByText('Компетенция')).not.toBeInTheDocument()
    expect(screen.queryByText(/2 ОР/)).not.toBeInTheDocument()
    expect(screen.queryByText(/1 ОР/)).not.toBeInTheDocument()

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
      name: '1.1. Метод наименьших квадратов',
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
        name: '1.1. Метод наименьших квадратов',
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

  it('handles mouse wheel zoom smoothly on the canvas container', async () => {
    const { container } = render(
      <DiagnosticRoadmapCanvas
        outline={sampleOutline}
        subjectTitle="Машинное обучение"
      />,
    )

    const wrapper =
      container.querySelector('[data-fullscreen]') ||
      container.firstElementChild
    expect(wrapper).toBeInTheDocument()

    expect(() => {
      fireEvent.wheel(wrapper!, { deltaY: -100, clientX: 200, clientY: 200 })
    }).not.toThrow()
  })

  it('renders nodes with circular icon hubs and titles positioned on top', async () => {
    render(
      <DiagnosticRoadmapCanvas
        outline={sampleOutline}
        subjectTitle="Машинное обучение"
      />,
    )

    const subjectNode = await screen.findByLabelText('Машинное обучение')
    expect(subjectNode).toBeInTheDocument()
    expect(subjectNode.querySelector('svg')).toBeInTheDocument()

    const topicNode = await screen.findByLabelText('Метод наименьших квадратов')
    expect(topicNode).toBeInTheDocument()
    expect(topicNode.querySelector('svg')).toBeInTheDocument()
    expect(topicNode).toHaveAttribute('data-selected', 'false')

    fireEvent.click(topicNode)
    expect(topicNode).toHaveAttribute('data-selected', 'true')
  })
})
