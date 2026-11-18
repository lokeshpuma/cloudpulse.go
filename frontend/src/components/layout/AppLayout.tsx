import React, { useState } from "react";
import { Link, NavLink, Outlet, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  Bell,
  BellRing,
  ChevronLeft,
  ChevronRight,
  ClipboardList,
  FileBarChart2,
  Globe,
  LayoutDashboard,
  LineChart,
  LogOut,
  Menu,
  Moon,
  Network,
  RefreshCw,
  Search,
  Server,
  Settings,
  ShieldAlert,
  Sun,
  User as UserIcon,
  X,
} from "lucide-react";
import { useAuth } from "@/app/AuthContext";
import { useWorkspace } from "@/app/ThemeContext";
import { api } from "@/services/api";
import type { TimeRangeKey } from "@/types/api";
import { cn } from "@/utils/format";

const NAV_ITEMS = [
  { to: "/", label: "Overview", icon: LayoutDashboard, end: true },
  { to: "/infrastructure", label: "Infrastructure", icon: Network },
  { to: "/servers", label: "Servers", icon: Server },
  { to: "/services", label: "Services", icon: Globe },
  { to: "/metrics", label: "Metrics Explorer", icon: LineChart },
  { to: "/alerts", label: "Alerts", icon: ShieldAlert },
  { to: "/alert-rules", label: "Alert Rules", icon: BellRing },
  { to: "/notifications", label: "Notifications", icon: Bell },
  { to: "/reports", label: "Reports", icon: FileBarChart2 },
  { to: "/activity-logs", label: "Activity Logs", icon: ClipboardList },
  { to: "/settings", label: "Settings", icon: Settings },
];

const TIME_RANGES: { value: TimeRangeKey; label: string }[] = [
  { value: "15m", label: "Last 15m" },
  { value: "30m", label: "Last 30m" },
  { value: "1h", label: "Last 1h" },
  { value: "6h", label: "Last 6h" },
  { value: "24h", label: "Last 24h" },
  { value: "7d", label: "Last 7d" },
  { value: "30d", label: "Last 30d" },
];

