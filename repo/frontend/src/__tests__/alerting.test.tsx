import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { AuthUser } from "../stores/auth";

const mockAuthStore = {
  user: null as AuthUser | null,
  loading: false,
  error: null as string | null,
  bootstrap: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
};

vi.mock("../stores/auth", () => ({
  useAuthStore: vi.fn((selector?: unknown) => {
    if (typeof selector === "function")
      return (selector as (s: typeof mockAuthStore) => unknown)(mockAuthStore);
    return mockAuthStore;
  }),
  hasRole: (user: AuthUser | null, role: string) =>
    user?.roles?.includes(role) ?? false,
  primaryRole: (user: AuthUser | null) => {
    if (!user) return null;
    if (user.roles.includes("administrator")) return "administrator";
    if (user.roles.includes("provider")) return "provider";
    if (user.roles.includes("customer")) return "customer";
    return null;
  },
  roleHomePath: (role: string | null) => {
    switch (role) {
      case "administrator":
        return "/admin";
      case "provider":
        return "/provider";
      case "customer":
        return "/customer";
      default:
        return "/login";
    }
  },
}));

vi.mock("../stores/compare", () => {
  const items: unknown[] = [];
  return {
    useCompareStore: vi.fn((selector?: unknown) => {
      const state = {
        items,
        add: vi.fn(() => true),
        remove: vi.fn(),
        clear: vi.fn(),
        has: vi.fn(() => false),
      };
      if (typeof selector === "function")
        return (selector as (s: typeof state) => unknown)(state);
      return state;
    }),
    MAX_COMPARE: 3,
  };
});

vi.mock("../api/engagement", () => ({
  interestApi: {
    customerSubmit: vi.fn(),
    customerList: vi.fn().mockResolvedValue({ interests: [] }),
    customerGet: vi.fn().mockResolvedValue({ interest: null, events: [] }),
    customerWithdraw: vi.fn(),
    providerList: vi.fn().mockResolvedValue({ interests: [] }),
    providerAccept: vi.fn(),
    providerDecline: vi.fn(),
  },
  messageApi: {
    customerThreads: vi.fn().mockResolvedValue({ threads: [] }),
    customerThread: vi.fn().mockResolvedValue({ messages: [] }),
    customerSend: vi.fn(),
    customerMarkRead: vi.fn().mockResolvedValue({ message: "ok" }),
    providerThreads: vi.fn().mockResolvedValue({ threads: [] }),
    providerThread: vi.fn().mockResolvedValue({ messages: [] }),
    providerSend: vi.fn(),
    providerMarkRead: vi.fn().mockResolvedValue({ message: "ok" }),
  },
  blockApi: {
    customerBlock: vi.fn(),
    customerUnblock: vi.fn(),
    providerBlock: vi.fn(),
    providerUnblock: vi.fn(),
  },
}));

vi.mock("../api/catalog", () => ({
  catalogApi: {
    listCategories: vi.fn().mockResolvedValue({ categories: [] }),
    listTags: vi.fn().mockResolvedValue({ tags: [] }),
    listServices: vi.fn().mockResolvedValue({ services: [], total: 0, page: 1, page_size: 20 }),
    getService: vi.fn().mockResolvedValue({ service: null }),
    getTrending: vi.fn().mockResolvedValue({ services: [] }),
    getHotKeywords: vi.fn().mockResolvedValue({ keywords: [] }),
    getAutocomplete: vi.fn().mockResolvedValue({ terms: [] }),
  },
  adminApi: {
    listCategories: vi.fn().mockResolvedValue({ categories: [] }),
    createCategory: vi.fn(),
    updateCategory: vi.fn(),
    listTags: vi.fn().mockResolvedValue({ tags: [] }),
    createTag: vi.fn(),
    updateTag: vi.fn(),
    listHotKeywords: vi.fn().mockResolvedValue({ keywords: [] }),
    createHotKeyword: vi.fn(),
    updateHotKeyword: vi.fn(),
    listAutocomplete: vi.fn().mockResolvedValue({ terms: [] }),
    createAutocomplete: vi.fn(),
    updateAutocomplete: vi.fn(),
  },
  providerApi: {
    listServices: vi.fn().mockResolvedValue({ services: [] }),
    getService: vi.fn().mockResolvedValue({ service: null }),
    createService: vi.fn(),
    updateService: vi.fn(),
    deleteService: vi.fn(),
    setAvailability: vi.fn(),
  },
  customerApi: {
    getFavorites: vi.fn().mockResolvedValue({ favorites: [] }),
    addFavorite: vi.fn(),
    removeFavorite: vi.fn(),
    getSearchHistory: vi.fn().mockResolvedValue({ history: [] }),
  },
}));

