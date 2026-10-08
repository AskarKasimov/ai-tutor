export type SessionToken = { epoch: number; userId: string | null }

export class SessionRegistry {
  private epoch = 0
  private userId: string | null = null
  private readonly requests = new Map<number, Set<AbortController>>()

  capture(): SessionToken {
    return { epoch: this.epoch, userId: this.userId }
  }

  isCurrent(token: SessionToken): boolean {
    return this.epoch === token.epoch && this.userId === token.userId
  }

  register(token: SessionToken, controller: AbortController): () => void {
    if (!this.isCurrent(token)) {
      controller.abort()
      return () => undefined
    }
    let requests = this.requests.get(token.epoch)
    if (!requests) {
      requests = new Set()
      this.requests.set(token.epoch, requests)
    }
    requests.add(controller)
    return () => {
      requests?.delete(controller)
      if (requests?.size === 0) this.requests.delete(token.epoch)
    }
  }

  replace(userId: string | null, expectedEpoch?: number): boolean {
    if (expectedEpoch !== undefined && this.epoch !== expectedEpoch)
      return false
    const oldEpoch = this.epoch
    this.epoch++
    this.userId = userId
    for (const controller of this.requests.get(oldEpoch) ?? [])
      controller.abort()
    this.requests.delete(oldEpoch)
    return true
  }
}
