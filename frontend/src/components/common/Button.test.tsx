import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Button from './Button';

describe('<Button>', () => {
  it('renders its children as the accessible label', () => {
    render(<Button>Start trial</Button>);
    expect(screen.getByRole('button', { name: 'Start trial' })).toBeInTheDocument();
  });

  it('fires onClick when activated', async () => {
    const onClick = vi.fn();
    render(<Button onClick={onClick}>Save</Button>);
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it('disables the button while loading and swallows clicks', async () => {
    // The loading prop is the only way an idle button becomes
    // unclickable mid-flight (signup, send-reminder, sign-and-send).
    // If this regresses, a double-click can submit the form twice.
    const onClick = vi.fn();
    render(
      <Button loading onClick={onClick}>
        Sending…
      </Button>,
    );
    const btn = screen.getByRole('button', { name: /sending/i });
    expect(btn).toBeDisabled();
    await userEvent.click(btn);
    expect(onClick).not.toHaveBeenCalled();
  });

  it('keeps explicit `disabled` honored even without loading', () => {
    render(<Button disabled>Closed</Button>);
    expect(screen.getByRole('button')).toBeDisabled();
  });

  it('applies the danger variant for destructive copy', () => {
    // Visual-regression-by-class — at minimum, "Delete"/"Revoke" buttons
    // need to render in the coral palette, never sage primary. Pinning
    // the class keeps drive-by refactors from swapping variants by mistake.
    render(<Button variant="danger">Delete child file</Button>);
    expect(screen.getByRole('button')).toHaveClass(/bg-critical-600/);
  });

  it('respects the fullWidth prop', () => {
    render(<Button fullWidth>Wide</Button>);
    expect(screen.getByRole('button')).toHaveClass('w-full');
  });
});
