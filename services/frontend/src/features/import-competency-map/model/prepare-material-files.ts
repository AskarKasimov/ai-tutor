import { AsyncUnzipInflate, Unzip } from 'fflate'

export const MATERIAL_ARCHIVE_LIMIT = 250 * 1024 * 1024
export const MATERIAL_FILE_LIMIT = 25 * 1024 * 1024
export const MATERIAL_ENTRY_LIMIT = 500
export const MATERIAL_QUEUE_LIMIT = 250 * 1024 * 1024

export class MaterialArchiveError extends Error {
  constructor(
    message: string,
    readonly paths: string[] = [],
  ) {
    super(message)
    this.name = 'MaterialArchiveError'
  }
}

function isPdf(name: string, data: Uint8Array) {
  if (!name.toLowerCase().endsWith('.pdf')) return false
  return new TextDecoder('latin1')
    .decode(data.subarray(0, 1024))
    .includes('%PDF-')
}

function isHiddenOrServiceEntry(name: string) {
  const segments = name.split('/')
  return segments.some(
    (segment) =>
      segment.startsWith('.') ||
      segment === '__MACOSX' ||
      segment.toLowerCase() === 'thumbs.db',
  )
}

export async function prepareMaterialFiles(file: File): Promise<File[]> {
  if (file.size > MATERIAL_ARCHIVE_LIMIT)
    throw new MaterialArchiveError('archive-too-large')
  if (file.name.toLowerCase().endsWith('.pdf')) {
    const data = new Uint8Array(await file.arrayBuffer())
    if (data.byteLength > MATERIAL_FILE_LIMIT)
      throw new MaterialArchiveError('file-too-large', [file.name])
    if (!isPdf(file.name, data))
      throw new MaterialArchiveError('invalid-pdf', [file.name])
    return [file]
  }
  if (!file.name.toLowerCase().endsWith('.zip'))
    throw new MaterialArchiveError('invalid-file-type', [file.name])

  const unzip = new Unzip()
  unzip.register(AsyncUnzipInflate)
  const entries: { name: string; chunks: Uint8Array[]; bytes: number }[] = []
  const invalid: string[] = []
  let totalBytes = 0
  let failure: Error | undefined
  let inputDone = false
  let finishedFiles = 0
  let activeReader: ReadableStreamDefaultReader<Uint8Array> | undefined
  const activeEntries: Array<{ terminate: () => void }> = []
  let finishCallback = () => {}
  const fail = (error: Error) => {
    failure = error
    void activeReader?.cancel(error)
    for (const entry of activeEntries) entry.terminate()
    finishCallback()
  }
  const complete = new Promise<void>((resolve, reject) => {
    const finish = () => {
      if (failure) reject(failure)
      else if (inputDone && finishedFiles === entries.length) resolve()
    }
    finishCallback = finish
    unzip.onfile = (entry) => {
      if (entry.name.endsWith('/')) return
      if (entries.length + invalid.length >= MATERIAL_ENTRY_LIMIT) {
        fail(new MaterialArchiveError('too-many-files'))
        entry.terminate()
        return
      }
      if (
        !entry.name.toLowerCase().endsWith('.pdf') ||
        isHiddenOrServiceEntry(entry.name)
      ) {
        invalid.push(entry.name)
        finish()
        return
      }
      const current = { name: entry.name, chunks: [] as Uint8Array[], bytes: 0 }
      activeEntries.push(entry)
      entries.push(current)
      entry.ondata = (err, chunk, final) => {
        if (err) {
          fail(err)
          entry.terminate()
          return
        }
        current.bytes += chunk.length
        totalBytes += chunk.length
        if (
          current.bytes > MATERIAL_FILE_LIMIT ||
          totalBytes > MATERIAL_QUEUE_LIMIT
        ) {
          fail(new MaterialArchiveError('expanded-too-large'))
          entry.terminate()
          return
        }
        current.chunks.push(chunk)
        if (final) {
          finishedFiles++
          const activeIndex = activeEntries.indexOf(entry)
          if (activeIndex >= 0) activeEntries.splice(activeIndex, 1)
          finish()
        }
      }
      try {
        entry.start()
      } catch (error) {
        fail(error as Error)
        entry.terminate()
      }
    }
    void (async () => {
      let archiveBytes = 0
      if (typeof file.stream === 'function') {
        const reader = file.stream().getReader()
        activeReader = reader
        while (true) {
          const { value, done } = await reader.read()
          if (done) break
          archiveBytes += value.byteLength
          if (archiveBytes > MATERIAL_ARCHIVE_LIMIT)
            throw new MaterialArchiveError('archive-too-large')
          unzip.push(value, false)
          if (failure) throw failure
        }
      } else {
        const bytes = new Uint8Array(await file.arrayBuffer())
        for (let offset = 0; offset < bytes.length; offset += 64 * 1024) {
          const chunk = bytes.subarray(
            offset,
            Math.min(offset + 64 * 1024, bytes.length),
          )
          archiveBytes += chunk.length
          if (archiveBytes > MATERIAL_ARCHIVE_LIMIT)
            throw new MaterialArchiveError('archive-too-large')
          unzip.push(chunk, false)
          if (failure) throw failure
        }
      }
      unzip.push(new Uint8Array(), true)
      inputDone = true
      finish()
    })().catch((error: unknown) =>
      fail(error instanceof Error ? error : new Error(String(error))),
    )
  })
  await complete
  if (failure) throw failure
  if (invalid.length)
    throw new MaterialArchiveError('contains-non-pdf', invalid)
  if (!entries.length) throw new MaterialArchiveError('empty-archive')
  return entries.map(({ name, chunks, bytes }) => {
    const data = new Uint8Array(bytes)
    let offset = 0
    for (const chunk of chunks) {
      data.set(chunk, offset)
      offset += chunk.length
    }
    if (!isPdf(name, data))
      throw new MaterialArchiveError('invalid-pdf', [name])
    return new File([data], name.split('/').pop() || name, {
      type: 'application/pdf',
    })
  })
}
