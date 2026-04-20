import { useState } from 'react';
import { NavLink, Outlet, Link, useNavigate } from 'react-router-dom';
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
  ShieldCheck,
  ClipboardCheck,
  FlaskConical,
} from 'lucide-react';

import { useSession } from '@/hooks/useSession';
import ToastViewport from './Toast';

interface NavItem {
  to: string;
  label: string;
  icon: typeof LayoutDashboard;
}

const NAV_ITEMS: NavItem[] = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/children', label: 'Children', icon: Baby },
  { to: '/staff', label: 'Staff', icon: Users },
  { to: '/operations', label: 'Operations', icon: ClipboardCheck },
  { to: '/inspections', label: 'Inspections', icon: FlaskConical },
  { to: '/documents', label: 'Documents', icon: FileText },
  { to: '/templates', label: 'Templates', icon: FileSignature },
  { to: '/settings', label: 'Settings', icon: Settings },
];

export default function Layout(): JSX.Element {
  const [mobileOpen, setMobileOpen] = useState(false);
  const user = useSession((s) => s.user);
  const signOut = useSession((s) => s.signOut);
  const navigate = useNavigate();

  const handleSignOut = async () => {
    await signOut();
    navigate('/login', { replace: true });
  };

  return (
    <div className="min-h-screen bg-surface-muted">
      {/* Mobile top bar */}
      <div className="lg:hidden sticky top-0 z-30 flex items-center justify-between px-4 h-14 bg-white border-b border-gray-100">
        <Link to="/dashboard" className="flex items-center gap-2">
          <ShieldCheck className="h-6 w-6 text-brand-600" />
          <span className="font-semibold">ComplianceKit</span>
        </Link>
        <button
          type="button"
          onClick={() => setMobileOpen((v) => !v)}
          aria-label="Toggle navigation"
          className="p-2 rounded-md text-gray-700 hover:bg-gray-100"
        >
          {mobileOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
        </button>
      </div>

      <div className="flex">
        {/* Sidebar */}
        <aside
          className={`${
            mobileOpen ? 'block' : 'hidden'
          } lg:block fixed lg:sticky top-0 lg:top-0 left-0 z-20 lg:z-auto h-screen w-64 bg-white border-r border-gray-100 flex-shrink-0`}
        >
          <div className="h-full flex flex-col">
            <div className="hidden lg:flex items-center gap-2 px-5 h-16 border-b border-gray-100">
              <ShieldCheck className="h-6 w-6 text-brand-600" />
              <span className="font-semibold text-gray-900">ComplianceKit</span>
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
                        : 'text-gray-700 hover:bg-gray-100'
                    }`
                  }
                >
                  <item.icon className="h-4 w-4" />
                  <span>{item.label}</span>
                </NavLink>
              ))}
            </nav>
            <div className="p-3 border-t border-gray-100">
              {user && (
                <div className="px-3 pb-3">
                  <p className="text-xs text-gray-500">Signed in as</p>
                  <p className="text-sm font-medium text-gray-800 truncate">
                    {user.fullName ?? user.email}
                  </p>
                </div>
              )}
              <button
                type="button"
                onClick={handleSignOut}
                className="w-full flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium text-gray-700 hover:bg-gray-100"
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
