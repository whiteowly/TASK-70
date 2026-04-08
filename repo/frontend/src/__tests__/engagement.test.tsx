import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { AuthUser } from "../stores/auth";

// ---------- Auth store mock ----------

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
      if (typeof selector === "function")
        return (selector as (s: typeof state) => unknown)(state);
      return state;
    }),
    MAX_COMPARE: 3,
  };
});

// ---------- Engagement API mock ----------

vi.mock("../api/engagement", () => ({
  interestApi: {
    customerSubmit: vi.fn(),
    customerList: vi.fn().mockResolvedValue({ interests: [] }),
    customerGet: vi.fn().mockResolvedValue({ interest: null, events: [] }),
    customerWithdraw: vi.fn(),
    providerList: vi.fn().mockResolvedValue({ interests: [] }),
    providerAccept: vi.fn().mockResolvedValue({ message: "Interest accepted." }),
    providerDecline: vi.fn().mockResolvedValue({ message: "Interest declined." }),
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
    customerBlock: vi.fn().mockResolvedValue({ message: "User blocked." }),
    customerUnblock: vi.fn().mockResolvedValue({ message: "Block removed." }),
    providerBlock: vi.fn().mockResolvedValue({ message: "User blocked." }),
    providerUnblock: vi.fn().mockResolvedValue({ message: "Block removed." }),
  },
}));

// ---------- Catalog API mock ----------

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
  customerApi: {
    getFavorites: vi.fn().mockResolvedValue({ favorites: [] }),
    addFavorite: vi.fn(),
    removeFavorite: vi.fn(),
    getSearchHistory: vi.fn().mockResolvedValue({ history: [] }),
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
}));

// ---------- Imports (after mocks) ----------

import CustomerInterestsPage from "../pages/customer/InterestsPage";
import CustomerThreadsPage from "../pages/customer/ThreadsPage";
import CustomerThreadDetailPage from "../pages/customer/ThreadDetailPage";
import ProviderInterestsPage from "../pages/provider/InterestsPage";
import ServiceDetailPage from "../pages/customer/ServiceDetailPage";
import { interestApi, messageApi } from "../api/engagement";
import { catalogApi } from "../api/catalog";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const fn = (f: any) => f as ReturnType<typeof vi.fn>;
import { ApiError } from "../api/client";

// ---------- Helpers ----------

function renderWithProviders(
  ui: React.ReactElement,
  { route = "/" }: { route?: string } = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[route]}>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

function withRoute(element: React.ReactElement, path: string, route: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[route]}>
        <Routes>
          <Route path={path} element={element} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const customerUser: AuthUser = {
  id: "u1",
  username: "customer",
  email: "c@test.com",
  roles: ["customer"],
};

const providerUser: AuthUser = {
  id: "u2",
  username: "provider",
  email: "p@test.com",
  roles: ["provider"],
};

// ---------- Tests ----------

