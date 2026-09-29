// Test double for the Durable Object runtime: hibernatable sockets, tags,
// auto-response timestamps and a single alarm, driven by the test's clock.
// Only test files import it, so the Worker bundle never includes it.

export class FakeSocket {
  readyState = 1;
  readonly sent: string[] = [];
  closed: { code: number; reason: string } | null = null;
  tags: string[] = [];
  autoResponseAt: Date | null = null;
  private attachment: unknown = null;

  send(message: string): void {
    if (this.readyState !== 1) throw new Error("socket is not open");
    this.sent.push(message);
  }

  close(code: number, reason: string): void {
    if (this.readyState === 3) return;
    this.readyState = 3;
    this.closed = { code, reason };
  }

  serializeAttachment(value: unknown): void {
    this.attachment = structuredClone(value);
  }

  deserializeAttachment(): unknown {
    return structuredClone(this.attachment);
  }

  frames(): Array<Record<string, unknown>> {
    return this.sent.flatMap((message) => {
      try {
        return [JSON.parse(message) as Record<string, unknown>];
      } catch {
        return [];
      }
    });
  }
}

export class FakeState {
  readonly sockets: FakeSocket[] = [];
  readonly values = new Map<string, unknown>();
  alarmAt: number | null = null;

  readonly storage = {
    get: async <T>(key: string): Promise<T | undefined> => this.values.get(key) as T | undefined,
    put: async (key: string, value: unknown): Promise<void> => { this.values.set(key, value); },
    getAlarm: async (): Promise<number | null> => this.alarmAt,
    setAlarm: async (at: number): Promise<void> => { this.alarmAt = at; },
    deleteAlarm: async (): Promise<void> => { this.alarmAt = null; },
  };

  acceptWebSocket(socket: FakeSocket, tags: string[]): void {
    socket.tags = tags;
    this.sockets.push(socket);
  }

  getWebSockets(tag?: string): FakeSocket[] {
    return this.sockets.filter((socket) => !tag || socket.tags.includes(tag));
  }

  setWebSocketAutoResponse(): void {}

  getWebSocketAutoResponseTimestamp(socket: FakeSocket): Date | null {
    return socket.autoResponseAt;
  }
}

export class FakeWebSocketPair {
  0 = new FakeSocket();
  1 = new FakeSocket();
}

export class FakeRequestResponsePair {
  constructor(readonly request: string, readonly response: string) {}
}

// Node's Response refuses status 101; the runtime's carries the client socket.
export function upgradeCapableResponse(Base: typeof Response): typeof Response {
  return class extends Base {
    override webSocket: WebSocket | null = null;
    constructor(body?: BodyInit | null, init?: ResponseInit & { webSocket?: WebSocket | null }) {
      if (init?.status === 101) {
        super(body, { ...init, status: 200 });
        Object.defineProperty(this, "status", { value: 101 });
        this.webSocket = init.webSocket ?? null;
        return;
      }
      super(body, init);
    }
  } as typeof Response;
}
