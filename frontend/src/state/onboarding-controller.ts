import type { ReactiveControllerHost } from 'lit';
import { dataSource } from '../data';

export interface TourStep {
  title: string;
  body: string;
  /** Element to spotlight; omit for a centered card with no cutout. */
  target?: (app: HTMLElement) => Element | null;
  /** Set app state for this step (open settings, switch pane). */
  before?: (app: HTMLElement) => void | Promise<void>;
}

export const ONBOARDING_SEEN = 'onboarding_seen';

/**
 * Drives the onboarding tour: which step is active and the target rect to
 * spotlight. Presentational rendering lives in <onboarding-tour>; app state
 * changes (open settings, switch pane) are expressed via each step's before().
 */
export class OnboardingController {
  active = false;
  index = 0;
  rect: DOMRect | null = null;

  constructor(
    private host: ReactiveControllerHost,
    private getSteps: () => TourStep[],
    private app: HTMLElement,
  ) {
    if (typeof window !== 'undefined') {
      window.addEventListener('resize', this.onResize);
    }
  }

  private onResize = () => { if (this.active) void this.measure(); };

  get step(): TourStep | undefined { return this.getSteps()[this.index]; }
  get total(): number { return this.getSteps().length; }

  async start() {
    this.active = true;
    this.index = 0;
    await this.enter();
  }

  async next() {
    if (this.index >= this.total - 1) { await this.finish(); return; }
    this.index += 1;
    await this.enter();
  }

  async back() {
    if (this.index === 0) return;
    this.index -= 1;
    await this.enter();
  }

  async skip() { await this.finish(); }

  private async finish() {
    this.active = false;
    this.rect = null;
    this.host.requestUpdate();
    await dataSource.setPreference(ONBOARDING_SEEN, '1');
  }

  /** Run the step's before() hook, wait for layout, then measure the target. */
  private async enter() {
    const step = this.step;
    if (step?.before) await step.before(this.app);
    // Let Lit render the state change before measuring.
    const anyHost = this.host as unknown as { updateComplete?: Promise<unknown> };
    if (anyHost.updateComplete) await anyHost.updateComplete;
    await new Promise((r) => requestAnimationFrame(() => r(null)));
    await this.measure();
  }

  private async measure() {
    const t = this.step?.target?.(this.app) ?? null;
    this.rect = t ? t.getBoundingClientRect() : null;
    this.host.requestUpdate();
  }
}
