import type { Subject } from '@/entities/subject'

export const mockSubjectCatalog: Subject[] = [
  { id: 'subject:demo-a', name: 'Демонстрационный предмет A', ready: true },
  { id: 'subject:demo-b', name: 'Демонстрационный предмет B', ready: true },
]

export function findMockSubject(id: string) {
  return mockSubjectCatalog.find((subject) => subject.id === id)
}
