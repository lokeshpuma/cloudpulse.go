import React, { createContext, useCallback, useContext, useEffect, useState } from "react";
import { ENV_DEMO_MODE, readCsrfCookie, setAccessToken, setCsrfToken } from "@/lib/apiClient";
import { DEMO_ORG, DEMO_USER } from "@/lib/demoData";
import { api, setApiDemoMode } from "@/services/api";
import type { AuthPayload, Organization, User } from "@/types/api";

interface AuthContextValue {
  user: User | null;
  organization: Organization | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  isDemoMode: boolean;
  canManageInfra: boolean;
  canOperateAlerts: boolean;
  sessionExpired: boolean;
  enterDemoMode: () => void;
  exitDemoMode: () => void;
  applyAuthPayload: (payload: AuthPayload) => void;
  refreshOrganization: () => Promise<void>;
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<User | null>(null);
  const [organization, setOrganization] = useState<Organization | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isDemoMode, setIsDemoMode] = useState(false);
  const [sessionExpired, setSessionExpired] = useState(false);

  const applyAuthPayload = useCallback((payload: AuthPayload) => {
    setApiDemoMode(false);
    setIsDemoMode(false);
    setSessionExpired(false);
    setAccessToken(payload.access_token);
    setCsrfToken(payload.csrf_token);
    setUser(payload.user);
    setOrganization(payload.organization);
  }, []);

  const enterDemoMode = useCallback(() => {
    setAccessToken(null);
    setCsrfToken(null);
    setApiDemoMode(true);
    setIsDemoMode(true);
    setSessionExpired(false);
    setUser(DEMO_USER);
    setOrganization(DEMO_ORG);
  }, []);

  const exitDemoMode = useCallback(() => {
    setApiDemoMode(false);
    setIsDemoMode(false);
    setUser(null);
    setOrganization(null);
  }, []);

  const refreshOrganization = useCallback(async () => {
    const org = await api.getOrg();
    setOrganization(org);
  }, []);

  const signOut = useCallback(async () => {
    if (!isDemoMode) {
      try {
        await api.logout();
      } catch {
        // Clear local session state even if network is unavailable
      }
    }
    setAccessToken(null);
    setCsrfToken(null);
    setApiDemoMode(false);
    setIsDemoMode(false);
    setUser(null);
    setOrganization(null);
  }, [isDemoMode]);

  useEffect(() => {
    let active = true;
    const bootstrap = async () => {
      if (ENV_DEMO_MODE) {
        if (active) {
          enterDemoMode();
          setIsLoading(false);
        }
        return;
      }
      // Only attempt cookie refresh if a CSRF session cookie exists (avoids noisy 403 on fresh visits)
      if (!readCsrfCookie()) {
        if (active) setIsLoading(false);
        return;
      }
      try {
        // Attempt seamless refresh via HttpOnly refresh cookie + CSRF cookie
        const payload = await api.refresh();
        if (active) {
          applyAuthPayload(payload);
        }
      } catch {
        // Not signed in yet
      } finally {
        if (active) setIsLoading(false);
      }
    };
    void bootstrap();
    return () => {
      active = false;
    };
  }, [applyAuthPayload, enterDemoMode]);

  const role = user?.role;
  const canManageInfra = role === "admin";
  const canOperateAlerts = role === "admin" || role === "operator";

  return (
    <AuthContext.Provider
      value={{
        user,
        organization,
        isLoading,
        isAuthenticated: Boolean(user),
        isDemoMode,
        canManageInfra,
        canOperateAlerts,
        sessionExpired,
        enterDemoMode,
        exitDemoMode,
        applyAuthPayload,
        refreshOrganization,
        signOut,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside AuthProvider");
  return ctx;
}