export const AppLayout: React.FC = () => {
  const { user, organization, isDemoMode, exitDemoMode, signOut } = useAuth();
  const {
    theme,
    toggleTheme,
    timeRange,
    setTimeRange,
    refreshIntervalMs,
    setRefreshIntervalMs,
    triggerRefresh,
    globalSearch,
    setGlobalSearch,
  } = useWorkspace();
  const navigate = useNavigate();

  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);

  const { data: unreadData } = useQuery({
    queryKey: ["unread-notifications", isDemoMode],
    queryFn: () => api.listNotifications({ unread_only: true }),
    refetchInterval: refreshIntervalMs > 0 ? refreshIntervalMs : false,
  });

  const unreadCount = unreadData?.total ?? 0;

  const handleSearchSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (globalSearch.trim()) {
      navigate(`/servers?search=${encodeURIComponent(globalSearch.trim())}`);
    }
  };

  const handleSignOut = async () => {
    setProfileOpen(false);
    await signOut();
    navigate("/login");
  };

  const renderSidebarLinks = (isMobile = false) => (
    <nav aria-label="Primary navigation" className="flex-1 space-y-1 px-2.5 py-4 overflow-y-auto">
      {NAV_ITEMS.map(({ to, label, icon: Icon, end }) => (
        <NavLink
          key={to}
          to={to}
          end={end}
          onClick={() => isMobile && setMobileOpen(false)}
          title={!isMobile && collapsed ? label : undefined}
          className={({ isActive }) =>
            cn(
              "group flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition-colors",
              isActive
                ? "bg-brand-600/20 text-sky-300 border border-sky-500/30"
                : "text-slate-300 hover:bg-navy-800 hover:text-white border border-transparent",
              !isMobile && collapsed && "justify-center px-2"
            )
          }
        >
          <Icon className="h-4.5 w-4.5 shrink-0" aria-hidden="true" />
          {(isMobile || !collapsed) && <span className="truncate">{label}</span>}
        </NavLink>
      ))}
    </nav>
  );

  return (
    <div className="min-h-screen flex bg-slate-50 dark:bg-navy-950 text-slate-900 dark:text-slate-100">
      {/* Desktop Dark Navy Sidebar */}
      <aside
        className={cn(
          "hidden lg:flex flex-col bg-navy-900 text-slate-100 border-r border-slate-800 transition-all duration-200 shrink-0",
          collapsed ? "w-16" : "w-60"
        )}
      >
        <div className="h-14 flex items-center justify-between px-3.5 border-b border-slate-800">
          <Link to="/" className="flex items-center gap-2.5 min-w-0">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-brand-600 text-white shrink-0">
              <Activity className="h-5 w-5" aria-hidden="true" />
            </div>
            {!collapsed && (
              <div className="truncate">
                <span className="text-sm font-bold tracking-tight text-white">CloudPulse</span>
                <p className="text-[10px] text-slate-400 truncate">{organization?.name ?? "Monitoring"}</p>
              </div>
            )}
          </Link>
          <button
            type="button"
            onClick={() => setCollapsed((c) => !c)}
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            className="rounded-md p-1 text-slate-400 hover:bg-navy-800 hover:text-white"
          >
            {collapsed ? <ChevronRight className="h-4 w-4" /> : <ChevronLeft className="h-4 w-4" />}
          </button>
        </div>

        {renderSidebarLinks(false)}

        {!collapsed && (
          <div className="border-t border-slate-800 p-3 text-xs text-slate-400">
            <div className="flex items-center justify-between">
              <span>Role</span>
              <span className="rounded bg-navy-800 px-2 py-0.5 font-medium capitalize text-sky-300">
                {user?.role}
              </span>
            </div>
          </div>
        )}
      </aside>

      {/* Mobile Drawer */}
      {mobileOpen && (
        <div className="fixed inset-0 z-50 flex lg:hidden" role="dialog" aria-modal="true" aria-label="Navigation menu">
          <div className="fixed inset-0 bg-slate-950/60" onClick={() => setMobileOpen(false)} />
          <div className="relative flex w-64 max-w-[80vw] flex-col bg-navy-900 text-slate-100 shadow-xl">
            <div className="h-14 flex items-center justify-between px-4 border-b border-slate-800">
              <div className="flex items-center gap-2">
                <Activity className="h-5 w-5 text-sky-400" />
                <span className="font-bold text-white">CloudPulse</span>
              </div>
              <button
                type="button"
                onClick={() => setMobileOpen(false)}
                aria-label="Close navigation menu"
                className="rounded-md p-1 text-slate-400 hover:text-white"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            {renderSidebarLinks(true)}
          </div>
        </div>
      )}

      {/* Main Content Column */}
      <div className="flex-1 flex flex-col min-w-0">
        {isDemoMode && (
          <div
            role="status"
            className="bg-amber-500/15 border-b border-amber-500/40 px-4 py-1.5 text-xs font-medium text-amber-900 dark:text-amber-200 flex flex-wrap items-center justify-between gap-2"
          >
            <span>
              <strong>DEMO MODE ACTIVE:</strong> Displaying predictable fixture data (not live Prometheus metrics). Demo data is never mixed with live infrastructure.
            </span>
            <button
              type="button"
              onClick={() => {
                exitDemoMode();
                navigate("/login");
              }}
              className="underline hover:text-amber-700 dark:hover:text-white"
            >
              Exit Demo Mode
            </button>
          </div>
        )}

        {/* Global Header */}
        <header className="sticky top-0 z-30 h-14 border-b border-slate-200 dark:border-slate-800 bg-white/95 dark:bg-navy-900/95 backdrop-blur-xs px-4 flex items-center justify-between gap-3">
          <div className="flex items-center gap-3 flex-1 min-w-0">
            <button
              type="button"
              onClick={() => setMobileOpen(true)}
              aria-label="Open navigation menu"
              className="lg:hidden rounded-lg p-1.5 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-navy-800"
            >
              <Menu className="h-5 w-5" />
            </button>

            <form onSubmit={handleSearchSubmit} className="relative flex-1 max-w-md hidden sm:block">
              <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-slate-400" />
              <input
                type="search"
                value={globalSearch}
                onChange={(e) => setGlobalSearch(e.target.value)}
                placeholder="Search servers, hostnames, groups..."
                aria-label="Search infrastructure"
                className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-navy-800 pl-9 pr-3 py-1.5 text-sm text-slate-900 dark:text-slate-100 placeholder:text-slate-400 focus:border-brand-500 focus:bg-white dark:focus:bg-navy-800 focus:outline-none"
              />
            </form>
          </div>

          <div className="flex items-center gap-2">
            {/* Time Range Selector */}
            <select
              aria-label="Time range"
              value={timeRange === "custom" ? "1h" : timeRange}
              onChange={(e) => setTimeRange(e.target.value as TimeRangeKey)}
              className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2.5 py-1.5 text-xs font-medium text-slate-700 dark:text-slate-200"
            >
              {TIME_RANGES.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label}
                </option>
              ))}
            </select>

            {/* Refresh Controls */}
            <div className="hidden md:flex items-center rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800">
              <button
                type="button"
                onClick={triggerRefresh}
                aria-label="Refresh metrics now"
                title="Refresh metrics now"
                className="px-2 py-1.5 text-slate-600 dark:text-slate-300 hover:text-brand-600"
              >
                <RefreshCw className="h-3.5 w-3.5" />
              </button>
              <select
                aria-label="Auto refresh interval"
                value={refreshIntervalMs}
                onChange={(e) => setRefreshIntervalMs(Number(e.target.value))}
                className="border-l border-slate-200 dark:border-slate-700 bg-transparent px-2 py-1.5 text-xs text-slate-600 dark:text-slate-300 focus:outline-none"
              >
                <option value={15000}>15s</option>
                <option value={30000}>30s</option>
                <option value={60000}>1m</option>
                <option value={0}>Off</option>
              </select>
            </div>

            {/* Notification Bell */}
            <Link
              to="/notifications"
              aria-label={`Notifications (${unreadCount} unread)`}
              className="relative rounded-lg p-2 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-navy-800"
            >
              <Bell className="h-4.5 w-4.5" />
              {unreadCount > 0 && (
                <span className="absolute -top-0.5 -right-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-rose-600 px-1 text-[10px] font-bold text-white">
                  {unreadCount > 9 ? "9+" : unreadCount}
                </span>
              )}
            </Link>

            {/* Theme Toggle */}
            <button
              type="button"
              onClick={toggleTheme}
              aria-label={`Switch to ${theme === "dark" ? "light" : "dark"} theme`}
              className="rounded-lg p-2 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-navy-800"
            >
              {theme === "dark" ? <Sun className="h-4.5 w-4.5" /> : <Moon className="h-4.5 w-4.5" />}
            </button>

            {/* User Profile Menu */}
            <div className="relative">
              <button
                type="button"
                onClick={() => setProfileOpen((o) => !o)}
                aria-expanded={profileOpen}
                aria-haspopup="menu"
                className="flex items-center gap-2 rounded-lg border border-slate-200 dark:border-slate-700 px-2.5 py-1.5 text-xs font-medium hover:bg-slate-50 dark:hover:bg-navy-800"
              >
                <span className="flex h-6 w-6 items-center justify-center rounded-full bg-brand-600 text-white text-xs font-semibold">
                  {(user?.name ?? "U").slice(0, 1).toUpperCase()}
                </span>
                <span className="hidden md:inline max-w-[120px] truncate">{user?.name}</span>
              </button>

              {profileOpen && (
                <div
                  role="menu"
                  className="absolute right-0 mt-2 w-56 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-900 py-1.5 shadow-xl z-40"
                >
                  <div className="border-b border-slate-100 dark:border-slate-800 px-3.5 py-2">
                    <p className="text-xs font-semibold text-slate-900 dark:text-white truncate">{user?.name}</p>
                    <p className="text-[11px] text-slate-500 truncate">{user?.email}</p>
                  </div>
                  <Link
                    to="/profile"
                    role="menuitem"
                    onClick={() => setProfileOpen(false)}
                    className="flex items-center gap-2 px-3.5 py-2 text-xs text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-navy-800"
                  >
                    <UserIcon className="h-3.5 w-3.5" /> My Profile
                  </Link>
                  <Link
                    to="/settings"
                    role="menuitem"
                    onClick={() => setProfileOpen(false)}
                    className="flex items-center gap-2 px-3.5 py-2 text-xs text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-navy-800"
                  >
                    <Settings className="h-3.5 w-3.5" /> Account Settings
                  </Link>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      toggleTheme();
                      setProfileOpen(false);
                    }}
                    className="w-full flex items-center gap-2 px-3.5 py-2 text-xs text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-navy-800"
                  >
                    {theme === "dark" ? <Sun className="h-3.5 w-3.5" /> : <Moon className="h-3.5 w-3.5" />}
                    Theme: {theme === "dark" ? "Dark" : "Light"}
                  </button>
                  <div className="my-1 border-t border-slate-100 dark:border-slate-800" />
                  <button
                    type="button"
                    role="menuitem"
                    onClick={handleSignOut}
                    className="w-full flex items-center gap-2 px-3.5 py-2 text-xs text-rose-600 dark:text-rose-400 hover:bg-rose-50 dark:hover:bg-rose-950/40"
                  >
                    <LogOut className="h-3.5 w-3.5" /> Sign Out
                  </button>
                </div>
              )}
            </div>
          </div>
        </header>

        {/* Page Outlet */}
        <main className="flex-1 p-4 md:p-6 max-w-[1600px] w-full mx-auto">
          <Outlet />
        </main>
      </div>
    </div>
  );
};
