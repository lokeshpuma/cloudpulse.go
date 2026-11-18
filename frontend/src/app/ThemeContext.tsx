import React, { createContext, useCallback, useContext, useEffect, useState } from "react";
import type { TimeRangeKey } from "@/types/api";

type Theme = "light" | "dark";

interface WorkspaceContextValue {
  theme: Theme;
  toggleTheme: () => void;
  setTheme: (t: Theme) => void;
  timeRange: TimeRangeKey;
  setTimeRange: (r: TimeRangeKey) => void;
  customStart: string;
  customEnd: string;
  setCustomRange: (start: string, end: string) => void;
  refreshIntervalMs: number;
  setRefreshIntervalMs: (ms: number) => void;
  refreshTick: number;
  triggerRefresh: () => void;
  globalSearch: string;
  setGlobalSearch: (s: string) => void;
}

const WorkspaceContext = createContext<WorkspaceContextValue | undefined>(undefined);

const THEME_PREF_KEY = "cloudpulse_ui_theme";

export const WorkspaceProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [theme, setThemeState] = useState<Theme>(() => {
    if (typeof window === "undefined") return "light";
    try {
      const stored = window.localStorage?.getItem?.(THEME_PREF_KEY);
      if (stored === "dark" || stored === "light") return stored;
    } catch {
      // ignore storage errors
    }
    return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  });

  const [timeRange, setTimeRange] = useState<TimeRangeKey>("1h");
  const [customStart, setCustomStart] = useState<string>("");
  const [customEnd, setCustomEnd] = useState<string>("");
  const [refreshIntervalMs, setRefreshIntervalMs] = useState<number>(15_000);
  const [refreshTick, setRefreshTick] = useState<number>(0);
  const [globalSearch, setGlobalSearch] = useState<string>("");

  useEffect(() => {
    const root = document.documentElement;
    if (theme === "dark") {
      root.classList.add("dark");
    } else {
      root.classList.remove("dark");
    }
    try {
      window.localStorage?.setItem?.(THEME_PREF_KEY, theme);
    } catch {
      // ignore storage restrictions
    }
  }, [theme]);

  const setTheme = useCallback((t: Theme) => setThemeState(t), []);
  const toggleTheme = useCallback(() => setThemeState((p) => (p === "dark" ? "light" : "dark")), []);
  const setCustomRange = useCallback((start: string, end: string) => {
    setCustomStart(start);
    setCustomEnd(end);
    setTimeRange("custom");
  }, []);
  const triggerRefresh = useCallback(() => setRefreshTick((t) => t + 1), []);

  return (
    <WorkspaceContext.Provider
      value={{
        theme,
        toggleTheme,
        setTheme,
        timeRange,
        setTimeRange,
        customStart,
        customEnd,
        setCustomRange,
        refreshIntervalMs,
        setRefreshIntervalMs,
        refreshTick,
        triggerRefresh,
        globalSearch,
        setGlobalSearch,
      }}
    >
      {children}
    </WorkspaceContext.Provider>
  );
};

export function useWorkspace() {
  const ctx = useContext(WorkspaceContext);
  if (!ctx) throw new Error("useWorkspace must be used inside WorkspaceProvider");
  return ctx;
}
