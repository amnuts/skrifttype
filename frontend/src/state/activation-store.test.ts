import { describe, it, expect } from 'vitest';
import { ActivationStore } from './activation-store';

describe('ActivationStore', () => {
  it('toggles manual activation and reflects active count', async () => {
    const s = new ActivationStore();
    await s.refresh();
    const start = s.activeCount;
    // pick an id that is not currently active
    let id = 1; while (s.isActive(id)) id++;
    await s.toggleManual(id);
    expect(s.isActive(id)).toBe(true);
    expect(s.activeCount).toBe(start + 1);
    await s.toggleManual(id);
    expect(s.isActive(id)).toBe(false);
  });
});
