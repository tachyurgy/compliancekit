import { useEffect, useState } from 'react';
import {
  NavLink,
  Outlet,
  Link,
  useNavigate,
  useLocation,
  useSearchParams,
} from 'react-router-dom';
import {
  LayoutDashboard,
  Baby,
  Users,
  FileText,
  FileSignature,
  Settings,
  LogOut,
  Menu,
  X,
  ClipboardCheck,
  FlaskConical,
  ScrollText,
  Scale,
  Zap,
  MessageSquare,
} from 'lucide-react';

import { useSession } from '@/hooks/useSession';
import { useActiveWorkspace } from '@/hooks/useWorkspaces';
import BrandMark from './BrandMark';
import ToastViewport from './Toast';
import WorkspaceSwitcher from './WorkspaceSwitcher';
import DemoBanner from './DemoBanner';
import DemoDeploymentNotice from './DemoDeploymentNotice';

interface NavItem {
  to: string;
  label: string;
  icon: typeof LayoutDashboard;
}

const NAV_ITEMS: NavItem[] = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/chase', label: 'Chase Mode', icon: Zap },
  { to: '/children', label: 'Children', icon: Baby },
  { to: '/staff', label: 'Staff', icon: Users },
  { to: '/operations', label: 'Operations', icon: ClipboardCheck },
  { to: '/inspections', label: 'Inspections', icon: FlaskConical },
  { to: '/messages/new', label: 'Send a message', icon: MessageSquare },
  { to: '/documents', label: 'Documents', icon: FileText },
  { to: '/templates', label: 'Templates', icon: FileSignature },
  { to: '/settings', label: 'Settings', icon: Settings },
  { to: '/settings/audit-log', label: 'Audit Log', icon: ScrollText },
];

export default function Layout(): JSX.Element {
  const [mobileOpen, setMobileOpen] = useState(false);
  const user = useSession((s) => s.user);
  const signOut = useSession((s) => s.signOut);
  const navigate = useNavigate();

  // REQ-038: URL reflects active workspace via ?demo=1. We don't fork the
  // router — one app, one tree — but the query param is the address-bar
  // indicator the operator asked for, and it survives copy/paste of links
  // among friends so the recipient lands in the matching mode.
  const location = useLocation();
  const [params, setParams] = useSearchParams();
  const active = useActiveWorkspace();
  useEffect(() => {
    if (!active) return;
    const hasParam = params.get('demo') === '1';
    if (active.isDemo && !hasParam) {
      const next = new URLSearchParams(params);
      next.set('demo', '1');
      setParams(next, { replace: true });
    } else if (!active.isDemo && hasParam) {
      const next = new URLSearchParams(params);
      next.delete('demo');
      setParams(next, { replace: true });
    }
    // location.pathname is included so we re-check on navigation (the
    // search-params hook doesn't refire on path-only changes).
  }, [active, params, setParams, location.pathname]);

  const handleSignOut = async () => {
    await signOut();
    navigate('/login', { replace: true });
  };

  return (
    <div className="min-h-screen bg-surface-muted">
      <DemoDeploymentNotice />
      <DemoBanner />
      {/* Mobile top bar */}
      <div className="lg:hidden sticky top-0 z-30 flex items-center justify-between px-4 h-14 bg-white border-b border-line-soft">
        <BrandMark to="/dashboard" size="sm" />
        <button
          type="button"
          onClick={() => setMobileOpen((v) => !v)}
          aria-label="Toggle navigation"
          className="p-2 rounded-md text-ink-700 hover:bg-surface-sunken"
        >
          {mobileOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
        </button>
      </div>

      <div className="flex">
        {/* Sidebar */}
        <aside
          className={`${
            mobileOpen ? 'block' : 'hidden'
          } lg:block fixed lg:sticky top-0 lg:top-0 left-0 z-20 lg:z-auto h-screen w-64 bg-white border-r border-line-soft flex-shrink-0`}
        >
          <div className="h-full flex flex-col">
            <div className="hidden lg:flex items-center px-5 h-16 border-b border-line-soft">
              <BrandMark to="/dashboard" size="md" />
            </div>
            <div className="p-3 border-b border-line-soft">
              <WorkspaceSwitcher />
            </div>
            <nav className="flex-1 p-3 space-y-1">
              {NAV_ITEMS.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  onClick={() => setMobileOpen(false)}
                  className={({ isActive }) =>
                    `flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
                      isActive
                        ? 'bg-brand-50 text-brand-700'
                        : 'text-ink-700 hover:bg-surface-sunken'
                    }`
                  }
                >
                  <item.icon className="h-4 w-4" />
                  <span>{item.label}</span>
                </NavLink>
              ))}
            </nav>
            <div className="p-3 border-t border-line-soft space-y-2">
              {/* Inspector Mode (REQ-009): one tap from anywhere in the
                  operator UI. Persistent — the owner wants this to be the
                  single most visible button on the screen. */}
              <Link
                to="/inspector"
                onClick={() => setMobileOpen(false)}
                className="w-full flex items-center gap-3 px-3 py-2 rounded-md text-sm font-semibold bg-slate-900 text-white hover:bg-slate-800"
              >
                <Scale className="h-4 w-4" />
                <span>Open Inspector View</span>
              </Link>
              {user && (
                <div className="px-3 pb-1 pt-2 border-t border-line-soft">
                  <p className="text-xs text-ink-500">Signed in as</p>
                  <p className="text-sm font-medium text-ink-800 truncate">
                    {user.fullName ?? user.email}
                  </p>
                </div>
              )}
              <button
                type="button"
                onClick={handleSignOut}
                className="w-full flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium text-ink-700 hover:bg-surface-sunken"
              >
                <LogOut className="h-4 w-4" />
                <span>Sign out</span>
              </button>
            </div>
          </div>
        </aside>

        {/* Main content */}
        <main className="flex-1 min-w-0">
          <div className="page-container">
            <Outlet />
          </div>
        </main>
      </div>
      <ToastViewport />
    </div>
  );
}
