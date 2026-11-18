import React from "react";
import { BrowserRouter, HashRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider } from "@/app/AuthContext";
import { WorkspaceProvider } from "@/app/ThemeContext";
import { ErrorBoundary } from "@/components/ui/Primitives";
import { ToastProvider } from "@/components/ui/ToastProvider";
import { AppRoutes } from "@/routes/AppRoutes";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

// Use HashRouter on static GitHub Pages subpath builds so deep links work reliably,
// and BrowserRouter when hosted behind Nginx/Vite.
const isStaticSubpath =
  typeof window !== "undefined" &&
  window.location.hostname.endsWith("github.io");

export const App: React.FC = () => {
  const Router = isStaticSubpath ? HashRouter : BrowserRouter;
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <WorkspaceProvider>
          <ToastProvider>
            <AuthProvider>
              <Router basename={isStaticSubpath ? undefined : import.meta.env.BASE_URL}>
                <AppRoutes />
              </Router>
            </AuthProvider>
          </ToastProvider>
        </WorkspaceProvider>
      </QueryClientProvider>
    </ErrorBoundary>
  );
};

export default App;
