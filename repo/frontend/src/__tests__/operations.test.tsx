import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
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
    upload: vi.fn().mockResolvedValue({
      document: {
        id: "d1",
        provider_id: "p1",
        filename: "test.pdf",
        mime_type: "application/pdf",
        size_bytes: 1024,
        checksum_sha256: "abc",
        storage_path: "/files/test.pdf",
        created_at: "2026-01-01T00:00:00Z",
      },
    }),
    delete: vi.fn().mockResolvedValue({ message: "Document deleted." }),
  },
  analyticsApi: {
    userGrowth: vi.fn().mockResolvedValue({ metrics: [] }),
    conversion: vi.fn().mockResolvedValue({ metrics: [] }),
    providerUtilization: vi.fn().mockResolvedValue({ providers: [] }),
  },
  exportsApi: {
    create: vi.fn().mockResolvedValue({
      export: {
        id: "e1",
        admin_id: "a1",
        export_type: "user_growth",
        status: "pending",
        file_path: null,
        created_at: "2026-01-01T00:00:00Z",
        completed_at: null,
      },
    }),
    list: vi.fn().mockResolvedValue({ exports: [] }),
    get: vi.fn(),
    downloadUrl: vi.fn((id: string) => `/api/v1/admin/exports/${id}/download`),
  },
}));

import { documentsApi, analyticsApi, exportsApi } from "../api/operations";
import DocumentsPage from "../pages/provider/DocumentsPage";
import AnalyticsPage from "../pages/admin/AnalyticsPage";
import ExportsPage from "../pages/admin/ExportsPage";

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

