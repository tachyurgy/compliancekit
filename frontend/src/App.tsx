import { lazy, Suspense, useEffect } from 'react';
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom';

import Layout from '@/components/common/Layout';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { useSession } from '@/hooks/useSession';

import Landing from '@/pages/Landing';
import Login from '@/pages/Login';
import Signup from '@/pages/Signup';
import ForgotPassword from '@/pages/ForgotPassword';
import ResetPassword from '@/pages/ResetPassword';
import OnboardingWizard from '@/pages/onboarding/OnboardingWizard';
import FacilityWizard from '@/pages/onboarding/FacilityWizard';
import OnboardingCheckout from '@/pages/onboarding/OnboardingCheckout';
import OnboardingLegal from '@/pages/onboarding/OnboardingLegal';
import Dashboard from '@/pages/Dashboard';
import Children from '@/pages/Children';
import ChildDetail from '@/pages/ChildDetail';
import Staff from '@/pages/Staff';
import StaffDetail from '@/pages/StaffDetail';
import Chase from '@/pages/Chase';
import Documents from '@/pages/Documents';
import DocumentDetail from '@/pages/DocumentDetail';
import Inspections from '@/pages/Inspections';
import InspectionDetail from '@/pages/InspectionDetail';
import Operations from '@/pages/Operations';
import Settings from '@/pages/Settings';
import SettingsBilling from '@/pages/SettingsBilling';
import SettingsAuditLog from '@/pages/SettingsAuditLog';
import SettingsSecurity from '@/pages/SettingsSecurity';
import SettingsKiosk from '@/pages/SettingsKiosk';
import Kiosk from '@/pages/Kiosk';
import DrillRoster from '@/pages/DrillRoster';
import PortalParent from '@/pages/PortalParent';
import PortalStaff from '@/pages/PortalStaff';
import PortalUpload from '@/pages/PortalUpload';

// Owned by the PdfSigner agent.
const DocumentTemplates = lazy(() => import('@/pages/DocumentTemplates'));
const SignDocument = lazy(() => import('@/pages/SignDocument'));

// Inspector Mode (REQ-009) — separate route tree, separate visual identity.
const InspectorView = lazy(() => import('@/pages/inspector/InspectorView'));

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
          <Route path="/login" element={<Login />} />
          <Route path="/signup" element={<Signup />} />
          <Route path="/forgot-password" element={<ForgotPassword />} />
          <Route path="/reset-password/:token" element={<ResetPassword />} />

          {/* REQ-016: kiosk + drill-roster. Both render outside the operator
              Layout — kiosk is facility-scoped (the iPad pairs with a 4h
              session token in localStorage), and the drill-roster view is a
              full-bleed glanceable page used during an evacuation. */}
          <Route path="/kiosk" element={<Kiosk />} />
          <Route path="/drill-roster" element={<DrillRoster />} />

          {/* Public token-gated portals (parents / staff). No account required. */}
          {/* /u/:token is the unified Levelbrook upload portal (REQ-005/006/008). */}
          {/* The /portal/* routes stay for back-compat with the v1 magic-link emails. */}
          <Route path="/u/:token" element={<PortalUpload />} />
          <Route path="/portal/parent/:token" element={<PortalParent />} />
          <Route path="/portal/staff/:token" element={<PortalStaff />} />

          {/* Public signing flow (token-gated). */}
          <Route path="/sign/:token" element={<SignDocument />} />

          {/* Onboarding — REQ-002 surfaces (checkout, legal) live above the
              wizard so a brand-new operator can pay + sign before they hit
              the REQ-003 FL facility wizard. Legacy /onboarding/* below
              still works for non-FL accounts until we've migrated them. */}
          <Route
            path="/onboarding/checkout"
            element={
              <RequireAuth>
                <OnboardingCheckout />
              </RequireAuth>
            }
          />
          <Route
            path="/onboarding/legal"
            element={
              <RequireAuth>
                <OnboardingLegal />
              </RequireAuth>
            }
          />
          <Route
            path="/onboarding/facility"
            element={
              <RequireAuth>
                <FacilityWizard />
              </RequireAuth>
            }
          />
          <Route
            path="/onboarding/*"
            element={
              <RequireAuth>
                <OnboardingWizard />
              </RequireAuth>
            }
          />

          {/* Inspector Mode — authenticated, but renders OUTSIDE the operator
              Layout so the navy palette and dropped chrome take over the
              whole page. The DCF Licensing Counselor sees only this. */}
          <Route
            path="/inspector"
            element={
              <RequireAuth>
                <RequireOnboarded>
                  <InspectorView />
                </RequireOnboarded>
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
            <Route path="/chase" element={<Chase />} />
            <Route path="/documents" element={<Documents />} />
            <Route path="/documents/:id" element={<DocumentDetail />} />
            <Route path="/inspections" element={<Inspections />} />
            <Route path="/inspections/:id" element={<InspectionDetail />} />
            <Route path="/operations" element={<Operations />} />
            <Route path="/templates" element={<DocumentTemplates />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="/settings/billing" element={<SettingsBilling />} />
            <Route path="/settings/audit-log" element={<SettingsAuditLog />} />
            <Route path="/settings/security" element={<SettingsSecurity />} />
            <Route path="/settings/kiosk" element={<SettingsKiosk />} />
          </Route>

          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Suspense>
    </>
  );
}
