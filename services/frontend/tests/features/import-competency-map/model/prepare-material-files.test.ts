import { zipSync, strToU8 } from 'fflate'
import { expect, it } from 'vitest'
import { prepareMaterialFiles } from '@/features/import-competency-map/model/prepare-material-files'

it('accepts one signed PDF', async () => {
  const file = new File(['%PDF-1.7 body'], 'week.pdf')
  await expect(prepareMaterialFiles(file)).resolves.toEqual([file])
})

it('rejects an archive containing any non-PDF entry', async () => {
  const bytes = zipSync({
    'week.pdf': strToU8('%PDF-1.7'),
    'notes.txt': strToU8('no'),
  })
  const file = new File([bytes], 'materials.zip')
  await expect(prepareMaterialFiles(file)).rejects.toMatchObject({
    paths: ['notes.txt'],
  })
})

it('rejects hidden and service PDF entries in an archive', async () => {
  const bytes = zipSync({
    'week.pdf': strToU8('%PDF-1.7'),
    '.hidden.pdf': strToU8('%PDF-1.7 hidden'),
    '__MACOSX/week.pdf': strToU8('%PDF-1.7 metadata'),
    '._week.pdf': strToU8('%PDF-1.7 apple metadata'),
  })
  const file = new File([bytes], 'materials.zip')
  await expect(prepareMaterialFiles(file)).rejects.toMatchObject({
    paths: ['.hidden.pdf', '__MACOSX/week.pdf', '._week.pdf'],
  })
})

it('extracts PDF entries from a ZIP while preserving only client-side files', async () => {
  const bytes = zipSync({
    'week-1.pdf': strToU8('%PDF-1.7 one'),
    'week-2.pdf': strToU8('%PDF-1.7 two'),
  })
  const result = await prepareMaterialFiles(new File([bytes], 'materials.zip'))
  expect(result.map((item) => item.name)).toEqual(['week-1.pdf', 'week-2.pdf'])
})
