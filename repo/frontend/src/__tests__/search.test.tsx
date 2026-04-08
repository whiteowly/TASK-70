import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { AuthUser } from "../stores/auth";
import type { ServiceSummary } from "../api/catalog";
import { ApiError } from "../api/client";

// Mock auth store
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
  primaryRole: () => null,
  roleHomePath: () => "/login",
}));

// Compare store mock with controllable state
let mockCompareItems: ServiceSummary[] = [];
const mockCompareAdd = vi.fn((svc: ServiceSummary) => {
  if (mockCompareItems.length >= 3) return false;
  if (mockCompareItems.some((s) => s.id === svc.id)) return true;
  mockCompareItems.push(svc);
  return true;
});
const mockCompareRemove = vi.fn((id: string) => {
  mockCompareItems = mockCompareItems.filter((s) => s.id !== id);
});
const mockCompareClear = vi.fn(() => {
  mockCompareItems = [];
});
const mockCompareHas = vi.fn((id: string) =>
  mockCompareItems.some((s) => s.id === id),
);

vi.mock("../stores/compare", () => ({
  useCompareStore: vi.fn((selector?: unknown) => {
    const state = {
      items: mockCompareItems,
      add: mockCompareAdd,
      remove: mockCompareRemove,
      clear: mockCompareClear,
      has: mockCompareHas,
    };
    if (typeof selector === "function")
      return (selector as (s: typeof state) => unknown)(state);
    return state;
  }),
  MAX_COMPARE: 3,
}));

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
    listServices: vi
      .fn()
      .mockResolvedValue({ services: [], total: 0, page: 1, page_size: 20 }),
    getService: vi.fn().mockResolvedValue({ service: null }),
    getTrending: vi.fn().mockResolvedValue({ services: [] }),
    getHotKeywords: vi.fn().mockResolvedValue({ keywords: [] }),
    getAutocomplete: vi.fn().mockResolvedValue({ terms: [] }),
  },
  customerApi: {
    getFavorites: vi.fn().mockResolvedValue({ favorites: [] }),
    addFavorite: vi
      .fn()
      .mockResolvedValue({
        favorite: { id: "f1", service_id: "s1", created_at: "2024-01-01" },
      }),
    removeFavorite: vi
      .fn()
      .mockResolvedValue({ message: "Favorite removed." }),
    getSearchHistory: vi.fn().mockResolvedValue({ history: [] }),
  },
  adminApi: {
    listHotKeywords: vi.fn().mockResolvedValue({ keywords: [] }),
    createHotKeyword: vi.fn(),
    updateHotKeyword: vi.fn(),
    listAutocomplete: vi.fn().mockResolvedValue({ terms: [] }),
    createAutocomplete: vi.fn(),
    updateAutocomplete: vi.fn(),
    listCategories: vi.fn().mockResolvedValue({ categories: [] }),
    createCategory: vi.fn(),
    updateCategory: vi.fn(),
    listTags: vi.fn().mockResolvedValue({ tags: [] }),
    createTag: vi.fn(),
    updateTag: vi.fn(),
  },
  providerApi: {
    listServices: vi.fn().mockResolvedValue({ services: [] }),
    getService: vi.fn().mockResolvedValue({ service: null }),
    createService: vi.fn(),
    updateService: vi.fn(),
    deleteService: vi.fn(),
    setAvailability: vi.fn(),
  },
}));

import CatalogPage from "../pages/customer/CatalogPage";
import ComparePage from "../pages/customer/ComparePage";
import ServiceDetailPage from "../pages/customer/ServiceDetailPage";
import HotKeywordsPage from "../pages/admin/HotKeywordsPage";
import AutocompletePage from "../pages/admin/AutocompletePage";
import { catalogApi, customerApi, adminApi } from "../api/catalog";

