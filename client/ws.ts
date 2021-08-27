type iterator = { value: Blob, done: false } | { value: undefined, done: true }
const key = Symbol()

class Peer {
  private readonly _messages = new Array<Blob>()
  private readonly _waiters = new Array<{ resolve: (o: iterator) => void, reject: (err: Error | ErrorEvent) => void }>()
  private _socket?: WebSocket

  constructor(
    private readonly _id: string,
    private _host: string,
    private _auth?: string,
  ) {
  }

  async send(peer: Peer, data: Blob | ArrayBuffer | ArrayBufferView | ReadableStream<Uint8Array>) {
    const response = await fetch(
      peer.toURL().toString(),
      {
        method: 'POST',
        keepalive: true,
        body: data,
      })
    if (!response.ok) {
      throw new Error(response.statusText)
    }
  }

  private _onClose() {
    while (true) {
      const waiter = this._waiters.shift()
      if (!waiter) {
        break
      }
      waiter.resolve({value: undefined, done: true})
    }
  }

  private _onError(ev: Event) {
    try {
      this._socket?.close()
    } catch (_) {
    }
    while (true) {
      const waiter = this._waiters.shift()
      if (!waiter) {
        break
      }
      waiter.resolve({value: undefined, done: true})
    }
  }

  get connected() {
    return this._socket?.readyState === WebSocket.OPEN
  }

  private _onOpen() {

  }

  set onclose(fn: (ev?: Event) => void) {
    this._socket?.addEventListener('error',(ev)=>{
      fn(ev)
    })
  }

  [Symbol.asyncIterator]() {
    return this;
  }

  next() {
    return new Promise<iterator>((resolve, reject) => {
      if (!this.connected) {
        return reject(new Error('NOT CONNECTED'))
      }
      const data = this._messages.shift()
      if (data) {
        return resolve({value: data, done: false})
      }
      this._waiters.push({resolve, reject})
    })
  }

  private _onMessage({data}: MessageEvent) {
    const waiter = this._waiters.shift()
    if (waiter) {
      return waiter.resolve({value: data, done: false})
    }
    this._messages.push(data)
  }

  connect() {
    return new Promise<unknown>((resolve, reject) => {
      if (this._socket) {
        return resolve(undefined)
      }
      this._socket = new WebSocket(this.toURL('wss').toString())
      this._socket.addEventListener('error', this._onError.bind(this))
      this._socket.addEventListener('open', this._onOpen.bind(this))
      this._socket.addEventListener('close', this._onClose.bind(this))
      this._socket.addEventListener('message', this._onMessage.bind(this))
      this._socket.addEventListener('error', reject, {once: true})
      this._socket.addEventListener('open', resolve, {once: true})
    })
  }

  get host() {
    return this._host
  }

  private get auth() {
    return this._auth ?? undefined
  }

  get id() {
    return this._id
  }

  private toURL(scheme = 'https') {
    return Peer.toURL(this.host, this.id, scheme, this.auth)
  }

  toJSON() {
    return {
      host: this.host,
      id: this.id,
    }
  }

  static toURL(host: string, id: string, scheme: string, auth?: string) {
    return new URL(`${scheme}://${auth ? `${auth}@` : ''}${host}/${id}`)
  }

}