describe("Engagement — Customer Interests", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthStore.user = customerUser;
    fn(interestApi.customerList).mockResolvedValue({ interests: [] });
  });

  it("customer interest list renders", async () => {
    fn(interestApi.customerList).mockResolvedValue({
      interests: [
        {
          id: "int-001",
          customer_id: "u1",
          provider_id: "p1",
          service_id: "s1",
          status: "submitted",
          created_at: "2025-01-01T00:00:00Z",
          updated_at: "2025-01-01T00:00:00Z",
        },
      ],
    });

    renderWithProviders(<CustomerInterestsPage />);

    await waitFor(() => {
      expect(screen.getByText("submitted")).toBeDefined();
    });

    // Status badge with blue color
    const badge = screen.getByTestId("status-badge-int-001");
    expect(badge.textContent).toBe("submitted");
    expect(badge.className).toContain("bg-blue-100");
  });

  it("duplicate interest shows inline error", async () => {
    fn(interestApi.customerSubmit).mockRejectedValue(
      new ApiError(409, {
        error: {
          code: "duplicate_interest",
          message: "Duplicate interest.",
          field_errors: {
            provider_id: [
              "Only one active interest is allowed within 7 days.",
            ],
          },
        },
      }),
    );

    fn(catalogApi.getService).mockResolvedValue({
      service: {
        id: "s1",
        title: "Plumbing Fix",
        description: null,
        price_cents: 5000,
        rating_avg: "4.5",
        popularity_score: 10,
        status: "active",
        category: { id: "c1", name: "Plumbing" },
        provider: { id: "p1", business_name: "Joe's", service_area_miles: null },
        tags: [],
        availability: [],
        created_at: "2024-01-01",
        updated_at: "2024-01-01",
      },
    });

    withRoute(
      <ServiceDetailPage />,
      "/customer/catalog/:id",
      "/customer/catalog/s1",
    );

    await waitFor(() => {
      expect(screen.getByText("Plumbing Fix")).toBeDefined();
    });

    fireEvent.click(screen.getByText("Submit Interest"));

    await waitFor(() => {
      expect(
        screen.getByText(
          "Only one active interest is allowed within 7 days.",
        ),
      ).toBeDefined();
    });
  });
});

describe("Engagement — Provider Interests", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthStore.user = providerUser;
  });

  it("provider accept/decline buttons work", async () => {
    fn(interestApi.providerList).mockResolvedValue({
      interests: [
        {
          id: "int-002",
          customer_id: "c1",
          provider_id: "u2",
          service_id: "s1",
          status: "submitted",
          created_at: "2025-01-01T00:00:00Z",
          updated_at: "2025-01-01T00:00:00Z",
        },
      ],
    });

    renderWithProviders(<ProviderInterestsPage />);

    await waitFor(() => {
      expect(screen.getByText("Accept")).toBeDefined();
      expect(screen.getByText("Decline")).toBeDefined();
    });

    fireEvent.click(screen.getByText("Accept"));

    await waitFor(() => {
      expect(interestApi.providerAccept).toHaveBeenCalledWith("int-002");
    });
  });
});

describe("Engagement — Customer Threads", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthStore.user = customerUser;
  });

  it("customer thread list renders", async () => {
    fn(messageApi.customerThreads).mockResolvedValue({
      threads: [
        {
          thread_id: "t1",
          other_user_id: "p1",
          other_name: "Joe's Plumbing",
          last_message: "We can schedule for Monday",
          last_at: "2025-01-15T12:00:00Z",
          unread_count: 3,
        },
      ],
    });

    renderWithProviders(<CustomerThreadsPage />);

    await waitFor(() => {
      expect(screen.getByText("Joe's Plumbing")).toBeDefined();
      expect(screen.getByText("We can schedule for Monday")).toBeDefined();
    });

    // Unread badge
    const badge = screen.getByTestId("unread-t1");
    expect(badge.textContent).toBe("3");
  });

  it("customer thread detail shows messages", async () => {
    fn(messageApi.customerThread).mockResolvedValue({
      messages: [
        {
          id: "m1",
          thread_id: "t1",
          sender_id: "p1",
          recipient_id: "u1",
          body: "Hello, how can I help?",
          created_at: "2025-01-15T10:00:00Z",
          read_status: "read",
        },
        {
          id: "m2",
          thread_id: "t1",
          sender_id: "u1",
          recipient_id: "p1",
          body: "I need drain repair",
          created_at: "2025-01-15T10:05:00Z",
          read_status: "sent",
        },
      ],
    });

    withRoute(
      <CustomerThreadDetailPage />,
      "/customer/messages/:threadId",
      "/customer/messages/t1",
    );

    await waitFor(() => {
      expect(screen.getByText("Hello, how can I help?")).toBeDefined();
      expect(screen.getByText("I need drain repair")).toBeDefined();
    });

    // "You" label for the user's own message
    expect(screen.getByText("You")).toBeDefined();
  });

  it("read receipt indicators render", async () => {
    fn(messageApi.customerThread).mockResolvedValue({
      messages: [
        {
          id: "m1",
          thread_id: "t1",
          sender_id: "u1",
          recipient_id: "p1",
          body: "Message one",
          created_at: "2025-01-15T10:00:00Z",
          read_status: "read",
        },
        {
          id: "m2",
          thread_id: "t1",
          sender_id: "u1",
          recipient_id: "p1",
          body: "Message two",
          created_at: "2025-01-15T10:05:00Z",
          read_status: "sent",
        },
        {
          id: "m3",
          thread_id: "t1",
          sender_id: "u1",
          recipient_id: "p1",
          body: "Message three",
          created_at: "2025-01-15T10:10:00Z",
          read_status: "delivered",
        },
      ],
    });

    withRoute(
      <CustomerThreadDetailPage />,
      "/customer/messages/:threadId",
      "/customer/messages/t1",
    );

    await waitFor(() => {
      expect(screen.getByTestId("receipt-m1").textContent).toBe("Read");
      expect(screen.getByTestId("receipt-m2").textContent).toBe("Sent");
      expect(screen.getByTestId("receipt-m3").textContent).toBe("Delivered");
    });
  });

  it("message send blocked shows feedback", async () => {
    fn(messageApi.customerThread).mockResolvedValue({ messages: [] });
    fn(messageApi.customerSend).mockRejectedValue(
      new ApiError(403, {
        error: {
          code: "forbidden",
          message: "You are blocked by this user.",
        },
      }),
    );

    withRoute(
      <CustomerThreadDetailPage />,
      "/customer/messages/:threadId",
      "/customer/messages/t1",
    );

    await waitFor(() => {
      expect(screen.getByPlaceholderText("Type a message...")).toBeDefined();
    });

    fireEvent.change(screen.getByPlaceholderText("Type a message..."), {
      target: { value: "Hello" },
    });
    fireEvent.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText("Cannot send — blocked")).toBeDefined();
    });
  });
});

