import { create } from "zustand";
import { api } from "../api/client";

export interface AuthUser {
  id: string;
  username: string;
  email: string;
  roles: string[];
}

interface AuthState {
  user: AuthUser | null;
  loading: boolean;
  error: string | null;

  bootstrap: () => Promise<void>;
  login: (username: string, password: string) => Promise<AuthUser>;
  logout: () => Promise<void>;
}

export const useAuthStore = create<AuthState>()((set) => ({
  user: null,
  loading: true,
  error: null,

  bootstrap: async () => {
    try {
      const data = await api.get<{ user: AuthUser }>("/auth/me");
      set({ user: data.user, loading: false, error: null });
    } catch (err) {
      set({ user: null, loading: false, error: null });
    }
  },

  login: async (username: string, password: string) => {
    const data = await api.post<{ user: AuthUser }>("/auth/login", {
      username,
      password,
    });
    set({ user: data.user, error: null });
    return data.user;
  },

  logout: async () => {
    try {
      await api.post("/auth/logout");
    } finally {
      set({ user: null });
    }
  },
}));

export function hasRole(user: AuthUser | null, role: string): boolean {
  return user?.roles.includes(role) ?? false;
}

export function primaryRole(user: AuthUser | null): string | null {
  if (!user) return null;
  if (user.roles.includes("administrator")) return "administrator";
  if (user.roles.includes("provider")) return "provider";
  if (user.roles.includes("customer")) return "customer";
  return null;
}

export function roleHomePath(role: string | null): string {
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
}
