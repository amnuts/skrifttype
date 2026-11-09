import type { DataSource } from './data-source';
import { MockDataSource } from './mock-data-source';
import { WailsDataSource } from './wails-data-source';

// Detect whether we're running inside the real Wails desktop webview (vs vite
// dev / Playwright / jsdom).
//
// We can't rely on `window._wails` or `_wails.invoke`: `@wailsio/runtime` sets
// both unconditionally on import, even in a browser. And `_wails.environment`
// (injected by Go) arrives ASYNCHRONOUSLY after the runtime is ready — the
// runtime itself polls for it — so it may be absent at module-eval time even in
// the packaged app, which would wrongly select the mock and make the folder
// picker fall back to a prompt.
//
// The host webview injects its native message channel synchronously before any
// script runs, so probing that is reliable and immediate:
//   - Windows (WebView2): window.chrome.webview.postMessage
//   - macOS (WKWebView):  window.webkit.messageHandlers.external.postMessage
//   - Android:            window.wails.invoke
// `_wails.environment` is kept as a late-positive signal for completeness.
function isWailsRuntime(): boolean {
  const w = globalThis as unknown as {
    chrome?: { webview?: { postMessage?: unknown } };
    webkit?: { messageHandlers?: { external?: { postMessage?: unknown } } };
    wails?: { invoke?: unknown };
    _wails?: { environment?: unknown };
  };
  return (
    typeof w.chrome?.webview?.postMessage !== 'undefined' ||
    typeof w.webkit?.messageHandlers?.external?.postMessage !== 'undefined' ||
    typeof w.wails?.invoke !== 'undefined' ||
    typeof w._wails?.environment !== 'undefined'
  );
}

// SWAP POINT: real bindings in the desktop app, mock everywhere else.
export const dataSource: DataSource = isWailsRuntime() ? new WailsDataSource() : new MockDataSource();
