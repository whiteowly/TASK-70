import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { AuthUser } from "../stores/auth";

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

vi.mock("../api/catalog", () => {
  const listCategories = vi.fn().mockResolvedValue({ categories: [] });
  const createCategory = vi.fn();
  const updateCategory = vi.fn();
  const listTags = vi.fn().mockResolvedValue({ tags: [] });
  const createTag = vi.fn();
  const updateTag = vi.fn();
  const listServices = vi.fn().mockResolvedValue({ services: [] });
  const createService = vi.fn();
  const updateService = vi.fn();
  const deleteService = vi.fn();
  const setAvailability = vi.fn();
  const catListCategories = vi.fn().mockResolvedValue({ categories: [] });
  const catListTags = vi.fn().mockResolvedValue({ tags: [] });
  const catListServices = vi
    .fn()
    .mockResolvedValue({ services: [], total: 0, page: 1, page_size: 20 });
  const getService = vi.fn().mockResolvedValue({ service: null });

  return {
    adminApi: {
      listCategories,
      createCategory,
      updateCategory,
      listTags,
      createTag,
      updateTag,
      listHotKeywords: vi.fn().mockResolvedValue({ keywords: [] }),
      createHotKeyword: vi.fn(),
      updateHotKeyword: vi.fn(),
      listAutocomplete: vi.fn().mockResolvedValue({ terms: [] }),
      createAutocomplete: vi.fn(),
      updateAutocomplete: vi.fn(),
    },
    providerApi: {
      listServices,
      getService: vi.fn().mockResolvedValue({ service: null }),
      createService,
      updateService,
      deleteService,
      setAvailability,
    },
    catalogApi: {
      listCategories: catListCategories,
      listTags: catListTags,
      listServices: catListServices,
      getService,
      getTrending: vi.fn().mockResolvedValue({ services: [] }),
      getHotKeywords: vi.fn().mockResolvedValue({ keywords: [] }),
      getAutocomplete: vi.fn().mockResolvedValue({ terms: [] }),
    },
    customerApi: {
      getFavorites: vi.fn().mockResolvedValue({ favorites: [] }),
      addFavorite: vi.fn(),
      removeFavorite: vi.fn(),
      getSearchHistory: vi.fn().mockResolvedValue({ history: [] }),
    },
  };
});

import CategoriesPage from "../pages/admin/CategoriesPage";
import ServicesPage from "../pages/provider/ServicesPage";
import ServiceFormPage from "../pages/provider/ServiceFormPage";
import { adminApi, providerApi, catalogApi } from "../api/catalog";
import { ApiError } from "../api/client";

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

