import { lazy, Suspense, useEffect } from 'react';
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom';

import Layout from '@/components/common/Layout';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { useSession } from '@/hooks/useSession';

import Landing from '@/pages/Landing';
import MagicLinkRequest from '@/pages/MagicLinkRequest';
import MagicLinkCallback from '@/pages/MagicLinkCallback';
import OnboardingWizard from '@/pages/onboarding/OnboardingWizard';
import Dashboard from '@/pages/Dashboard';
import Children from '@/pages/Children';
import ChildDetail from '@/pages/ChildDetail';
import Staff from '@/pages/Staff';
import StaffDetail from '@/pages/StaffDetail';
import Documents from '@/pages/Documents';
import DocumentDetail from '@/pages/DocumentDetail';
import Inspections from '@/pages/Inspections';
import InspectionDetail from '@/pages/InspectionDetail';
import Operations from '@/pages/Operations';
import Settings from '@/pages/Settings';
import SettingsBilling from '@/pages/SettingsBilling';
import SettingsAuditLog from '@/pages/SettingsAuditLog';
import PortalParent from '@/pages/PortalParent';
import PortalStaff from '@/pages/PortalStaff';

// Owned by the PdfSigner agent.
const DocumentTemplates = lazy(() => import('@/pages/DocumentTemplates'));
const SignDocument = lazy(() => import('@/pages/SignDocument'));

function RequireAuth({ children }: { children: JSX.Element }): JSX.Element {
  const { user, status } = useSession();
  const location = useLocation();
  if (status === 'loading') {
    return (
      <div className="h-screen grid place-items-center">
        <LoadingSpinner label="Loading your workspace" />
      </div>
    );
  }
  if (!user) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }
  return children;
}

function RequireOnboarded({ children }: { children: JSX.Element }): JSX.Element {
  const { user } = useSession();
  if (user && !user.onboardingComplete) {
    return <Navigate to="/onboarding" replace />;
  }
  return children;
}

// Handle the GitHub Pages 404.html redirect trick by rewriting ?p=/foo back into a real path.
function GhPagesRedirectHandler(): null {
  const location = useLocation();
  const navigate = useNavigate();
  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const p = params.get('p');
    if (p) {
      const q = params.get('q');
      const search = q ? `?${q.replace(/~and~/g, '&')}` : '';
      navigate(p.replace(/~and~/g, '&') + search, { replace: true });
    }
  }, [location.search, navigate]);
  return null;
}

export default function App(): JSX.Element {
  const rehydrate = useSession((s) => s.rehydrate);
  useEffect(() => {
    void rehydrate();
  }, [rehydrate]);

  return (
    <>
      <GhPagesRedirectHandler />
      <Suspense
        fallback={
          <div className="h-screen grid place-items-center">
            <LoadingSpinner label="Loading..." />
          </div>
        }
      >
        <Routes>
          {/* Public */}
          <Route path="/" element={<Landing />} />
          <Route path="/login" element={<MagicLinkRequest />} />
          <Route path="/auth/callback/:token" element={<MagicLinkCallback />} />

          {/* Public token-gated portals (parents / staff). No account required. */}
          <Route path="/portal/parent/:token" element={<PortalParent />} />
          <Route path="/portal/staff/:token" element={<PortalStaff />} />

          {/* Public signing flow (token-gated). */}
          <Route path="/sign/:token" element={<SignDocument />} />

          {/* Onboarding — requires auth but NOT onboarded. */}
          <Route
            path="/onboarding/*"
            element={
              <RequireAuth>
                <OnboardingWizard />
              </RequireAuth>
            }
          />

          {/* Authenticated app. */}
          <Route
            element={
              <RequireAuth>
                <RequireOnboarded>
                  <Layout />
                </RequireOnboarded>
              </RequireAuth>
            }
          >
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/children" element={<Children />} />
            <Route path="/children/:id" element={<ChildDetail />} />
            <Route path="/staff" element={<Staff />} />
            <Route path="/staff/:id" element={<StaffDetail />} />
            <Route path="/documents" element={<Documents />} />
            <Route path="/documents/:id" element={<DocumentDetail />} />
            <Route path="/inspections" element={<Inspections />} />
            <Route path="/inspections/:id" element={<InspectionDetail />} />
            <Route path="/operations" element={<Operations />} />
            <Route path="/templates" element={<DocumentTemplates />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="/settings/billing" element={<SettingsBilling />} />
            <Route path="/settings/audit-log" element={<SettingsAuditLog />} />
          </Route>

          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Suspense>
    </>
  );
}