describe("Operations", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthStore.user = null;
  });

  it("provider document upload success", async () => {
    mockAuthStore.user = {
      id: "3",
      username: "provider",
      email: "p@test.com",
      roles: ["provider"],
    };

    renderPage(<DocumentsPage />);

    const fileInput = screen.getByTestId("file-input");
    const file = new File(["content"], "test.pdf", {
      type: "application/pdf",
    });

    fireEvent.change(fileInput, { target: { files: [file] } });

    await waitFor(() => {
      expect(documentsApi.upload).toHaveBeenCalledWith(file);
    });
  });

  it("provider document rejected file error", async () => {
    mockAuthStore.user = {
      id: "3",
      username: "provider",
      email: "p@test.com",
      roles: ["provider"],
    };

    const { ApiError } = await import("../api/client");
    (documentsApi.upload as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new ApiError(415, {
        error: { code: "invalid_file_type", message: "Invalid file type" },
      }),
    );

    renderPage(<DocumentsPage />);

    const fileInput = screen.getByTestId("file-input");
    const file = new File(["content"], "test.exe", {
      type: "application/octet-stream",
    });

    fireEvent.change(fileInput, { target: { files: [file] } });

    await waitFor(() => {
      expect(screen.getByText("File type not allowed")).toBeDefined();
    });
  });

  it("provider document list and delete", async () => {
    mockAuthStore.user = {
      id: "3",
      username: "provider",
      email: "p@test.com",
      roles: ["provider"],
    };

    (documentsApi.list as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      documents: [
        {
          id: "d1",
          provider_id: "p1",
          filename: "invoice.pdf",
          mime_type: "application/pdf",
          size_bytes: 2048,
          checksum_sha256: "abc",
          storage_path: "/files/invoice.pdf",
          created_at: "2026-01-15T00:00:00Z",
        },
        {
          id: "d2",
          provider_id: "p1",
          filename: "photo.jpg",
          mime_type: "image/jpeg",
          size_bytes: 500000,
          checksum_sha256: "def",
          storage_path: "/files/photo.jpg",
          created_at: "2026-02-01T00:00:00Z",
        },
      ],
    });

    renderPage(<DocumentsPage />);

    await waitFor(() => {
      expect(screen.getByText("invoice.pdf")).toBeDefined();
      expect(screen.getByText("photo.jpg")).toBeDefined();
    });

    const deleteButtons = screen.getAllByText("Delete");
    fireEvent.click(deleteButtons[0]);

    // Confirmation step
    await waitFor(() => {
      expect(screen.getByText("Confirm")).toBeDefined();
    });

    fireEvent.click(screen.getByText("Confirm"));

    await waitFor(() => {
      expect(documentsApi.delete).toHaveBeenCalledWith("d1");
    });
  });

  it("admin analytics dashboard renders metrics", async () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };

    (analyticsApi.userGrowth as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      metrics: [
        { date: "2026-01-01", value: 42, label: "customer" },
      ],
    });
    (analyticsApi.conversion as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      metrics: [
        { date: "2026-01-01", searches: 100, interests: 25, rate: 0.25 },
      ],
    });
    (analyticsApi.providerUtilization as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      providers: [
        {
          id: "p1",
          business_name: "Acme Plumbing",
          active_services: 5,
          total_interests: 12,
          messages_sent: 30,
        },
      ],
    });

    renderPage(<AnalyticsPage />);

    await waitFor(() => {
      expect(screen.getByText("42")).toBeDefined();
      expect(screen.getByText("customer")).toBeDefined();
    });

    await waitFor(() => {
      expect(screen.getByText("100")).toBeDefined();
      expect(screen.getByText("25")).toBeDefined();
      expect(screen.getByText("25.0%")).toBeDefined();
    });

    await waitFor(() => {
      expect(screen.getByText("Acme Plumbing")).toBeDefined();
      expect(screen.getByText("5")).toBeDefined();
      expect(screen.getByText("12")).toBeDefined();
      expect(screen.getByText("30")).toBeDefined();
    });
  });

  it("admin export request and list", async () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };

    (exportsApi.list as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      exports: [
        {
          id: "e1",
          admin_id: "a1",
          export_type: "user_growth",
          status: "completed",
          file_path: "/exports/e1.csv",
          created_at: "2026-01-01T00:00:00Z",
          completed_at: "2026-01-01T00:01:00Z",
        },
        {
          id: "e2",
          admin_id: "a1",
          export_type: "conversion",
          status: "pending",
          file_path: null,
          created_at: "2026-01-02T00:00:00Z",
          completed_at: null,
        },
      ],
    });

    renderPage(<ExportsPage />);

    await waitFor(() => {
      expect(screen.getByText("user_growth")).toBeDefined();
      expect(screen.getByText("completed")).toBeDefined();
      expect(screen.getByText("conversion")).toBeDefined();
      expect(screen.getByText("pending")).toBeDefined();
    });

    // completed export has download link
    expect(screen.getByText("Download")).toBeDefined();
  });

  it("integration flow: provider uploads doc, admin reads analytics, admin requests export", async () => {
    // ========== Step 1: Provider uploads a document ==========
    mockAuthStore.user = {
      id: "3",
      username: "provider",
      email: "p@test.com",
      roles: ["provider"],
    };

    // After upload succeeds, the list should refresh with the new doc
    (documentsApi.upload as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      document: {
        id: "d-new",
        provider_id: "p1",
        filename: "license.pdf",
        mime_type: "application/pdf",
        size_bytes: 4096,
        checksum_sha256: "abc123",
        storage_path: "/app/data/uploads/uuid.pdf",
        created_at: "2026-04-01T00:00:00Z",
      },
    });

    const { unmount: unmount1 } = renderPage(<DocumentsPage />);

    // Trigger file upload via file input
    const fileInput = screen.getByTestId("file-input");
    const testFile = new File(["pdf content"], "license.pdf", {
      type: "application/pdf",
    });
    fireEvent.change(fileInput, { target: { files: [testFile] } });

    // Verify upload was called with the file
    await waitFor(() => {
      expect(documentsApi.upload).toHaveBeenCalledWith(testFile);
    });

    unmount1();

    // ========== Step 2: Admin views analytics with real data ==========
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };

    (analyticsApi.userGrowth as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      metrics: [
        { date: "2026-04-01", value: 15, label: "customer" },
        { date: "2026-04-01", value: 3, label: "provider" },
      ],
    });
    (analyticsApi.conversion as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      metrics: [
        { date: "2026-04-01", searches: 200, interests: 40, rate: 0.2 },
      ],
    });
    (analyticsApi.providerUtilization as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      providers: [
        {
          id: "p1",
          business_name: "Demo Provider",
          active_services: 3,
          total_interests: 8,
          messages_sent: 12,
        },
      ],
    });

    const { unmount: unmount2 } = renderPage(<AnalyticsPage />);

    // Verify user growth metrics rendered
    await waitFor(() => {
      expect(screen.getByText("15")).toBeDefined();
      expect(screen.getByText("customer")).toBeDefined();
    });

    // Verify conversion metrics rendered
    await waitFor(() => {
      expect(screen.getByText("200")).toBeDefined();
      expect(screen.getByText("20.0%")).toBeDefined();
    });

    // Verify provider utilization rendered
    await waitFor(() => {
      expect(screen.getByText("Demo Provider")).toBeDefined();
    });

    unmount2();

    // ========== Step 3: Admin requests export and sees completed job ==========
    (exportsApi.create as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      export: {
        id: "e-new",
        admin_id: "a1",
        export_type: "user_growth",
        status: "completed",
        file_path: "/app/data/exports/e-new.csv",
        created_at: "2026-04-01T10:00:00Z",
        completed_at: "2026-04-01T10:00:01Z",
      },
    });
    (exportsApi.list as ReturnType<typeof vi.fn>).mockResolvedValue({
      exports: [
        {
          id: "e-new",
          admin_id: "a1",
          export_type: "user_growth",
          status: "completed",
          file_path: "/app/data/exports/e-new.csv",
          created_at: "2026-04-01T10:00:00Z",
          completed_at: "2026-04-01T10:00:01Z",
        },
      ],
    });

    renderPage(<ExportsPage />);

    // Click the "Request Export" button (not the heading)
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Request Export" })).toBeDefined();
    });

    fireEvent.click(screen.getByRole("button", { name: "Request Export" }));

    // Verify export was created
    await waitFor(() => {
      expect(exportsApi.create).toHaveBeenCalled();
    });

    // Verify completed export appears in list
    await waitFor(() => {
      expect(screen.getByText("user_growth")).toBeDefined();
      expect(screen.getByText("completed")).toBeDefined();
      expect(screen.getByText("Download")).toBeDefined();
    });
  });
});
