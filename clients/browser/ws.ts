type iterator = { value: MessageEvent, done: false } | { value: undefined, done: true }
const key = Symbol()

class WS implements AsyncIterator<MessageEvent> {
  static CONNECTING = WebSocket.CONNECTING
  static OPEN = WebSocket.OPEN
  static CLOSING = WebSocket.CLOSING
  static CLOSED = WebSocket.CLOSED
  private readonly _messages = new Array<MessageEvent>()
  private readonly _waiters = new Array<{ resolve: (o: iterator) => void, reject: (err: Error | ErrorEvent) => void }>()
  private readonly _socket

  constructor(
    private readonly _url: string,
    _?: Symbol,
  ) {
    if (arguments[1] !== key) {
      throw new Error('use static connect method to init WS')
    }
    this._socket = new WebSocket(_url)
  }

  get readyState() {
    return this._socket.readyState
  }

  close() {
    return this._socket.close()
  }

  set onClose(cb: (ev: Event) => void) {
    this._socket.addEventListener('error', (ev) => {
      cb(ev)
    }, {once: true})
  }

  [Symbol.asyncIterator]() {
    return this;
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
      return awaiter.resolve({value: ev, done: false})
    }
    this._messages.push(ev)
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

  static connect(url: string) {
    return new Promise<WS>((resolve, reject) => {
      const client = new WS(url, key)
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