describe("Catalog management pages", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (adminApi.listCategories as ReturnType<typeof vi.fn>).mockResolvedValue({
      categories: [],
    });
    (adminApi.createCategory as ReturnType<typeof vi.fn>).mockResolvedValue({
      category: {
        id: "1",
        name: "Test",
        slug: "test",
        parent_id: null,
        sort_order: 0,
        created_at: "2024-01-01",
      },
    });
    (providerApi.listServices as ReturnType<typeof vi.fn>).mockResolvedValue({
      services: [],
    });
  });

  it("admin can create a category", async () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };

    renderWithProviders(<CategoriesPage />);

    fireEvent.click(screen.getByText("Add Category"));

    fireEvent.change(screen.getByLabelText(/Name/), {
      target: { value: "Plumbing" },
    });
    fireEvent.change(screen.getByLabelText(/Slug/), {
      target: { value: "plumbing" },
    });

    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(adminApi.createCategory).toHaveBeenCalledWith({
        name: "Plumbing",
        slug: "plumbing",
        parent_id: null,
        sort_order: 0,
      });
    });
  });

  it("provider can see service list", async () => {
    mockAuthStore.user = {
      id: "3",
      username: "provider",
      email: "p@test.com",
      roles: ["provider"],
    };

    (providerApi.listServices as ReturnType<typeof vi.fn>).mockResolvedValue({
      services: [
        {
          id: "s1",
          title: "Drain Repair",
          description: null,
          price_cents: 5000,
          rating_avg: "4.5",
          popularity_score: 10,
          status: "active",
          category: { id: "c1", name: "Plumbing" },
          provider: { id: "p1", business_name: "Joe's" },
          tags: [],
          created_at: "2024-01-01",
          updated_at: "2024-01-01",
        },
      ],
    });

    renderWithProviders(<ServicesPage />);

    await waitFor(() => {
      expect(screen.getByText("Drain Repair")).toBeDefined();
    });
  });

  it("validation error shown on empty category name", async () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };

    renderWithProviders(<CategoriesPage />);

    fireEvent.click(screen.getByText("Add Category"));

    const nameInput = screen.getByLabelText(/Name/) as HTMLInputElement;
    const slugInput = screen.getByLabelText(/Slug/) as HTMLInputElement;
    expect(nameInput.required).toBe(true);
    expect(slugInput.required).toBe(true);
  });

  it("renders inline field errors from backend-shaped validation response on category form", async () => {
    mockAuthStore.user = {
      id: "2",
      username: "admin",
      email: "a@test.com",
      roles: ["administrator"],
    };

    // Mock createCategory to reject with backend-shaped field_errors (object map)
    (adminApi.createCategory as ReturnType<typeof vi.fn>).mockRejectedValue(
      new ApiError(422, {
        error: {
          code: "validation_error",
          message: "One or more fields failed validation.",
          field_errors: {
            name: ["Name is required."],
            slug: ["Slug is required.", "Slug must be lowercase."],
          },
        },
      }),
    );

    renderWithProviders(<CategoriesPage />);

    fireEvent.click(screen.getByText("Add Category"));

    // Fill both fields to bypass HTML required validation
    fireEvent.change(screen.getByLabelText(/Name/), {
      target: { value: "x" },
    });
    fireEvent.change(screen.getByLabelText(/Slug/), {
      target: { value: "x" },
    });
    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      // Field error for name should be rendered inline
      expect(screen.getByText("Name is required.")).toBeDefined();
      // Slug has two messages — they should be joined
      expect(
        screen.getByText("Slug is required. Slug must be lowercase."),
      ).toBeDefined();
    });

    // The top-level API error message should also appear
    expect(
      screen.getByText("One or more fields failed validation."),
    ).toBeDefined();
  });

  it("renders inline field errors from backend-shaped validation response on service form", async () => {
    mockAuthStore.user = {
      id: "3",
      username: "provider",
      email: "p@test.com",
      roles: ["provider"],
    };

    // Provide categories and tags for the form dropdowns
    (catalogApi.listCategories as ReturnType<typeof vi.fn>).mockResolvedValue({
      categories: [
        { id: "c1", name: "Plumbing", slug: "plumbing", parent_id: null, sort_order: 0, created_at: "2024-01-01" },
      ],
    });
    (catalogApi.listTags as ReturnType<typeof vi.fn>).mockResolvedValue({
      tags: [],
    });

    // Mock createService to reject with backend-shaped field_errors
    (providerApi.createService as ReturnType<typeof vi.fn>).mockRejectedValue(
      new ApiError(422, {
        error: {
          code: "validation_error",
          message: "One or more fields failed validation.",
          field_errors: {
            title: ["Title is required."],
            price_cents: ["Price must be non-negative."],
          },
        },
      }),
    );

    renderWithProviders(<ServiceFormPage />);

    // Wait for categories to load
    await waitFor(() => {
      expect(screen.getByText("Plumbing")).toBeDefined();
    });

    // Fill in required fields to bypass HTML validation
    fireEvent.change(screen.getByLabelText(/Title/), {
      target: { value: "Test" },
    });
    fireEvent.change(screen.getByLabelText(/Price/), {
      target: { value: "50" },
    });
    fireEvent.change(screen.getByLabelText(/Category/), {
      target: { value: "c1" },
    });

    fireEvent.click(screen.getByText("Save"));

    await waitFor(() => {
      expect(screen.getByText("Title is required.")).toBeDefined();
      expect(screen.getByText("Price must be non-negative.")).toBeDefined();
    });

    expect(
      screen.getByText("One or more fields failed validation."),
    ).toBeDefined();
  });
});
