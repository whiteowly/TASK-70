import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { ServiceSummary } from "../api/catalog";

interface CompareState {
  items: ServiceSummary[];
  add: (service: ServiceSummary) => boolean;
  remove: (serviceId: string) => void;
  clear: () => void;
  has: (serviceId: string) => boolean;
}

export const MAX_COMPARE = 3;

export const useCompareStore = create<CompareState>()(
  persist(
    (set, get) => ({
      items: [],
      add: (service) => {
        if (get().items.length >= MAX_COMPARE) return false;
        if (get().items.some((s) => s.id === service.id)) return true;
        set((state) => ({ items: [...state.items, service] }));
        return true;
      },
      remove: (serviceId) =>
        set((state) => ({
          items: state.items.filter((s) => s.id !== serviceId),
        })),
      clear: () => set({ items: [] }),
      has: (serviceId) => get().items.some((s) => s.id === serviceId),
    }),
    { name: "fieldserve-compare" },
  ),
);
