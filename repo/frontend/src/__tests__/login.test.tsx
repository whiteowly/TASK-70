import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { AuthUser } from "../stores/auth";
import { ApiError } from "../api/client";

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

import LoginPage from "../pages/LoginPage";

function renderLogin() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/login"]}>
        <LoginPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("LoginPage", () => {
  beforeEach(() => {
    mockAuthStore.user = null;
    mockAuthStore.loading = false;
    mockAuthStore.error = null;
    mockAuthStore.bootstrap.mockReset();
    mockAuthStore.login.mockReset();
    mockAuthStore.logout.mockReset();
  });

  it("shows error on failed login", async () => {
    mockAuthStore.login.mockRejectedValue(
      new ApiError(401, {
        error: {
          code: "invalid_credentials",
          message: "Invalid username or password.",
        },
      }),
    );

    renderLogin();

    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "baduser" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "badpass" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign In" }));

    await waitFor(() => {
      expect(screen.getByText("Invalid username or password.")).toBeDefined();
    });
  });

  it("redirects on successful login", async () => {
    const loggedInUser: AuthUser = {
      id: "1",
      username: "customer",
      email: "c@test.com",
      roles: ["customer"],
    };

    mockAuthStore.login.mockResolvedValue(loggedInUser);

    renderLogin();

    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "customer" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "pass123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign In" }));

    await waitFor(() => {
      expect(mockAuthStore.login).toHaveBeenCalledWith("customer", "pass123");
    });
  });
});
