// Fix for Node.js 25+ global localStorage conflicting with jsdom's localStorage.
// Node 25 ships a built-in localStorage with no clear/setItem/getItem methods
// unless --localstorage-file is provided. Override with a simple in-memory mock
// so jsdom tests work correctly.
const storage: Record<string, string> = {};
const mockStorage: Storage = {
  getItem: (key: string) => storage[key] ?? null,
  setItem: (key: string, value: string) => { storage[key] = value; },
  removeItem: (key: string) => { delete storage[key]; },
  clear: () => { for (const k of Object.keys(storage)) delete storage[k]; },
  key: (index: number) => Object.keys(storage)[index] ?? null,
  get length() { return Object.keys(storage).length; },
};
// Override Node's broken global localStorage with a working mock.
Object.defineProperty(globalThis, 'localStorage', { value: mockStorage, configurable: true, writable: true });
