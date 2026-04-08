import { api, ApiError } from "./client";

export interface ProviderDocument {
  id: string;
  provider_id: string;
  filename: string;
  mime_type: string;
  size_bytes: number;
  checksum_sha256: string;
  storage_path: string;
  created_at: string;
}

export interface GrowthMetric {
  date: string;
  value: number;
  label: string;
}

export interface ConversionMetric {
  date: string;
  searches: number;
  interests: number;
  rate: number;
}

export interface UtilizationMetric {
  id: string;
  business_name: string;
  active_services: number;
  total_interests: number;
  messages_sent: number;
}

export interface ExportJob {
  id: string;
  admin_id: string;
  export_type: string;
  status: string;
  file_path: string | null;
  created_at: string;
  completed_at: string | null;
}

export const documentsApi = {
  list: () =>
    api.get<{ documents: ProviderDocument[] }>("/provider/documents"),

  upload: async (file: File): Promise<{ document: ProviderDocument }> => {
    const fd = new FormData();
    fd.append("file", file);
    const res = await fetch("/api/v1/provider/documents", {
      method: "POST",
      credentials: "include",
      body: fd,
    });
    if (!res.ok) {
      const b = await res.json().catch(() => ({
        error: { code: "UNKNOWN", message: res.statusText },
      }));
      throw new ApiError(res.status, b);
    }
    return res.json();
  },

  delete: (id: string) =>
    api.delete<{ message: string }>(`/provider/documents/${id}`),
};

export const analyticsApi = {
  userGrowth: (from?: string, to?: string) => {
    const qs = new URLSearchParams();
    if (from) qs.set("from", from);
    if (to) qs.set("to", to);
    return api.get<{ metrics: GrowthMetric[] }>(
      `/admin/analytics/user-growth${qs.toString() ? "?" + qs : ""}`,
    );
  },

  conversion: (from?: string, to?: string) => {
    const qs = new URLSearchParams();
    if (from) qs.set("from", from);
    if (to) qs.set("to", to);
    return api.get<{ metrics: ConversionMetric[] }>(
      `/admin/analytics/conversion${qs.toString() ? "?" + qs : ""}`,
    );
  },

  providerUtilization: () =>
    api.get<{ providers: UtilizationMetric[] }>(
      "/admin/analytics/provider-utilization",
    ),
};

export const exportsApi = {
  create: (data: { export_type: string; from?: string; to?: string }) =>
    api.post<{ export: ExportJob }>("/admin/exports", data),

  list: () => api.get<{ exports: ExportJob[] }>("/admin/exports"),

  get: (id: string) =>
    api.get<{ export: ExportJob }>(`/admin/exports/${id}`),

  downloadUrl: (id: string) => `/api/v1/admin/exports/${id}/download`,
};