describe("Engagement — Blocked state on service detail", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthStore.user = customerUser;
  });

  it("blocked state shows on service detail", async () => {
    fn(interestApi.customerSubmit).mockRejectedValue(
      new ApiError(403, {
        error: {
          code: "forbidden",
          message: "You are blocked by this provider.",
        },
      }),
    );

    fn(catalogApi.getService).mockResolvedValue({
      service: {
        id: "s2",
        title: "Lawn Mowing",
        description: null,
        price_cents: 3000,
        rating_avg: "4.0",
        popularity_score: 5,
        status: "active",
        category: { id: "c2", name: "Landscaping" },
        provider: { id: "p2", business_name: "Green Co", service_area_miles: null },
        tags: [],
        availability: [],
        created_at: "2024-01-01",
        updated_at: "2024-01-01",
      },
    });

    withRoute(
      <ServiceDetailPage />,
      "/customer/catalog/:id",
      "/customer/catalog/s2",
    );

    await waitFor(() => {
      expect(screen.getByText("Lawn Mowing")).toBeDefined();
    });

    fireEvent.click(screen.getByText("Submit Interest"));

    await waitFor(() => {
      expect(screen.getAllByText("Blocked").length).toBeGreaterThan(0);
    });

    // Interest button should be disabled after blocked
    const btn = screen.getByText("Submit Interest") as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
  });

  it("blocked provider service detail returns not found from API", async () => {
    fn(catalogApi.getService).mockRejectedValue(
      new ApiError(404, {
        error: {
          code: "not_found",
          message: "Service not found.",
        },
      }),
    );

    withRoute(
      <ServiceDetailPage />,
      "/customer/catalog/:id",
      "/customer/catalog/s-blocked",
    );

    await waitFor(() => {
      expect(screen.getAllByText("Service not found.").length).toBeGreaterThan(0);
    });
  });
});

