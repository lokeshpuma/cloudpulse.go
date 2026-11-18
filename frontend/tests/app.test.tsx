import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider } from "@/app/AuthContext";
import { WorkspaceProvider } from "@/app/ThemeContext";
import { ToastProvider } from "@/components/ui/ToastProvider";
import { EmptyState, ErrorState } from "@/components/ui/Primitives";
import { LoginPage, RegisterPage } from "@/features/auth/AuthPages";
import { OverviewPage } from "@/features/overview/OverviewPage";
import { ServersPage } from "@/features/infrastructure/ServersPage";
import { AlertsPage } from "@/features/alerts/AlertsPages";
import { api, setApiDemoMode } from "@/services/api";

function renderWithProviders(ui: React.ReactElement, initialRoute = "/") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <WorkspaceProvider>
        <ToastProvider>
          <AuthProvider>
            <MemoryRouter initialEntries={[initialRoute]}>{ui}</MemoryRouter>
          </AuthProvider>
        </ToastProvider>
      </WorkspaceProvider>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
  setApiDemoMode(false);
  document.cookie = "cloudpulse_csrf=test-csrf; path=/";
});

describe("CloudPulse Frontend Suite (PRD Section 32.3)", () => {
  it("renders Login form and validates empty submission", async () => {
    vi.spyOn(api, "refresh").mockRejectedValue(new Error("no session"));
    renderWithProviders(<LoginPage />, "/login");

    expect(screen.getByText(/Sign in to CloudPulse/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /^Sign in$/i }));
    expect(await screen.findByText(/Enter a valid email address/i)).toBeInTheDocument();
  });

  it("validates Registration password complexity and confirmation match", async () => {
    vi.spyOn(api, "refresh").mockRejectedValue(new Error("no session"));
    renderWithProviders(<RegisterPage />, "/register");

    await userEvent.type(screen.getByLabelText(/Full name/i), "Jane DevOps");
    await userEvent.type(screen.getByLabelText(/Email address/i), "jane@example.com");
    await userEvent.type(screen.getByLabelText(/Organization name/i), "Acme Ops");
    await userEvent.type(screen.getByLabelText(/^Password$/i), "weak");
    await userEvent.type(screen.getByLabelText(/Confirm password/i), "different");
    await userEvent.click(screen.getByRole("button", { name: /Create account/i }));

    expect(await screen.findByText(/at least 8 characters/i)).toBeInTheDocument();
  });

  it("renders EmptyState and ErrorState with retry callback", async () => {
    const onRetry = vi.fn();
    render(
      <div>
        <EmptyState title="No hosts" description="Register a server." />
        <ErrorState message="Prometheus offline" onRetry={onRetry} />
      </div>
    );
    expect(screen.getByText("No hosts")).toBeInTheDocument();
    expect(screen.getByText("Prometheus offline")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Retry/i }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("renders Dashboard KPIs and server metrics in Demo Mode", async () => {
    setApiDemoMode(true);
    vi.spyOn(api, "refresh").mockRejectedValue(new Error("demo"));
    renderWithProviders(<OverviewPage />, "/");

    expect(await screen.findByText("Total Servers")).toBeInTheDocument();
    expect(await screen.findByText("prod-api-01")).toBeInTheDocument();
    expect(screen.getByText("Resource Usage Timeline")).toBeInTheDocument();
  });

  it("filters servers and supports server creation modal for Admins", async () => {
    setApiDemoMode(true);
    vi.spyOn(api, "refresh").mockResolvedValue({
      access_token: "tok",
      expires_at: new Date(Date.now() + 60000).toISOString(),
      csrf_token: "csrf",
      user: {
        id: "u1",
        organization_id: "o1",
        name: "Admin",
        email: "a@a.com",
        role: "admin",
        is_active: true,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
      organization: {
        id: "o1",
        name: "Org",
        description: "",
        timezone: "UTC",
        display_units: "binary",
        date_format: "YYYY-MM-DD",
        time_format: "24h",
        default_scrape_interval: 15,
        default_monitoring_timeout: 5,
        agent_connection_timeout: 10,
        disk_exclusion_patterns: "/proc",
        default_environment: "production",
        onboarding_completed: true,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    });
    const listSpy = vi.spyOn(api, "listServers").mockResolvedValue({
      items: [
        {
          id: "s1",
          organization_id: "o1",
          name: "alpha-web-01",
          hostname: "alpha.internal",
          environment: "production",
          operating_system: "linux",
          architecture: "amd64",
          description: "",
          server_group: "web",
          scrape_address: "10.0.0.5:9180",
          monitoring_interval: 15,
          agent_version: "1.0.0",
          monitoring_enabled: true,
          tags: [],
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
          status: "UP",
          availability: "UP",
          cpu_percent: 32.5,
          memory_percent: 51.0,
          disk_percent: 40.2,
          is_stale: false,
          data_source: "live_prometheus",
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
      total_pages: 1,
    });

    renderWithProviders(<ServersPage />, "/servers");
    expect(await screen.findByText("alpha-web-01")).toBeInTheDocument();

    const searchInput = screen.getByLabelText(/Search servers/i);
    await userEvent.type(searchInput, "alpha");
    await waitFor(() => expect(listSpy).toHaveBeenCalled());

    await userEvent.click(screen.getByRole("button", { name: /Register Server/i }));
    expect(await screen.findByText(/Register New Server/i)).toBeInTheDocument();
  });

  it("enforces Viewer permission restrictions by hiding Admin/Operator mutation controls", async () => {
    vi.spyOn(api, "refresh").mockResolvedValue({
      access_token: "tok",
      expires_at: new Date(Date.now() + 60000).toISOString(),
      csrf_token: "csrf",
      user: {
        id: "u2",
        organization_id: "o1",
        name: "Viewer User",
        email: "v@a.com",
        role: "viewer",
        is_active: true,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
      organization: {
        id: "o1",
        name: "Org",
        description: "",
        timezone: "UTC",
        display_units: "binary",
        date_format: "YYYY-MM-DD",
        time_format: "24h",
        default_scrape_interval: 15,
        default_monitoring_timeout: 5,
        agent_connection_timeout: 10,
        disk_exclusion_patterns: "/proc",
        default_environment: "production",
        onboarding_completed: true,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    });
    vi.spyOn(api, "listAlerts").mockResolvedValue([
      {
        fingerprint: "fp-1",
        title: "High CPU",
        alert_name: "CPUUsageWarning",
        severity: "warning",
        description: "CPU > 75%",
        server_id: "s1",
        server_name: "prod-01",
        environment: "production",
        state: "active",
        triggered_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        labels: {},
        annotations: {},
        acknowledged: false,
      },
    ]);

    renderWithProviders(<AlertsPage />, "/alerts");
    expect(await screen.findByText("High CPU")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Acknowledge/i })).not.toBeInTheDocument();
  });

  it("allows Operator/Admin to acknowledge an alert via modal", async () => {
    vi.spyOn(api, "refresh").mockResolvedValue({
      access_token: "tok",
      expires_at: new Date(Date.now() + 60000).toISOString(),
      csrf_token: "csrf",
      user: {
        id: "u3",
        organization_id: "o1",
        name: "Ops",
        email: "ops@a.com",
        role: "operator",
        is_active: true,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
      organization: {
        id: "o1",
        name: "Org",
        description: "",
        timezone: "UTC",
        display_units: "binary",
        date_format: "YYYY-MM-DD",
        time_format: "24h",
        default_scrape_interval: 15,
        default_monitoring_timeout: 5,
        agent_connection_timeout: 10,
        disk_exclusion_patterns: "/proc",
        default_environment: "production",
        onboarding_completed: true,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    });
    vi.spyOn(api, "listAlerts").mockResolvedValue([
      {
        fingerprint: "fp-2",
        title: "Disk Warning",
        alert_name: "DiskUsageWarning",
        severity: "warning",
        description: "Disk > 80%",
        server_id: "s1",
        server_name: "prod-01",
        environment: "production",
        state: "active",
        triggered_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        labels: {},
        annotations: {},
        acknowledged: false,
      },
    ]);
    const ackSpy = vi.spyOn(api, "acknowledgeAlert").mockResolvedValue({
      id: "ack-1",
      alert_fingerprint: "fp-2",
      acknowledged_at: new Date().toISOString(),
      note: "Checking disk",
    });

    renderWithProviders(<AlertsPage />, "/alerts");
    const ackBtn = await screen.findByRole("button", { name: /^Acknowledge$/i });
    await userEvent.click(ackBtn);
    const confirmBtns = await screen.findAllByRole("button", { name: /^Acknowledge$/i });
    await userEvent.click(confirmBtns[confirmBtns.length - 1]);
    await waitFor(() => expect(ackSpy).toHaveBeenCalledWith("fp-2", ""));
  });
});
