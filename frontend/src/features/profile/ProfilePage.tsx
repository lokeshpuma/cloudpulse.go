import React from "react";
import { useAuth } from "@/app/AuthContext";
import { Card, StatusBadge } from "@/components/ui/Primitives";
import { formatTimestamp } from "@/utils/format";

export const ProfilePage: React.FC = () => {
  const { user, organization, isDemoMode } = useAuth();
  const tz = organization?.timezone ?? "UTC";

  return (
    <div className="max-w-2xl space-y-6">
      <div>
        <h1 className="text-xl font-bold text-slate-900 dark:text-white">My Profile</h1>
        <p className="text-xs text-slate-500">Account identity, role permissions, and session posture</p>
      </div>

      <Card title="Account Details">
        <dl className="grid sm:grid-cols-2 gap-4 text-xs">
          <div><dt className="text-slate-500">Full Name</dt><dd className="mt-1 font-semibold">{user?.name}</dd></div>
          <div><dt className="text-slate-500">Email</dt><dd className="mt-1 font-mono">{user?.email}</dd></div>
          <div><dt className="text-slate-500">Role</dt><dd className="mt-1 capitalize font-semibold">{user?.role}</dd></div>
          <div><dt className="text-slate-500">Status</dt><dd className="mt-1"><StatusBadge status={user?.is_active ? "UP" : "DOWN"} /></dd></div>
          <div><dt className="text-slate-500">Organization</dt><dd className="mt-1 font-medium">{organization?.name}</dd></div>
          <div><dt className="text-slate-500">Member Since</dt><dd className="mt-1">{formatTimestamp(user?.created_at, tz)}</dd></div>
        </dl>
      </Card>

      <Card title="Session Security">
        <p className="text-xs text-slate-600 dark:text-slate-300">
          {isDemoMode
            ? "Currently exploring CloudPulse in explicit Demo Mode (no backend session credentials are active)."
            : "Your short-lived access token is stored strictly in browser memory (never localStorage), and session renewal uses an HttpOnly SameSite cookie with CSRF protection."}
        </p>
      </Card>
    </div>
  );
};