vi.mock("../api/operations", () => ({
  documentsApi: {
    list: vi.fn().mockResolvedValue({ documents: [] }),
    upload: vi.fn(),
    delete: vi.fn(),
  },
  analyticsApi: {
    userGrowth: vi.fn().mockResolvedValue({ metrics: [] }),
    conversion: vi.fn().mockResolvedValue({ metrics: [] }),
    providerUtilization: vi.fn().mockResolvedValue({ providers: [] }),
  },
  exportsApi: {
    create: vi.fn(),
    list: vi.fn().mockResolvedValue({ exports: [] }),
    get: vi.fn(),
    downloadUrl: vi.fn(),
  },
}));

vi.mock("../api/alerting", () => ({
  alertRulesApi: {
    list: vi.fn().mockResolvedValue({ alert_rules: [] }),
    create: vi.fn(),
    update: vi.fn(),
  },
  alertsApi: {
    list: vi.fn().mockResolvedValue({ alerts: [] }),
    get: vi.fn(),
    assign: vi.fn(),
    acknowledge: vi.fn(),
  },
  onCallApi: {
    list: vi.fn().mockResolvedValue({ on_call_schedules: [] }),
    create: vi.fn(),
  },
  workOrdersApi: {
    create: vi.fn(),
    list: vi.fn().mockResolvedValue({ work_orders: [] }),
    get: vi.fn(),
    dispatch: vi.fn(),
    acknowledge: vi.fn(),
    start: vi.fn(),
    resolve: vi.fn(),
    postIncidentReview: vi.fn(),
    close: vi.fn(),
    uploadEvidence: vi.fn(),
    listEvidence: vi.fn(),
  },
}));

import { alertRulesApi, alertsApi, workOrdersApi, onCallApi } from "../api/alerting";
import AlertRulesPage from "../pages/admin/AlertRulesPage";
import AlertCenterPage from "../pages/admin/AlertCenterPage";
import WorkOrderDetailPage from "../pages/admin/WorkOrderDetailPage";

