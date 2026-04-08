import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { AuthUser } from "../stores/auth";

// Mock the auth store
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
    if (typeof selector === "function") return (selector as (s: typeof mockAuthStore) => unknown)(mockAuthStore);
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
      if (typeof selector === "function") return (selector as (s: typeof state) => unknown)(state);
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
    get: vi.fn().mockResolvedValue({ work_order: null, events: [], evidence: [] }),
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

import App from "../App";

function renderWithProviders(initialRoute = "/") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialRoute]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("App", () => {
  beforeEach(() => {
    mockAuthStore.user = null;
    mockAuthStore.loading = false;
    mockAuthStore.error = null;
    mockAuthStore.bootstrap.mockReset();
    mockAuthStore.login.mockReset();
    mockAuthStore.logout.mockReset();
  });

  it("renders login page at /login", () => {
    renderWithProviders("/login");
    expect(screen.getByText("FieldServe")).toBeDefined();
    expect(screen.getByLabelText("Username")).toBeDefined();
    expect(screen.getByLabelText("Password")).toBeDefined();
    expect(screen.getByRole("button", { name: "Sign In" })).toBeDefined();
  });

  it("redirects unauthenticated user from /customer to /login", () => {
    renderWithProviders("/customer");
    expect(screen.getByText("FieldServe")).toBeDefined();
    expect(screen.getByLabelText("Username")).toBeDefined();
  });

  it("shows 404 for unknown routes", () => {
    renderWithProviders("/some/random");
    expect(screen.getByText("404")).toBeDefined();
    expect(screen.getByText("Page not found")).toBeDefined();
  });

  it("renders customer dashboard for authenticated customer", () => {
    mockAuthStore.user = {
      id: "1",
      username: "customer",
      email: "c@test.com",
      roles: ["customer"],
    };
    renderWithProviders("/customer");
    expect(screen.getByText("Customer Dashboard")).toBeDefined();
  });

  it("renders admin dashboard for authenticated admin", () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };
    renderWithProviders("/admin");
    expect(screen.getByText("Admin Dashboard")).toBeDefined();
  });

  it("shows forbidden for customer accessing admin route", () => {
    mockAuthStore.user = {
      id: "1",
      username: "customer",
      email: "c@test.com",
      roles: ["customer"],
    };
    renderWithProviders("/admin");
    expect(screen.getByText("Forbidden — Insufficient permissions")).toBeDefined();
  });

  it("renders provider dashboard for authenticated provider", () => {
    mockAuthStore.user = {
      id: "3",
      username: "provider",
      email: "p@test.com",
      roles: ["provider"],
    };
    renderWithProviders("/provider");
    expect(screen.getByText("Provider Dashboard")).toBeDefined();
  });

  it("renders admin categories page", () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };
    renderWithProviders("/admin/categories");
    expect(screen.getByRole("heading", { name: "Categories" })).toBeDefined();
  });

  it("renders provider services page", () => {
    mockAuthStore.user = {
      id: "3",
      username: "provider",
      email: "p@test.com",
      roles: ["provider"],
    };
    renderWithProviders("/provider/services");
    expect(screen.getByText("My Services")).toBeDefined();
  });

  it("renders customer catalog page", () => {
    mockAuthStore.user = {
      id: "1",
      username: "customer",
      email: "c@test.com",
      roles: ["customer"],
    };
    renderWithProviders("/customer/catalog");
    expect(screen.getByText("Browse Services")).toBeDefined();
  });

  it("renders customer favorites page", () => {
    mockAuthStore.user = {
      id: "1",
      username: "customer",
      email: "c@test.com",
      roles: ["customer"],
    };
    renderWithProviders("/customer/favorites");
    expect(screen.getByText("My Favorites")).toBeDefined();
  });

  it("renders customer compare page", () => {
    mockAuthStore.user = {
      id: "1",
      username: "customer",
      email: "c@test.com",
      roles: ["customer"],
    };
    renderWithProviders("/customer/compare");
    expect(screen.getByText("Compare Services")).toBeDefined();
  });

  it("renders admin hot keywords page", () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };
    renderWithProviders("/admin/hot-keywords");
    expect(screen.getByRole("heading", { name: "Hot Keywords" })).toBeDefined();
  });

  it("renders admin autocomplete page", () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };
    renderWithProviders("/admin/autocomplete");
    expect(screen.getByRole("heading", { name: "Autocomplete Terms" })).toBeDefined();
  });

  it("renders customer interests page", () => {
    mockAuthStore.user = {
      id: "1",
      username: "customer",
      email: "c@test.com",
      roles: ["customer"],
    };
    renderWithProviders("/customer/interests");
    expect(screen.getByText("My Interests")).toBeDefined();
  });

  it("renders customer messages page", () => {
    mockAuthStore.user = {
      id: "1",
      username: "customer",
      email: "c@test.com",
      roles: ["customer"],
    };
    renderWithProviders("/customer/messages");
    expect(screen.getByRole("heading", { name: "Messages" })).toBeDefined();
  });

  it("renders provider interests page", () => {
    mockAuthStore.user = {
      id: "3",
      username: "provider",
      email: "p@test.com",
      roles: ["provider"],
    };
    renderWithProviders("/provider/interests");
    expect(screen.getByText("Incoming Interests")).toBeDefined();
  });

  it("renders provider messages page", () => {
    mockAuthStore.user = {
      id: "3",
      username: "provider",
      email: "p@test.com",
      roles: ["provider"],
    };
    renderWithProviders("/provider/messages");
    expect(screen.getByRole("heading", { name: "Messages" })).toBeDefined();
  });

  it("renders provider documents page", () => {
    mockAuthStore.user = {
      id: "3",
      username: "provider",
      email: "p@test.com",
      roles: ["provider"],
    };
    renderWithProviders("/provider/documents");
    expect(screen.getByRole("heading", { name: "Documents" })).toBeDefined();
  });

  it("renders admin analytics page", () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };
    renderWithProviders("/admin/analytics");
    expect(screen.getByRole("heading", { name: "Analytics" })).toBeDefined();
  });

  it("renders admin exports page", () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };
    renderWithProviders("/admin/exports");
    expect(screen.getByRole("heading", { name: "Exports" })).toBeDefined();
  });

  it("renders admin alert rules page", () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };
    renderWithProviders("/admin/alert-rules");
    expect(screen.getByRole("heading", { name: "Alert Rules" })).toBeDefined();
  });

  it("renders admin alert center page", () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };
    renderWithProviders("/admin/alerts");
    expect(screen.getByRole("heading", { name: "Alert Center" })).toBeDefined();
  });

  it("renders admin work orders page", () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };
    renderWithProviders("/admin/work-orders");
    expect(screen.getByRole("heading", { name: "Work Orders" })).toBeDefined();
  });
});
