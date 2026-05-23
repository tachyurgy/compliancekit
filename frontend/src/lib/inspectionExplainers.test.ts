import { describe, it, expect } from 'vitest';
import {
  getExplainer,
  hasExplainer,
  inspectionExplainers,
  fallbackExplainer,
} from './inspectionExplainers';

describe('inspectionExplainers', () => {
  it('covers at least 10 of the highest-stakes FL items', () => {
    // REQ-040 promised "10-15 most common questions" — the QA pass
    // floor is 10 so we don't slip below the bar.
    expect(Object.keys(inspectionExplainers).length).toBeGreaterThanOrEqual(10);
  });

  it('every curated entry has all three sections', () => {
    for (const [id, exp] of Object.entries(inspectionExplainers)) {
      expect(exp.meaning, `${id} meaning`).toBeTruthy();
      expect(exp.steps, `${id} steps`).toBeInstanceOf(Array);
      expect(exp.steps.length, `${id} steps len`).toBeGreaterThanOrEqual(2);
      expect(exp.source, `${id} source`).toMatch(/F\.S\.|F\.A\.C\.|Handbook/);
    }
  });

  it('hasExplainer returns true for curated items and false for others', () => {
    expect(hasExplainer('fl.01.ratios')).toBe(true);
    expect(hasExplainer('fl.99.nonexistent')).toBe(false);
  });

  it('getExplainer returns fallback for unknown ids', () => {
    expect(getExplainer('fl.99.nonexistent')).toBe(fallbackExplainer);
  });

  it('every curated explainer avoids fear words (CLAUDE.md voice rule)', () => {
    // Voice rule: "operators are already scared of DCF. Don't repeat it."
    // The strict list is short on purpose — we don't want to ban "fire"
    // (fire drills), "emergency", or "violation" outright because those
    // are domain words. We do ban panic-flavored phrasing.
    const banned = ['terrifying', 'scary', 'panic', 'nightmare', 'disaster'];
    for (const [id, exp] of Object.entries(inspectionExplainers)) {
      const blob = (exp.meaning + ' ' + exp.steps.join(' ')).toLowerCase();
      for (const word of banned) {
        expect(blob, `${id} contains banned voice word "${word}"`).not.toContain(
          word,
        );
      }
    }
  });
});
