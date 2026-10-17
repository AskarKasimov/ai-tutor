import { describe, expect, it } from 'vitest'
import type { CourseMaterial } from '@/entities/competency-map'
import {
  materialInProgress,
  materialsRefetchInterval,
} from '@/features/import-competency-map'

function material(overrides: Partial<CourseMaterial>): CourseMaterial {
  return {
    id: 'm',
    subjectId: 's',
    filename: 'week1.pdf',
    contentType: 'application/pdf',
    fileSize: 1,
    s3RawKey: 'raw',
    s3ParsedKey: 'parsed',
    parsedText: '',
    charCount: 0,
    status: 'parsed',
    week: null,
    kind: null,
    ocrWarnings: [],
    createdAt: 1,
    updatedAt: 1,
    ...overrides,
  }
}

describe('admin polling', () => {
  it('polls materials until every one is terminal', () => {
    const ocr = material({
      status: 'pending',
      processing: {
        stage: 'ocr',
        status: 'processing',
        updatedAt: 1,
        attempts: 1,
      },
    })
    const cleaning = material({
      cleaning: {
        status: 'pending',
        updatedAt: 1,
        attempts: 0,
        errorMessage: '',
      },
    })
    const done = material({
      cleaning: {
        status: 'ready',
        updatedAt: 1,
        attempts: 1,
        errorMessage: '',
      },
    })
    expect(materialInProgress(ocr)).toBe(true)
    expect(materialInProgress(cleaning)).toBe(true)
    expect(materialInProgress(material({ status: 'failed' }))).toBe(false)
    expect(materialsRefetchInterval([done, cleaning])).toBeGreaterThan(0)
    expect(materialsRefetchInterval([done])).toBe(false)
  })
})
