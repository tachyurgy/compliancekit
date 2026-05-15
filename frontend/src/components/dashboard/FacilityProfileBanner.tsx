import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowRight, ClipboardList, X } from 'lucide-react';

import { facilityApi, type Facility } from '@/api/facility';

// We don't gate access on facility data anymore — operators land on the
// dashboard right after signing legal. But the compliance score and the
// inspector export both lean on the address, license, and capacity fields.
// This banner is the dashboard's one nudge to come back and fill them in,
// dismissible per browser so we don't nag forever.

const DISMISS_KEY = 'levelbrook-facility-banner-dismissed';

function isProfileIncomplete(f: Facility | null): boolean {
  if (!f) return false;
  // We count the profile as "started" once any of the high-value fields
  // exists. We surface the banner until they all do.
  if (!f.licenseNumber.trim()) return true;
  if (!f.licenseExpiresOn) return true;
  if (!f.address1.trim()) return true;
  if (!f.city.trim()) return true;
  if (!f.postalCode.trim()) return true;
  if (!f.phone.trim()) return true;
  if (f.capacity <= 0) return true;
  return false;
}

export default function FacilityProfileBanner(): JSX.Element | null {
  const [facility, setFacility] = useState<Facility | null>(null);
  const [dismissed, setDismissed] = useState<boolean>(() => {
    try {
      return window.localStorage.getItem(DISMISS_KEY) === '1';
    } catch {
      return false;
    }
  });

  useEffect(() => {
    let cancelled = false;
    facilityApi
      .get()
      .then((f) => {
        if (!cancelled) setFacility(f);
      })
      .catch(() => {
        // Non-fatal — banner just won't render.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function dismiss(): void {
    setDismissed(true);
    try {
      window.localStorage.setItem(DISMISS_KEY, '1');
    } catch {
      // Private mode etc — banner reappears next session, fine.
    }
  }

  if (dismissed) return null;
  if (!isProfileIncomplete(facility)) return null;

  return (
    <div className="mb-6 rounded-lb border border-warm-200 bg-warm-50/60 p-4 flex items-start gap-3">
      <span className="mt-0.5 h-9 w-9 rounded-full bg-white grid place-items-center ring-1 ring-warm-200 shrink-0">
        <ClipboardList className="h-5 w-5 text-warm-700" aria-hidden />
      </span>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-semibold text-ink-900">
          Finish your facility profile when you have a minute.
        </p>
        <p className="mt-0.5 text-sm text-ink-700 leading-relaxed">
          We'll use it to score your compliance and pre-fill your DCF inspection export. About two
          minutes — and you can skip back here any time.
        </p>
        <div className="mt-3 flex items-center gap-3">
          <Link
            to="/onboarding/facility"
            className="inline-flex items-center gap-1.5 text-sm font-semibold text-brand-700 hover:text-brand-800"
          >
            Add facility details
            <ArrowRight className="h-3.5 w-3.5" aria-hidden />
          </Link>
          <button
            type="button"
            onClick={dismiss}
            className="text-xs text-ink-500 hover:text-ink-700"
          >
            Not now
          </button>
        </div>
      </div>
      <button
        type="button"
        onClick={dismiss}
        aria-label="Dismiss"
        className="text-ink-400 hover:text-ink-600"
      >
        <X className="h-4 w-4" />
      </button>
    </div>
  );
}