function renderPage(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderPageWithRoute(initialRoute: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialRoute]}>
        <Routes>
          <Route path="/admin/work-orders/:id" element={<WorkOrderDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("Alerting", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };
  });

  it("alert rule create with validation", async () => {
    (alertRulesApi.create as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      alert_rule: {
        id: "r1",
        name: "Unresolved Interests",
        condition: { metric: "unresolved_interests", threshold: 5 },
        severity: "high",
        quiet_hours_start: null,
        quiet_hours_end: null,
        enabled: true,
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    });

    renderPage(<AlertRulesPage />);

    // Click Add Rule
    fireEvent.click(screen.getByText("Add Rule"));

    // Fill form
    fireEvent.change(screen.getByLabelText(/Name/), {
      target: { value: "Unresolved Interests" },
    });
    fireEvent.change(screen.getByLabelText(/Threshold/), {
      target: { value: "90" },
    });
    fireEvent.change(screen.getByLabelText(/Severity/), {
      target: { value: "high" },
    });

    // Submit
    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(alertRulesApi.create).toHaveBeenCalledWith(
        expect.objectContaining({
          name: "Unresolved Interests",
          condition: { metric: "unresolved_interests", threshold: 90 },
          severity: "high",
          enabled: true,
        }),
      );
    });
  });

  it("alert center renders alerts with severity badges", async () => {
    (alertsApi.list as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      alerts: [
        {
          id: "a1",
          rule_id: "r1",
          rule_name: "High CPU Alert",
          severity: "critical",
          status: "new",
          data: { value: 95 },
          created_at: "2026-01-01T00:00:00Z",
          resolved_at: null,
        },
        {
          id: "a2",
          rule_id: "r2",
          rule_name: "Disk Usage Warning",
          severity: "low",
          status: "assigned",
          data: { value: 70 },
          created_at: "2026-01-02T00:00:00Z",
          resolved_at: null,
        },
      ],
    });

    renderPage(<AlertCenterPage />);

    await waitFor(() => {
      expect(screen.getByText("High CPU Alert")).toBeDefined();
      expect(screen.getByText("Disk Usage Warning")).toBeDefined();
      expect(screen.getByText("critical")).toBeDefined();
      expect(screen.getByText("low")).toBeDefined();
    });
  });

  it("work order lifecycle actions", async () => {
    // Start with a "new" work order
    (workOrdersApi.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      work_order: {
        id: "wo1",
        alert_id: "a1",
        status: "new",
        assigned_to: "user1",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
      events: [],
      evidence: [],
    });

    (workOrdersApi.dispatch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      work_order: {
        id: "wo1",
        alert_id: "a1",
        status: "dispatched",
        assigned_to: "user1",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:01:00Z",
      },
    });

    renderPageWithRoute("/admin/work-orders/wo1");

    // Should show Dispatch button for "new" status
    await waitFor(() => {
      expect(screen.getByText("Dispatch")).toBeDefined();
    });

    fireEvent.click(screen.getByText("Dispatch"));

    await waitFor(() => {
      expect(workOrdersApi.dispatch).toHaveBeenCalledWith("wo1");
    });
  });

  it("evidence upload on work order", async () => {
    (workOrdersApi.get as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      work_order: {
        id: "wo2",
        alert_id: null,
        status: "in_progress",
        assigned_to: "user1",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
      events: [],
      evidence: [],
    });

    (workOrdersApi.uploadEvidence as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      evidence: {
        id: "ev1",
        work_order_id: "wo2",
        file_path: "/uploads/photo.jpg",
        uploaded_by: "user1",
        created_at: "2026-01-01T00:00:00Z",
        retention_expires_at: "2027-01-01T00:00:00Z",
      },
    });

    renderPageWithRoute("/admin/work-orders/wo2");

    await waitFor(() => {
      expect(screen.getByText("Upload Evidence")).toBeDefined();
    });

    const fileInput = screen.getByTestId("evidence-file-input");
    const file = new File(["image content"], "photo.jpg", {
      type: "image/jpeg",
    });

    fireEvent.change(fileInput, { target: { files: [file] } });

    await waitFor(() => {
      expect(workOrdersApi.uploadEvidence).toHaveBeenCalledWith("wo2", file);
    });
  });

  it("integration flow: rule -> alert -> work order -> resolution", async () => {
    // Step 1: Create an alert rule
    (alertRulesApi.create as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      alert_rule: {
        id: "r1",
        name: "Unresolved Interests Alert",
        condition: { metric: "unresolved_interests", threshold: 85 },
        severity: "high",
        quiet_hours_start: null,
        quiet_hours_end: null,
        enabled: true,
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    });

    const { unmount: unmount1 } = renderPage(<AlertRulesPage />);

    fireEvent.click(screen.getByText("Add Rule"));
    fireEvent.change(screen.getByLabelText(/Name/), {
      target: { value: "Memory Alert" },
    });
    fireEvent.change(screen.getByLabelText(/Metric/), {
      target: { value: "unresolved_interests" },
    });
    fireEvent.change(screen.getByLabelText(/Threshold/), {
      target: { value: "85" },
    });
    fireEvent.change(screen.getByLabelText(/Severity/), {
      target: { value: "high" },
    });
    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(alertRulesApi.create).toHaveBeenCalled();
    });

    unmount1();

    // Step 2: See alert and assign it
    (alertsApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({
      alerts: [
        {
          id: "a1",
          rule_id: "r1",
          rule_name: "Memory Alert",
          severity: "high",
          status: "new",
          data: { value: 92 },
          created_at: "2026-01-01T01:00:00Z",
          resolved_at: null,
        },
      ],
    });

    (alertsApi.assign as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      assignment: {
        id: "asgn1",
        alert_id: "a1",
        assignee_id: "tech-1",
        assigned_at: "2026-01-01T01:05:00Z",
        acknowledged_at: null,
      },
    });

    // Mock on-call users so the select has options
    (onCallApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({
      on_call_schedules: [
        { id: "oc1", user_id: "tech-1", tier: 1, start_time: "2026-01-01T00:00:00Z", end_time: "2026-01-02T00:00:00Z", created_at: "2026-01-01T00:00:00Z" },
      ],
    });

    (workOrdersApi.create as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      work_order: {
        id: "wo1",
        alert_id: "a1",
        status: "new",
        assigned_to: null,
        created_at: "2026-01-01T01:10:00Z",
        updated_at: "2026-01-01T01:10:00Z",
      },
    });

    const { unmount: unmount2 } = renderPage(<AlertCenterPage />);

    await waitFor(() => {
      expect(screen.getByText("Memory Alert")).toBeDefined();
    });

    // Assign — now uses select dropdown
    const assignButtons = screen.getAllByText("Assign");
    fireEvent.click(assignButtons[0]);

    const assignSelect = screen.getByTestId("assignee-select");
    fireEvent.change(assignSelect, { target: { value: "tech-1" } });
    fireEvent.click(screen.getByText("Confirm Assign"));

    await waitFor(() => {
      expect(alertsApi.assign).toHaveBeenCalledWith("a1", "tech-1");
    });

    // Create work order
    fireEvent.click(screen.getByText("Create Work Order"));

    await waitFor(() => {
      expect(workOrdersApi.create).toHaveBeenCalledWith({ alert_id: "a1" });
    });

    unmount2();

    // Step 3: Work order lifecycle - dispatch through close
    (workOrdersApi.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      work_order: {
        id: "wo1",
        alert_id: "a1",
        status: "new",
        assigned_to: null,
        created_at: "2026-01-01T01:10:00Z",
        updated_at: "2026-01-01T01:10:00Z",
      },
      events: [],
      evidence: [],
    });

    (workOrdersApi.dispatch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      work_order: { id: "wo1", status: "dispatched", alert_id: "a1", assigned_to: null, created_at: "2026-01-01T01:10:00Z", updated_at: "2026-01-01T01:11:00Z" },
    });

    const { unmount: unmount3 } = renderPageWithRoute("/admin/work-orders/wo1");

    await waitFor(() => {
      expect(screen.getByText("Dispatch")).toBeDefined();
    });

    fireEvent.click(screen.getByText("Dispatch"));

    await waitFor(() => {
      expect(workOrdersApi.dispatch).toHaveBeenCalledWith("wo1");
    });

    unmount3();

    // Step 4: Progress through remaining lifecycle — acknowledge → start → resolve → post-incident review → close
    const progressSteps = [
      { current: "dispatched", action: "Acknowledge", fn: workOrdersApi.acknowledge, next: "acknowledged" },
      { current: "acknowledged", action: "Start", fn: workOrdersApi.start, next: "in_progress" },
      { current: "in_progress", action: "Resolve", fn: workOrdersApi.resolve, next: "resolved" },
      { current: "resolved", action: "Post-Incident Review", fn: workOrdersApi.postIncidentReview, next: "post_incident_review" },
      { current: "post_incident_review", action: "Close", fn: workOrdersApi.close, next: "closed" },
    ];

    for (const step of progressSteps) {
      (workOrdersApi.get as ReturnType<typeof vi.fn>).mockResolvedValue({
        work_order: {
          id: "wo1", alert_id: "a1", status: step.current, assigned_to: null,
          created_at: "2026-01-01T01:10:00Z", updated_at: "2026-01-01T01:15:00Z",
        },
        events: [{ id: "ev1", work_order_id: "wo1", old_status: null, new_status: step.current, actor_id: "2", created_at: "2026-01-01T01:15:00Z" }],
        evidence: [],
      });

      (step.fn as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        work_order: {
          id: "wo1", alert_id: "a1", status: step.next, assigned_to: null,
          created_at: "2026-01-01T01:10:00Z", updated_at: "2026-01-01T01:16:00Z",
        },
      });

      const { unmount: umStep } = renderPageWithRoute("/admin/work-orders/wo1");

      await waitFor(() => {
        expect(screen.getByText(step.action)).toBeDefined();
      });

      fireEvent.click(screen.getByText(step.action));

      await waitFor(() => {
        expect(step.fn).toHaveBeenCalledWith("wo1");
      });

      umStep();
    }

    // Step 5: Upload evidence on the closed work order
    (workOrdersApi.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      work_order: {
        id: "wo1", alert_id: "a1", status: "closed", assigned_to: null,
        created_at: "2026-01-01T01:10:00Z", updated_at: "2026-01-01T01:20:00Z",
      },
      events: [],
      evidence: [],
    });

    (workOrdersApi.uploadEvidence as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      evidence: {
        id: "ev-new", work_order_id: "wo1", file_path: "/app/data/evidence/uuid.pdf",
        uploaded_by: "2", created_at: "2026-01-01T02:00:00Z",
        retention_expires_at: "2026-07-01T02:00:00Z",
      },
    });

    const { unmount: unmountEvidence } = renderPageWithRoute("/admin/work-orders/wo1");

    await waitFor(() => {
      expect(screen.getByTestId("evidence-file-input")).toBeDefined();
    });

    const evidenceInput = screen.getByTestId("evidence-file-input");
    const evidenceFile = new File(["evidence pdf"], "incident-report.pdf", { type: "application/pdf" });
    fireEvent.change(evidenceInput, { target: { files: [evidenceFile] } });

    await waitFor(() => {
      expect(workOrdersApi.uploadEvidence).toHaveBeenCalledWith("wo1", evidenceFile);
    });

    unmountEvidence();
  });

  it("alert center shows on-call users in assignment select", async () => {
    (alertsApi.list as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      alerts: [
        {
          id: "a3", rule_id: "r3", rule_name: "Test Alert", severity: "high",
          status: "new", data: {}, created_at: "2026-01-01T00:00:00Z", resolved_at: null,
        },
      ],
    });

    (onCallApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({
      on_call_schedules: [
        { id: "oc1", user_id: "oncall-user-1", tier: 1, start_time: "2026-01-01T00:00:00Z", end_time: "2026-01-02T00:00:00Z", created_at: "2026-01-01T00:00:00Z" },
        { id: "oc2", user_id: "oncall-user-2", tier: 2, start_time: "2026-01-01T00:00:00Z", end_time: "2026-01-02T00:00:00Z", created_at: "2026-01-01T00:00:00Z" },
      ],
    });

    renderPage(<AlertCenterPage />);

    await waitFor(() => {
      expect(screen.getByText("Test Alert")).toBeDefined();
    });

    // Click Assign to show the select
    const assignButtons = screen.getAllByText("Assign");
    fireEvent.click(assignButtons[0]);

    // Should show on-call user select
    await waitFor(() => {
      const select = screen.getByTestId("assignee-select");
      expect(select).toBeDefined();
    });
  });

  // Focused acknowledge test
  it("acknowledge action calls alert API correctly", async () => {
    (alertsApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({
      alerts: [
        {
          id: "a2", rule_id: "r2", rule_name: "Overdue WO", severity: "critical",
          status: "new", data: { value: 3 }, created_at: "2026-02-01T00:00:00Z", resolved_at: null,
        },
      ],
    });

    (alertsApi.acknowledge as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      message: "Alert acknowledged.",
    });

    renderPage(<AlertCenterPage />);

    await waitFor(() => {
      expect(screen.getByText("Overdue WO")).toBeDefined();
    });

    // Find and click the Acknowledge button
    const ackButtons = screen.getAllByText("Acknowledge");
    fireEvent.click(ackButtons[0]);

    await waitFor(() => {
      expect(alertsApi.acknowledge).toHaveBeenCalledWith("a2");
    });
  });
});
