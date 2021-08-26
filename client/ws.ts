type iterator = { value: Blob, done: false } | { value: undefined, done: true }
const key = Symbol()

class Client implements AsyncIterator<Blob> {
  static CONNECTING = WebSocket.CONNECTING
  static OPEN = WebSocket.OPEN
  static CLOSING = WebSocket.CLOSING
  static CLOSED = WebSocket.CLOSED
  private readonly _messages = new Array<Blob>()
  private readonly _waiters = new Array<{ resolve: (o: iterator) => void, reject: (err: Error | ErrorEvent) => void }>()
  private readonly _socket

  constructor(
    private readonly _url: URL,
    _?: Symbol,
  ) {
    if (arguments[1] !== key) {
      throw new Error('use static connect method to init WS')
    }
    const url = Object.assign({}, _url)
    url.protocol = 'wss';
    this._socket = new WebSocket(url.toString())
  }

  get readyState() {
    return this._socket.readyState
  }

  close() {
    return this._socket.close()
  }

  get url() {
    return this._url
  }

  set onClose(cb: (ev: Event) => void) {
    this._socket.addEventListener('error', (ev) => {
      cb(ev)
    }, {once: true})
  }

  [Symbol.asyncIterator]() {
    return this;
  }

  async send(to: string | number | bigint, data: Blob) {
    const url = Object.assign({}, this._url)
    url.protocol = 'https'
    url.pathname = to.toString()
    const response = await fetch(url.toString(), {
      method: 'POST',
      body: data,
    })
    if (!response.ok) {
      throw new Error(response.statusText || response.status.toString())
    }
  }

  next() {
    return new Promise<iterator>((resolve, reject) => {
      switch (this.readyState) {
        case WebSocket.CLOSED:
        case WebSocket.CLOSING:
          return reject(new Error('CLOSED'))
      }
      const message = this._messages.shift()
      if (message) {
        return resolve({value: message, done: false})
      }
      this._waiters.push({resolve, reject})
    })
  }

  private _handleMessage(ev: MessageEvent) {
    const awaiter = this._waiters.shift()
    if (awaiter) {
      return awaiter.resolve({value: ev.data, done: false})
    }
    this._messages.push(ev.data)
  }

  private _handleClose(ev: CloseEvent) {
    let waiter = this._waiters.shift()
    while (waiter) {
      waiter.resolve({value: undefined, done: true})
      waiter = this._waiters.shift()
    }
  }

  private _handleError(ev: ErrorEvent) {
    let waiter = this._waiters.shift()
    while (waiter) {
      waiter.reject(ev)
      waiter = this._waiters.shift()
    }
  }

  static connect(host: string, port: number, id: number | string | bigint, auth?: string) {
    const url = new URL(`https://${auth ? `${auth}@` : ''}${host}:${port}/${id.toString()}`)
    return new Promise<Client>((resolve, reject) => {
      const client = new Client(url as URL, key)
      client._socket.addEventListener('error', (ev) => {
        switch (client.readyState) {
          case WebSocket.CONNECTING:
          case WebSocket.OPEN:
            client.close()
        }
        client._handleError(ev as ErrorEvent)
        reject(ev)
      }, {once: true})

      client._socket.addEventListener('open', () => {
        client._socket.addEventListener('close', client._handleClose.bind(client))
        resolve(client)
      }, {once: true})

      client._socket.addEventListener('message', client._handleMessage.bind(client))
    })
  }
}

async function main() {
  const cli = Client.connect('localhost', 4433, 123)
}

main().catch(console.error)