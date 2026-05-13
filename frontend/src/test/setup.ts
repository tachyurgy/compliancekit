// Vitest setup — loaded once per test process.
//
// @testing-library/jest-dom extends Vitest's expect with DOM matchers
// (toBeInTheDocument, toHaveTextContent, toHaveClass, etc.) so component
// specs read naturally without hand-rolled assertions on .innerHTML.
import '@testing-library/jest-dom/vitest';
import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';

// Unmount any rendered tree between tests so DOM state from one spec
// can't leak into the next.
afterEach(() => {
  cleanup();
});
