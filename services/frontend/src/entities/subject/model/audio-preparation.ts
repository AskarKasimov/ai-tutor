export type AudioPreparationState =
  | 'no_graph'
  | 'checking'
  | 'queued'
  | 'processing'
  | 'retry_wait'
  | 'partial'
  | 'ready'
  | 'empty'

export type AudioPreparationDirection = {
  version: string
  state: AudioPreparationState
  total: number
  unchecked: number
  queued: number
  processing: number
  retry_wait: number
  ready: number
  failed: number
  unavailable: number
  observed_at: string
}

export type AudioPreparation = {
  subject_id: string
  exam: (AudioPreparationDirection & { published: boolean }) | null
  learn: (AudioPreparationDirection & { revision: number }) | null
}
