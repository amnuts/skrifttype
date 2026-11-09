import { describe, it, expect, beforeEach, vi } from 'vitest';
import { OnboardingController, type TourStep } from './onboarding-controller';
import { dataSource } from '../data';

const steps: TourStep[] = [
  { title: 'A', body: 'a' },
  { title: 'B', body: 'b' },
  { title: 'C', body: 'c' },
];

describe('OnboardingController', () => {
  beforeEach(() => localStorage.clear());

  it('advances, goes back, and reports the active step', async () => {
    const host = { requestUpdate: vi.fn() } as any;
    const c = new OnboardingController(host, () => steps, {} as any);
    await c.start();
    expect(c.active).toBe(true);
    expect(c.index).toBe(0);
    await c.next();
    expect(c.index).toBe(1);
    await c.back();
    expect(c.index).toBe(0);
  });

  it('finishing past the last step marks onboarding seen and closes', async () => {
    const host = { requestUpdate: vi.fn() } as any;
    const c = new OnboardingController(host, () => steps, {} as any);
    await c.start();
    await c.next(); await c.next(); // index 2 (last)
    await c.next();                 // past end -> finish
    expect(c.active).toBe(false);
    expect(await dataSource.getPreference('onboarding_seen')).toBe('1');
  });

  it('skip closes and marks seen', async () => {
    const host = { requestUpdate: vi.fn() } as any;
    const c = new OnboardingController(host, () => steps, {} as any);
    await c.start();
    await c.skip();
    expect(c.active).toBe(false);
    expect(await dataSource.getPreference('onboarding_seen')).toBe('1');
  });
});
