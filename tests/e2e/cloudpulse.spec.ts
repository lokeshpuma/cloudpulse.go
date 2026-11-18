import { test, expect } from "@playwright/test";
import path from "path";

const SCREENSHOT_DIR = path.resolve(__dirname, "../../docs/screenshots");
const CAPTURE = process.env.CAPTURE_SCREENSHOTS === "true";

async function maybeShot(page: import("@playwright/test").Page, name: string) {
  if (!CAPTURE) return;
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.screenshot({ path: path.join(SCREENSHOT_DIR, name), fullPage: false });
}

test.describe("CloudPulse Full E2E Monitoring Journey (PRD Section 32.4)", () => {
  test("complete onboarding, multi-server live Prometheus monitoring, metrics explorer, alerts & recovery", async ({ page, request }) => {
    const uniqueEmail = `e2e-${Date.now()}-${Math.floor(Math.random() * 1000)}@cloudpulse.local`;
    const consoleErrors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") consoleErrors.push(msg.text());
    });

    // 1. Open CloudPulse Login
    await page.goto("/login");
    await expect(page.getByText(/Sign in to CloudPulse/i)).toBeVisible();
    await maybeShot(page, "01-login.png");

    // 2. Register new organization & admin
    await page.goto("/register");
    await page.getByLabel(/Full name/i).fill("CloudPulse Admin");
    await page.getByLabel(/Email address/i).fill(uniqueEmail);
    await page.getByLabel(/Organization name/i).fill("CloudPulse Production Ops");
    await page.getByLabel(/^Password$/i).fill("CloudPulse123");
    await page.getByLabel(/Confirm password/i).fill("CloudPulse123");
    await page.getByRole("button", { name: /Create account/i }).click();

    // 3. Complete 6-step Onboarding Wizard & register first live Go agent (agent-node-1:9180)
    await expect(page).toHaveURL(/\/onboarding/);
    await page.getByRole("button", { name: /Continue/i }).click();
    await page.getByRole("button", { name: /Register First Server/i }).click();
    await page.getByRole("button", { name: /Register Server/i }).click();
    await page.getByRole("button", { name: /Verify Connection/i }).click();
    await page.getByRole("button", { name: /Continue to Finish|Connection Verified/i }).click();
    await page.getByRole("button", { name: /Open Main Dashboard/i }).click();

    // 4. Register second live Go agent (agent-node-2:9180)
    await page.goto("/servers");
    await page.getByRole("button", { name: /Register Server/i }).click();
    await page.locator("#srv-name").fill("db-server-01");
    await page.locator("#srv-host").fill("db-server-01.internal");
    await page.locator("#srv-addr").fill("agent-node-2:9180");
    await page.locator("#srv-env").selectOption("staging");
    await page.locator("#srv-group").fill("database");
    await page.getByRole("button", { name: /^Register Server$/i }).last().click();
    await expect(page.getByRole("link", { name: "db-server-01", exact: true })).toBeVisible();

    // 5. Wait for Prometheus file_sd to scrape both live agents & display real metrics on Main Dashboard
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Infrastructure Overview" })).toBeVisible();
    await expect(page.getByRole("cell", { name: "app-server-01", exact: true })).toBeVisible();
    await expect(page.getByRole("cell", { name: "db-server-01", exact: true })).toBeVisible();

    // Poll manual refresh until both hosts show Healthy in the Server Status table (live Prometheus scrape)
    const tableHealthy = page.locator("tbody").getByText("Healthy", { exact: true });
    for (let i = 0; i < 15; i++) {
      const healthyBadges = await tableHealthy.count();
      if (healthyBadges >= 2) break;
      await page.waitForTimeout(2000);
      await page.getByLabel("Refresh metrics now").click();
    }
    await expect(tableHealthy.first()).toBeVisible();
    await page.waitForTimeout(500);
    await maybeShot(page, "02-dashboard-live.png");

    // 6. Inspect Server Detail page (Sections A-H)
    await page.getByText("app-server-01").first().click();
    await expect(page.getByText("Section A: Server Information")).toBeVisible();
    await expect(page.getByText("Section C: CPU Analytics")).toBeVisible();
    await expect(page.getByText("Section E: Disk Analytics")).toBeVisible();
    await expect(page.getByText("Section F: Network Analytics")).toBeVisible();
    await page.waitForTimeout(500);
    await maybeShot(page, "03-server-detail.png");

    // 7. Metrics Explorer live query
    await page.goto("/metrics");
    await page.getByRole("button", { name: /Execute Query/i }).click();
    await expect(page.getByText(/Executed:/i)).toBeVisible();
    await page.waitForTimeout(500);
    await maybeShot(page, "04-metrics-explorer.png");

    // 8. Trigger controlled test alert in Alertmanager, verify in Alerts page, and Acknowledge it
    const now = new Date();
    const ends = new Date(now.getTime() + 5 * 60_000);
    await request.post("http://127.0.0.1:9093/api/v2/alerts", {
      data: [
        {
          labels: {
            alertname: "ControlledE2ECPUTest",
            severity: "warning",
            server_name: "app-server-01",
            environment: "production",
          },
          annotations: {
            summary: "Controlled CPU threshold test on app-server-01",
            description: "Synthetic Alertmanager alert triggered during E2E verification.",
          },
          startsAt: now.toISOString(),
          endsAt: ends.toISOString(),
        },
      ],
    });

    await page.goto("/alerts");
    await expect(page.getByText("Controlled CPU threshold test on app-server-01")).toBeVisible();
    await maybeShot(page, "05-alerts.png");

    // Acknowledge the controlled alert
    const row = page.locator("tr", { hasText: "Controlled CPU threshold test on app-server-01" });
    await row.getByRole("button", { name: /^Acknowledge$/i }).click();
    await page.locator("textarea").fill("Verified during Playwright E2E run");
    await page.getByRole("button", { name: /^Acknowledge$/i }).last().click();
    await expect(row.getByText(/Ack by/i)).toBeVisible();

    // Verify Alert Rules page
    await page.goto("/alert-rules");
    await expect(page.getByText("CPU Usage Warning")).toBeVisible();
    await expect(page.getByText("Server Down")).toBeVisible();

    // 9. Verify Reports & Settings -> Integrations (Postgres, Prometheus, Alertmanager, Grafana)
    await page.goto("/reports");
    await expect(page.getByText("Reports & Analytics")).toBeVisible();

    await page.goto("/settings");
    await page.getByRole("button", { name: /Integrations/i }).click();
    await expect(page.getByRole("heading", { name: "Prometheus TSDB" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Grafana Visualization" })).toBeVisible();
    await page.waitForTimeout(500);
    await maybeShot(page, "06-integrations.png");

    expect(consoleErrors.filter((e) => !e.includes("401"))).toEqual([]);
  });
});