describe("Engagement — Full engagement workflow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthStore.user = customerUser;
  });

  it("full engagement workflow", async () => {
    // Step 1: Submit interest
    fn(interestApi.customerSubmit).mockResolvedValue({
      interest: {
        id: "int-100",
        customer_id: "u1",
        provider_id: "p1",
        service_id: "s1",
        status: "submitted",
        created_at: "2025-02-01T00:00:00Z",
        updated_at: "2025-02-01T00:00:00Z",
      },
    });

    fn(catalogApi.getService).mockResolvedValue({
      service: {
        id: "s1",
        title: "Test Service",
        description: null,
        price_cents: 1000,
        rating_avg: "5.0",
        popularity_score: 1,
        status: "active",
        category: null,
        provider: { id: "p1", business_name: "Test Biz", service_area_miles: null },
        tags: [],
        availability: [],
        created_at: "2024-01-01",
        updated_at: "2024-01-01",
      },
    });

    const { unmount } = withRoute(
      <ServiceDetailPage />,
      "/customer/catalog/:id",
      "/customer/catalog/s1",
    );

    await waitFor(() => {
      expect(screen.getByText("Test Service")).toBeDefined();
    });

    fireEvent.click(screen.getByText("Submit Interest"));

    await waitFor(() => {
      expect(interestApi.customerSubmit).toHaveBeenCalled();
      expect(screen.getByText("Interest submitted successfully!")).toBeDefined();
    });

    unmount();

    // Step 2: See it in list
    fn(interestApi.customerList).mockResolvedValue({
      interests: [
        {
          id: "int-100",
          customer_id: "u1",
          provider_id: "p1",
          service_id: "s1",
          status: "submitted",
          created_at: "2025-02-01T00:00:00Z",
          updated_at: "2025-02-01T00:00:00Z",
        },
      ],
    });

    const { unmount: unmount2 } = renderWithProviders(
      <CustomerInterestsPage />,
    );

    await waitFor(() => {
      expect(screen.getByText("submitted")).toBeDefined();
    });

    unmount2();

    // Step 3: Provider accepts
    mockAuthStore.user = providerUser;
    fn(interestApi.providerList).mockResolvedValue({
      interests: [
        {
          id: "int-100",
          customer_id: "u1",
          provider_id: "u2",
          service_id: "s1",
          status: "submitted",
          created_at: "2025-02-01T00:00:00Z",
          updated_at: "2025-02-01T00:00:00Z",
        },
      ],
    });

    const { unmount: unmount3 } = renderWithProviders(
      <ProviderInterestsPage />,
    );

    await waitFor(() => {
      expect(screen.getByText("Accept")).toBeDefined();
    });

    fireEvent.click(screen.getByText("Accept"));

    await waitFor(() => {
      expect(interestApi.providerAccept).toHaveBeenCalledWith("int-100");
    });

    unmount3();

    // Step 4: Customer sends a message
    mockAuthStore.user = customerUser;
    fn(messageApi.customerThread).mockResolvedValue({ messages: [] });
    fn(messageApi.customerSend).mockResolvedValue({
      message: {
        id: "m100",
        thread_id: "int-100",
        sender_id: "u1",
        recipient_id: "p1",
        body: "When can you come?",
        created_at: "2025-02-02T00:00:00Z",
        read_status: "sent",
      },
    });

    const { unmount: unmount4 } = withRoute(
      <CustomerThreadDetailPage />,
      "/customer/messages/:threadId",
      "/customer/messages/int-100",
    );

    await waitFor(() => {
      expect(screen.getByPlaceholderText("Type a message...")).toBeDefined();
    });

    fireEvent.change(screen.getByPlaceholderText("Type a message..."), {
      target: { value: "When can you come?" },
    });
    fireEvent.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(messageApi.customerSend).toHaveBeenCalled();
      const callArgs = fn(messageApi.customerSend).mock.calls[0];
      expect(callArgs[0]).toBe("int-100");
      expect(callArgs[1]).toBe("When can you come?");
    });

    // Step 5: Mark read was called on mount
    expect(messageApi.customerMarkRead).toHaveBeenCalledWith("int-100");

    unmount4();
  });
});