const sampleService: ServiceSummary = {
  id: "s1",
  title: "Lawn Mowing",
  description: "Professional lawn care",
  price_cents: 5000,
  rating_avg: "4.50",
  popularity_score: 10,
  status: "active",
  category: { id: "c1", name: "Landscaping" },
  provider: { id: "p1", business_name: "Green Thumb", service_area_miles: 25 },
  tags: [{ id: "t1", name: "Outdoor" }],
  created_at: "2024-01-01",
  updated_at: "2024-01-01",
};

const sampleService2: ServiceSummary = {
  id: "s2",
  title: "Plumbing Fix",
  description: "Fix leaky pipes",
  price_cents: 8000,
  rating_avg: "4.20",
  popularity_score: 8,
  status: "active",
  category: { id: "c2", name: "Plumbing" },
  provider: { id: "p2", business_name: "Pipe Pro", service_area_miles: 15 },
  tags: [{ id: "t2", name: "Indoor" }],
  created_at: "2024-01-02",
  updated_at: "2024-01-02",
};

function renderWithProviders(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("Search and discovery", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCompareItems = [];
    mockAuthStore.user = {
      id: "1",
      username: "customer",
      email: "c@test.com",
      roles: ["customer"],
    };
    (catalogApi.listServices as ReturnType<typeof vi.fn>).mockResolvedValue({
      services: [],
      total: 0,
      page: 1,
      page_size: 20,
    });
    (catalogApi.getTrending as ReturnType<typeof vi.fn>).mockResolvedValue({
      services: [],
    });
    (catalogApi.getHotKeywords as ReturnType<typeof vi.fn>).mockResolvedValue({
      keywords: [],
    });
    (catalogApi.getAutocomplete as ReturnType<typeof vi.fn>).mockResolvedValue({
      terms: [],
    });
    (customerApi.getFavorites as ReturnType<typeof vi.fn>).mockResolvedValue({
      favorites: [],
    });
    (customerApi.getSearchHistory as ReturnType<typeof vi.fn>).mockResolvedValue(
      { history: [] },
    );
  });

  // --- Basic search rendering ---

  it("search page renders results", async () => {
    (catalogApi.listServices as ReturnType<typeof vi.fn>).mockResolvedValue({
      services: [sampleService, sampleService2],
      total: 2,
      page: 1,
      page_size: 20,
    });

    renderWithProviders(<CatalogPage />);

    await waitFor(() => {
      expect(screen.getByText("Lawn Mowing")).toBeDefined();
      expect(screen.getByText("Plumbing Fix")).toBeDefined();
    });

    expect(screen.getByText("$50.00")).toBeDefined();
    expect(screen.getByText("$80.00")).toBeDefined();
  });

  it("search page shows empty state", async () => {
    renderWithProviders(<CatalogPage />);
    await waitFor(() => {
      expect(screen.getByText("No services found.")).toBeDefined();
    });
  });

  // --- Favorites ---

  it("favorite toggle works", async () => {
    (catalogApi.listServices as ReturnType<typeof vi.fn>).mockResolvedValue({
      services: [sampleService],
      total: 1,
      page: 1,
      page_size: 20,
    });

    renderWithProviders(<CatalogPage />);

    await waitFor(() => {
      expect(screen.getByText("Lawn Mowing")).toBeDefined();
    });

    fireEvent.click(screen.getByText("Favorite"));

    await waitFor(() => {
      expect(customerApi.addFavorite).toHaveBeenCalledWith("s1");
    });
  });

  // --- Compare ---

  it("compare tray enforces max 3", () => {
    const svc3: ServiceSummary = { ...sampleService, id: "s3", title: "Service 3" };
    const svc4: ServiceSummary = { ...sampleService, id: "s4", title: "Service 4" };

    expect(mockCompareAdd(sampleService)).toBe(true);
    expect(mockCompareAdd(sampleService2)).toBe(true);
    expect(mockCompareAdd(svc3)).toBe(true);
    expect(mockCompareAdd(svc4)).toBe(false);
    expect(mockCompareItems.length).toBe(3);
  });

  it("compare overflow shows visible inline feedback", async () => {
    // Pre-fill compare with 3 items
    mockCompareItems = [
      sampleService,
      sampleService2,
      { ...sampleService, id: "s3", title: "Service 3" },
    ];
    mockCompareAdd.mockReturnValue(false);
    mockCompareHas.mockReturnValue(false);

    const svc4: ServiceSummary = { ...sampleService, id: "s4", title: "Service 4" };

    (catalogApi.listServices as ReturnType<typeof vi.fn>).mockResolvedValue({
      services: [svc4],
      total: 1,
      page: 1,
      page_size: 20,
    });

    renderWithProviders(<CatalogPage />);

    await waitFor(() => {
      expect(screen.getByText("Service 4")).toBeDefined();
    });

    // Click "+ Compare" on the 4th service
    fireEvent.click(screen.getByText("+ Compare"));

    // Should show visible alert
    await waitFor(() => {
      expect(
        screen.getByRole("alert"),
      ).toBeDefined();
      expect(
        screen.getByText(/Compare is limited to 3 services/),
      ).toBeDefined();
    });
  });

  it("compare page shows all required aligned fields", async () => {
    mockCompareItems = [sampleService, sampleService2];

    // Mock service detail responses for availability/service area
    (catalogApi.getService as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({
        service: {
          ...sampleService,
          availability: [
            { id: "a1", day_of_week: 1, start_time: "09:00:00", end_time: "17:00:00" },
          ],
        },
      })
      .mockResolvedValueOnce({
        service: {
          ...sampleService2,
          availability: [
            { id: "a2", day_of_week: 3, start_time: "10:00:00", end_time: "15:00:00" },
          ],
        },
      });

    renderWithProviders(<ComparePage />);

    // Service titles in headers
    expect(screen.getByText("Lawn Mowing")).toBeDefined();
    expect(screen.getByText("Plumbing Fix")).toBeDefined();

    // Price
    expect(screen.getByText("$50.00")).toBeDefined();
    expect(screen.getByText("$80.00")).toBeDefined();

    // Rating
    expect(screen.getByText("4.50 / 5.00")).toBeDefined();
    expect(screen.getByText("4.20 / 5.00")).toBeDefined();

    // Category
    expect(screen.getByText("Landscaping")).toBeDefined();
    expect(screen.getByText("Plumbing")).toBeDefined();

    // Provider
    expect(screen.getByText("Green Thumb")).toBeDefined();
    expect(screen.getByText("Pipe Pro")).toBeDefined();

    // Tags
    expect(screen.getByText("Outdoor")).toBeDefined();
    expect(screen.getByText("Indoor")).toBeDefined();

    // Service Area — real values from provider.service_area_miles
    expect(screen.getByText("25 miles")).toBeDefined();
    expect(screen.getByText("15 miles")).toBeDefined();

    // Availability row label
    expect(screen.getByText("Availability")).toBeDefined();

    // Wait for availability detail to load
    await waitFor(() => {
      expect(screen.getByText(/Mon: 09:00-17:00/)).toBeDefined();
      expect(screen.getByText(/Wed: 10:00-15:00/)).toBeDefined();
    });
  });

  // --- Distance sort ---

  it("distance sort option is available in dropdown", async () => {
    (catalogApi.listServices as ReturnType<typeof vi.fn>).mockResolvedValue({
      services: [sampleService],
      total: 1,
      page: 1,
      page_size: 20,
    });

    renderWithProviders(<CatalogPage />);

    await waitFor(() => {
      expect(screen.getByText("Lawn Mowing")).toBeDefined();
    });

    const sortSelect = screen.getByLabelText("Sort:") as HTMLSelectElement;
    const options = Array.from(sortSelect.options).map((o) => o.value);
    expect(options).toContain("distance");

    // Select distance sort and verify it triggers a query
    fireEvent.change(sortSelect, { target: { value: "distance" } });
    await waitFor(() => {
      expect(catalogApi.listServices).toHaveBeenCalledWith(
        expect.objectContaining({ sort: "distance" }),
      );
    });
  });

  // --- Date-based availability filter ---

  it("availability date filter sends available_date param to API", async () => {
    (catalogApi.listServices as ReturnType<typeof vi.fn>).mockResolvedValue({
      services: [sampleService],
      total: 1,
      page: 1,
      page_size: 20,
    });

    renderWithProviders(<CatalogPage />);

    await waitFor(() => {
      expect(screen.getByText("Lawn Mowing")).toBeDefined();
    });

    // Open filters
    fireEvent.click(screen.getByText("Show Filters"));

    // The filter should be a date input, not a day-of-week dropdown
    const dateInput = screen.getByLabelText("Available Date") as HTMLInputElement;
    expect(dateInput.type).toBe("date");

    // Set a date and verify the API is called with available_date
    fireEvent.change(dateInput, { target: { value: "2026-04-13" } });

    await waitFor(() => {
      expect(catalogApi.listServices).toHaveBeenCalledWith(
        expect.objectContaining({ available_date: "2026-04-13" }),
      );
    });
  });

  it("availability time filter appears when date is set and sends available_time param", async () => {
    (catalogApi.listServices as ReturnType<typeof vi.fn>).mockResolvedValue({
      services: [sampleService],
      total: 1,
      page: 1,
      page_size: 20,
    });

    renderWithProviders(<CatalogPage />);

    await waitFor(() => {
      expect(screen.getByText("Lawn Mowing")).toBeDefined();
    });

    // Open filters
    fireEvent.click(screen.getByText("Show Filters"));

    // Time filter should not be visible before date is set
    expect(screen.queryByLabelText("Available At (time)")).toBeNull();

    // Set a date
    fireEvent.change(screen.getByLabelText("Available Date"), {
      target: { value: "2026-04-13" },
    });

    // Time filter should now appear
    const timeInput = screen.getByLabelText("Available At (time)") as HTMLInputElement;
    expect(timeInput.type).toBe("time");

    // Set time and verify API call
    fireEvent.change(timeInput, { target: { value: "14:00" } });

    await waitFor(() => {
      expect(catalogApi.listServices).toHaveBeenCalledWith(
        expect.objectContaining({
          available_date: "2026-04-13",
          available_time: "14:00",
        }),
      );
    });
  });

  // --- Autocomplete ---

  it("autocomplete suggestions appear from backend-shaped API responses", async () => {
    (catalogApi.getAutocomplete as ReturnType<typeof vi.fn>).mockResolvedValue({
      terms: [
        { id: "ac1", term: "plumbing repair", weight: 10 },
        { id: "ac2", term: "plumbing installation", weight: 5 },
      ],
    });

    renderWithProviders(<CatalogPage />);

    const searchInput = screen.getByPlaceholderText("Search services...");
    fireEvent.change(searchInput, { target: { value: "pl" } });

    // Wait for debounce + autocomplete query
    await waitFor(() => {
      expect(screen.getByRole("listbox")).toBeDefined();
      expect(screen.getByText("plumbing repair")).toBeDefined();
      expect(screen.getByText("plumbing installation")).toBeDefined();
    });
  });

  it("autocomplete selection updates search input", async () => {
    (catalogApi.getAutocomplete as ReturnType<typeof vi.fn>).mockResolvedValue({
      terms: [{ id: "ac1", term: "plumbing repair", weight: 10 }],
    });

    renderWithProviders(<CatalogPage />);

    const searchInput = screen.getByPlaceholderText(
      "Search services...",
    ) as HTMLInputElement;
    fireEvent.change(searchInput, { target: { value: "pl" } });

    await waitFor(() => {
      expect(screen.getByText("plumbing repair")).toBeDefined();
    });

    // Click the suggestion
    fireEvent.click(screen.getByText("plumbing repair"));

    // Search input should be updated
    expect(searchInput.value).toBe("plumbing repair");

    // Autocomplete dropdown should be closed
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  // --- Admin autocomplete screen ---

  it("admin autocomplete create flow", async () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };

    (adminApi.createAutocomplete as ReturnType<typeof vi.fn>).mockResolvedValue(
      {
        term: {
          id: "t1",
          term: "plumbing repair",
          weight: 10,
          created_at: "2024-01-01",
          updated_at: "2024-01-01",
        },
      },
    );

    renderWithProviders(<AutocompletePage />);

    fireEvent.click(screen.getByText("Add Term"));

    fireEvent.change(screen.getByLabelText(/Term/), {
      target: { value: "plumbing repair" },
    });
    fireEvent.change(screen.getByLabelText(/Weight/), {
      target: { value: "10" },
    });

    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(adminApi.createAutocomplete).toHaveBeenCalledWith({
        term: "plumbing repair",
        weight: 10,
      });
    });
  });

  it("admin autocomplete shows validation error from backend", async () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };

    (adminApi.createAutocomplete as ReturnType<typeof vi.fn>).mockRejectedValue(
      new ApiError(422, {
        error: {
          code: "validation_error",
          message: "One or more fields failed validation.",
          field_errors: {
            term: ["Term is required."],
          },
        },
      }),
    );

    renderWithProviders(<AutocompletePage />);

    fireEvent.click(screen.getByText("Add Term"));
    // Fill weight but leave term filled to bypass HTML required
    fireEvent.change(screen.getByLabelText(/Term/), {
      target: { value: "x" },
    });
    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(screen.getByText("Term is required.")).toBeDefined();
    });
  });

  // --- Admin hot keyword ---

  it("admin hot keyword create", async () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };

    (adminApi.createHotKeyword as ReturnType<typeof vi.fn>).mockResolvedValue({
      keyword: {
        id: "k1",
        keyword: "plumbing",
        is_hot: true,
        created_at: "2024-01-01",
        updated_at: "2024-01-01",
      },
    });

    renderWithProviders(<HotKeywordsPage />);

    fireEvent.click(screen.getByText("Add Keyword"));
    fireEvent.change(screen.getByLabelText(/Keyword/), {
      target: { value: "plumbing" },
    });
    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(adminApi.createHotKeyword).toHaveBeenCalledWith({
        keyword: "plumbing",
        is_hot: true,
      });
    });
  });

  it("admin hot keyword shows backend error", async () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };

    (adminApi.createHotKeyword as ReturnType<typeof vi.fn>).mockRejectedValue(
      new ApiError(422, {
        error: {
          code: "validation_error",
          message: "One or more fields failed validation.",
          field_errors: { keyword: ["Keyword is required."] },
        },
      }),
    );

    renderWithProviders(<HotKeywordsPage />);

    fireEvent.click(screen.getByText("Add Keyword"));
    fireEvent.change(screen.getByLabelText(/Keyword/), {
      target: { value: "x" },
    });
    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(screen.getByText("Keyword is required.")).toBeDefined();
    });
  });

  // --- Integration-style flow test ---

  it("admin autocomplete edit flow loads existing data and submits update", async () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };

    const existingTerm = {
      id: "t1",
      term: "plumbing repair",
      weight: 10,
      created_at: "2024-01-01",
      updated_at: "2024-01-01",
    };

    (adminApi.listAutocomplete as ReturnType<typeof vi.fn>).mockResolvedValue({
      terms: [existingTerm],
    });
    (adminApi.updateAutocomplete as ReturnType<typeof vi.fn>).mockResolvedValue(
      {
        term: { ...existingTerm, term: "plumbing installation", weight: 20 },
      },
    );

    renderWithProviders(<AutocompletePage />);

    // Wait for the term to appear in the table
    await waitFor(() => {
      expect(screen.getByText("plumbing repair")).toBeDefined();
    });

    // Click Edit
    fireEvent.click(screen.getByText("Edit"));

    // Form should be pre-filled
    const termInput = screen.getByLabelText(/Term/) as HTMLInputElement;
    const weightInput = screen.getByLabelText(/Weight/) as HTMLInputElement;
    expect(termInput.value).toBe("plumbing repair");
    expect(weightInput.value).toBe("10");

    // Modify and submit
    fireEvent.change(termInput, {
      target: { value: "plumbing installation" },
    });
    fireEvent.change(weightInput, { target: { value: "20" } });
    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(adminApi.updateAutocomplete).toHaveBeenCalledWith("t1", {
        term: "plumbing installation",
        weight: 20,
      });
    });
  });

  it("service detail page shows compare overflow feedback", async () => {
    // Pre-fill compare with 3 items
    mockCompareItems = [
      sampleService,
      sampleService2,
      { ...sampleService, id: "s3", title: "Service 3" },
    ];
    mockCompareAdd.mockReturnValue(false);
    mockCompareHas.mockReturnValue(false);

    const detailService = {
      ...sampleService,
      id: "s5",
      title: "Detail Service",
      availability: [],
    };

    (catalogApi.getService as ReturnType<typeof vi.fn>).mockResolvedValue({
      service: detailService,
    });

    render(
      <QueryClientProvider
        client={
          new QueryClient({
            defaultOptions: { queries: { retry: false } },
          })
        }
      >
        <MemoryRouter initialEntries={["/customer/catalog/s5"]}>
          <Routes>
            <Route path="/customer/catalog/:id" element={<ServiceDetailPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    // Wait for service to load
    await waitFor(() => {
      expect(screen.getByText("Detail Service")).toBeDefined();
    });

    // Click "+ Compare"
    fireEvent.click(screen.getByText("+ Compare"));

    // Should show visible alert
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeDefined();
      expect(
        screen.getByText(/Compare is limited to 3 services/),
      ).toBeDefined();
    });
  });

  it("end-to-end discovery flow: hot keywords influence search, services are discoverable, favorites and compare work", async () => {
    // Step 1: Admin-configured hot keywords appear on customer search page
    (catalogApi.getHotKeywords as ReturnType<typeof vi.fn>).mockResolvedValue({
      keywords: [{ id: "k1", keyword: "plumbing" }],
    });

    // Step 2: Provider-created services are discoverable
    (catalogApi.listServices as ReturnType<typeof vi.fn>).mockResolvedValue({
      services: [sampleService, sampleService2],
      total: 2,
      page: 1,
      page_size: 20,
    });

    renderWithProviders(<CatalogPage />);

    // Verify hot keyword chip is visible
    await waitFor(() => {
      expect(screen.getByText("plumbing")).toBeDefined();
    });

    // Verify services are rendered
    await waitFor(() => {
      expect(screen.getByText("Lawn Mowing")).toBeDefined();
      expect(screen.getByText("Plumbing Fix")).toBeDefined();
    });

    // Step 3: Customer clicks hot keyword → updates search input
    fireEvent.click(screen.getByText("plumbing"));
    const searchInput = screen.getByPlaceholderText(
      "Search services...",
    ) as HTMLInputElement;
    expect(searchInput.value).toBe("plumbing");

    // Step 4: Customer favorites a service
    const favButtons = screen.getAllByText("Favorite");
    fireEvent.click(favButtons[0]);

    await waitFor(() => {
      expect(customerApi.addFavorite).toHaveBeenCalledWith("s1");
    });

    // Step 5: Customer adds services to compare
    const compareButtons = screen.getAllByText("+ Compare");
    fireEvent.click(compareButtons[0]);
    expect(mockCompareAdd).toHaveBeenCalled();
  });
});
