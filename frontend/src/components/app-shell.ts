import { LitElement, html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { provide } from '@lit/context';
import {
  preferencesContext, libraryContext, activationContext, collectionsContext,
} from '../state/contexts';
import { PreferencesStore } from '../state/preferences-store';
import { LibraryStore } from '../state/library-store';
import { ActivationStore } from '../state/activation-store';
import { CollectionsStore } from '../state/collections-store';
import './chrome/title-bar';
import './chrome/main-toolbar';
import './chrome/status-bar';
import './collections/collections-sidebar';
import './library/library-pane';
import './preview/preview-pane';
import './comparison/comparison-pane';
import './glyphmap/glyph-map';
import './reports/report-launcher';
import './settings/settings-dialog';
import './onboarding/onboarding-tour';
import { OnboardingController, type TourStep, ONBOARDING_SEEN } from '../state/onboarding-controller';
import { dataSource } from '../data';
import type { Pane } from './chrome/pane-tabs';

@customElement('skrift-app')
export class SkriftApp extends LitElement {
  @provide({ context: preferencesContext }) prefs = new PreferencesStore();
  @provide({ context: libraryContext }) library = new LibraryStore();
  @provide({ context: activationContext }) activation = new ActivationStore();
  @provide({ context: collectionsContext }) collections = new CollectionsStore();

  @state() pane: Pane = 'preview';
  @state() settingsOpen = false;
  @state() private scanning = false;
  private offLibraryEvents?: () => void;

  private tourSteps: TourStep[] = [
    {
      title: 'Welcome to skrifttype',
      body: 'Browse and preview your fonts, then activate only the ones you need. Fonts stay in your own folders and are loaded on demand — so you avoid bloating the Windows fonts directory and slowing down your design apps.',
    },
    {
      title: 'Add a font folder',
      body: 'Point skrifttype at the folders where your fonts live. Open Settings and choose "Add folder…" to pick one now — you can add as many as you like.',
      before: (app) => { (app as SkriftApp).settingsOpen = true; },
      // Spotlight the open dialog's card (the host element is 0-height because
      // its ui-dialog child is position:fixed), so query through to .card.
      target: (app) => (app as SkriftApp).shadowRoot!
        .querySelector('settings-dialog')?.shadowRoot
        ?.querySelector('ui-dialog')?.shadowRoot
        ?.querySelector('.card') ?? null,
    },
    {
      title: 'The toolbar',
      body: 'Search your families, filter by classification (Serif, Sans, Mono, Script, Display), and switch the right-hand view between Preview, Glyphs, Compare and Report.',
      before: (app) => { (app as SkriftApp).settingsOpen = false; },
      target: (app) => (app as SkriftApp).shadowRoot!.querySelector('main-toolbar'),
    },
    {
      title: 'Library & Collections',
      body: 'Jump to System fonts, Available fonts or just the Active ones, and group fonts into Collections you can activate together — perfect for per-project font sets.',
      target: (app) => (app as SkriftApp).shadowRoot!.querySelector('collections-sidebar'),
    },
    {
      title: 'Preview, Glyphs, Compare & Report',
      body: 'Preview a font with live OpenType features; explore every Glyph; Compare several side by side; or generate a printable specimen/overview Report.',
      target: (app) => (app as SkriftApp).shadowRoot!.querySelector('.right'),
    },
    {
      title: 'You’re all set',
      body: 'That’s the tour! You can replay it any time from Settings ⚙.',
    },
  ];

  private tour = new OnboardingController(this, () => this.tourSteps, this);

  static styles = css`
    :host{display:grid;grid-template-rows:auto auto 1fr auto;height:100%;min-height:0;background:var(--bg);overflow:hidden}
    /* sidebar (persisted width) | drag handle | library (flex) | drag handle | right pane (persisted width) */
    .body{display:grid;grid-template-columns:var(--left-w,200px) 6px minmax(0,1fr) 6px var(--right-w,420px);min-height:0}
    .col{min-width:0;min-height:0;overflow:hidden;display:flex;flex-direction:column}
    .right{background:var(--panel)}
    .resizer{cursor:col-resize;background:var(--border);touch-action:none;position:relative;transition:background var(--dur) var(--ease)}
    .resizer::after{content:"";position:absolute;inset:0 -3px;}
    .resizer:hover,.resizer.dragging{background:var(--accent)}
  `;

  async connectedCallback() {
    super.connectedCallback();
    const rerender = () => this.requestUpdate();
    [this.prefs, this.library, this.activation, this.collections].forEach((s) => s.subscribe(rerender));

    // A rescan (on launch, or after a folder change) can add, remove or
    // re-group fonts, so reload every store when it finishes.
    this.offLibraryEvents = dataSource.onLibraryEvents({
      scanning: (on) => { this.scanning = on; },
      changed: () => {
        void Promise.all([this.library.refresh(), this.activation.refresh(), this.collections.refresh()]);
      },
    });
    this.scanning = await dataSource.isScanning();
    await Promise.all([this.library.refresh(), this.activation.refresh(), this.collections.refresh()]);

    const seen = await dataSource.getPreference(ONBOARDING_SEEN);
    const roots = await dataSource.listWatchedRoots();
    if (!seen && roots.length === 0) await this.tour.start();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.offLibraryEvents?.();
  }

  @state() private dragging: 'left' | 'right' | null = null;

  private startLeftResize = (e: PointerEvent) => {
    e.preventDefault();
    this.dragging = 'left';
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    const onMove = (ev: PointerEvent) => {
      this.prefs.leftWidth = this.prefs.clampLeftWidth(ev.clientX - 3);
      this.requestUpdate();
    };
    const onUp = () => {
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerup', onUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      this.dragging = null;
      this.prefs.setLeftWidth(this.prefs.leftWidth); // persist final width
    };
    document.addEventListener('pointermove', onMove);
    document.addEventListener('pointerup', onUp);
  };

  private startResize = (e: PointerEvent) => {
    e.preventDefault();
    this.dragging = 'right';
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    const onMove = (ev: PointerEvent) => {
      this.prefs.rightWidth = this.prefs.clampWidth(window.innerWidth - ev.clientX - 3);
      this.requestUpdate();
    };
    const onUp = () => {
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerup', onUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      this.dragging = null;
      this.prefs.setRightWidth(this.prefs.rightWidth); // persist final width
    };
    document.addEventListener('pointermove', onMove);
    document.addEventListener('pointerup', onUp);
  };

  private renderPane() {
    switch (this.pane) {
      case 'compare': return html`<comparison-pane></comparison-pane>`;
      case 'glyphs': return html`<glyph-map></glyph-map>`;
      case 'report': return html`<report-launcher></report-launcher>`;
      default: return html`<preview-pane></preview-pane>`;
    }
  }

  render() {
    return html`
      <title-bar .total=${this.library.fonts.length} .active=${this.activation.activeCount}
        @open-settings=${() => (this.settingsOpen = true)}></title-bar>
      <main-toolbar .pane=${this.pane}
        @select-pane=${(e: CustomEvent<Pane>) => (this.pane = e.detail)}></main-toolbar>
      <div class="body" style="--left-w:${this.prefs.leftWidth}px;--right-w:${this.prefs.rightWidth}px">
        <div class="col"><collections-sidebar></collections-sidebar></div>
        <div class="resizer ${this.dragging === 'left' ? 'dragging' : ''}" role="separator" aria-orientation="vertical"
          title="Drag to resize" @pointerdown=${this.startLeftResize}></div>
        <div class="col"><library-pane></library-pane></div>
        <div class="resizer ${this.dragging === 'right' ? 'dragging' : ''}" role="separator" aria-orientation="vertical"
          title="Drag to resize" @pointerdown=${this.startResize}></div>
        <div class="col right">${this.renderPane()}</div>
      </div>
      <status-bar .loading=${this.library.loading} .scanning=${this.scanning}
        .selected=${this.library.selectedIds.length}></status-bar>
      <settings-dialog ?open=${this.settingsOpen}
        @close=${() => (this.settingsOpen = false)}
        @replay-onboarding=${() => this.tour.start()}></settings-dialog>
      <onboarding-tour ?open=${this.tour.active} .step=${this.tour.step} .index=${this.tour.index}
        .total=${this.tour.total} .rect=${this.tour.rect}
        @next=${() => this.tour.next()} @back=${() => this.tour.back()}
        @skip=${() => this.tour.skip()} @finish=${() => this.tour.next()}></onboarding-tour>
    `;
  }
}
