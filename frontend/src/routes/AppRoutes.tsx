import React, { Suspense, lazy } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "@/app/AuthContext";
import { AppLayout } from "@/components/layout/AppLayout";
import { Skeleton } from "@/components/ui/Primitives";
import {
  ForgotPasswordPage,
  LoginPage,
  RegisterPage,
  ResetPasswordPage,
  SessionExpiredPage,
  UnauthorizedPage,
} from "@/features/auth/AuthPages";

// Route-level code splitting (PRD Section 22.1 & 30.1)
const OverviewPage = lazy(() =>
  import("@/features/overview/OverviewPage").then((m) => ({ default: m.OverviewPage }))
);
const OnboardingPage = lazy(() =>
  import("@/features/onboarding/OnboardingPage").then((m) => ({ default: m.OnboardingPage }))
);
const InfrastructurePage = lazy(() =>
  import("@/features/infrastructure/InfrastructurePage").then((m) => ({ default: m.InfrastructurePage }))
);
const ServersPage = lazy(() =>
  import("@/features/infrastructure/ServersPage").then((m) => ({ default: m.ServersPage }))
);
const ServerDetailPage = lazy(() =>
  import("@/features/infrastructure/ServerDetailPage").then((m) => ({ default: m.ServerDetailPage }))
);
const ServicesPage = lazy(() =>
  import("@/features/services/ServicesPage").then((m) => ({ default: m.ServicesPage }))
);
const MetricsExplorerPage = lazy(() =>
  import("@/features/monitoring/MetricsExplorerPage").then((m) => ({ default: m.MetricsExplorerPage }))
);
const AlertsPage = lazy(() =>
  import("@/features/alerts/AlertsPages").then((m) => ({ default: m.AlertsPage }))
);
const AlertDetailPage = lazy(() =>
  import("@/features/alerts/AlertsPages").then((m) => ({ default: m.AlertDetailPage }))
);
const AlertRulesPage = lazy(() =>
  import("@/features/alerts/AlertsPages").then((m) => ({ default: m.AlertRulesPage }))
);
const NotificationsPage = lazy(() =>
  import("@/features/notifications/NotificationsPage").then((m) => ({ default: m.NotificationsPage }))
);
const ReportsPage = lazy(() =>
  import("@/features/reports/ReportsPage").then((m) => ({ default: m.ReportsPage }))
);
const ActivityLogsPage = lazy(() =>
  import("@/features/audit/ActivityLogsPage").then((m) => ({ default: m.ActivityLogsPage }))
);
const SettingsPage = lazy(() =>
  import("@/features/settings/SettingsPage").then((m) => ({ default: m.SettingsPage }))
);
const ProfilePage = lazy(() =>
  import("@/features/profile/ProfilePage").then((m) => ({ default: m.ProfilePage }))
);

const PageLoader: React.FC = () => (
  <div className="space-y-4 p-4">
    <Skeleton className="h-8 w-56" />
    <Skeleton className="h-48 w-full" />
  </div>
);

const ProtectedRoute: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { isAuthenticated, isLoading } = useAuth();
  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-50 dark:bg-navy-950 p-6">
        <div className="w-full max-w-md space-y-3">
          <Skeleton className="h-10 w-40 mx-auto" />
          <Skeleton className="h-32 w-full" />
        </div>
      </div>
    );
  }
  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
};

export const AppRoutes: React.FC = () => (
  <Suspense fallback={<PageLoader />}>
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/forgot-password" element={<ForgotPasswordPage />} />
      <Route path="/reset-password" element={<ResetPasswordPage />} />
      <Route path="/unauthorized" element={<UnauthorizedPage />} />
      <Route path="/session-expired" element={<SessionExpiredPage />} />

      <Route
        path="/"
        element={
          <ProtectedRoute>
            <AppLayout />
          </ProtectedRoute>
        }
      >
        <Route index element={<OverviewPage />} />
        <Route path="onboarding" element={<OnboardingPage />} />
        <Route path="infrastructure" element={<InfrastructurePage />} />
        <Route path="servers" element={<ServersPage />} />
        <Route path="servers/:id" element={<ServerDetailPage />} />
        <Route path="services" element={<ServicesPage />} />
        <Route path="metrics" element={<MetricsExplorerPage />} />
        <Route path="alerts" element={<AlertsPage />} />
        <Route path="alerts/:fingerprint" element={<AlertDetailPage />} />
        <Route path="alert-rules" element={<AlertRulesPage />} />
        <Route path="notifications" element={<NotificationsPage />} />
        <Route path="reports" element={<ReportsPage />} />
        <Route path="activity-logs" element={<ActivityLogsPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="profile" element={<ProfilePage />} />
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  </Suspense>
);
