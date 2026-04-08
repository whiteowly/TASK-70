import { api } from "./client";

export interface Category {
  id: string;
  parent_id: string | null;
  name: string;
  slug: string;
  sort_order: number;
  created_at: string;
}

export interface Tag {
  id: string;
  name: string;
  created_at: string;
}

export interface ServiceSummary {
  id: string;
  title: string;
  description: string | null;
  price_cents: number;
  rating_avg: string;
  popularity_score: number;
  status: string;
  category: { id: string; name: string } | null;
  provider: { id: string; business_name: string; service_area_miles: number | null };
  tags: { id: string; name: string }[];
  created_at: string;
  updated_at: string;
}

export interface ServiceDetail extends ServiceSummary {
  availability: {
    id: string;
    day_of_week: number;
    start_time: string;
    end_time: string;
  }[];
}

// Catalog reads
export const catalogApi = {
  listCategories: () =>
    api.get<{ categories: Category[] }>("/catalog/categories"),
  listTags: () => api.get<{ tags: Tag[] }>("/catalog/tags"),
  listServices: (params?: {
    q?: string;
    category_id?: string;
    tag_ids?: string[];
    min_price?: number;
    max_price?: number;
    min_rating?: number;
    radius_miles?: number;
    available_date?: string;
    available_time?: string;
    sort?: string;
    page?: number;
    page_size?: number;
  }) => {
    const qs = new URLSearchParams();
    if (params?.q) qs.set("q", params.q);
    if (params?.category_id) qs.set("category_id", params.category_id);
    if (params?.tag_ids?.length) qs.set("tag_ids", params.tag_ids.join(","));
    if (params?.min_price != null) qs.set("min_price", String(params.min_price));
    if (params?.max_price != null) qs.set("max_price", String(params.max_price));
    if (params?.min_rating != null) qs.set("min_rating", String(params.min_rating));
    if (params?.radius_miles != null) qs.set("radius_miles", String(params.radius_miles));
    if (params?.available_date) qs.set("available_date", params.available_date);
    if (params?.available_time) qs.set("available_time", params.available_time);
    if (params?.sort) qs.set("sort", params.sort);
    if (params?.page) qs.set("page", String(params.page));
    if (params?.page_size) qs.set("page_size", String(params.page_size));
    const q = qs.toString();
    return api.get<{
      services: ServiceSummary[];
      total: number;
      page: number;
      page_size: number;
    }>(`/catalog/services${q ? "?" + q : ""}`);
  },
  getService: (id: string) =>
    api.get<{ service: ServiceDetail }>(`/catalog/services/${id}`),
  getTrending: () =>
    api.get<{ services: ServiceSummary[] }>("/catalog/trending"),
  getHotKeywords: () =>
    api.get<{ keywords: { id: string; keyword: string }[] }>("/catalog/hot-keywords"),
  getAutocomplete: (q?: string) =>
    api.get<{ terms: { id: string; term: string; weight: number }[] }>(
      `/catalog/autocomplete${q ? "?q=" + encodeURIComponent(q) : ""}`,
    ),
};

// Customer APIs
export const customerApi = {
  getFavorites: () =>
    api.get<{ favorites: { id: string; service_id: string; created_at: string }[] }>("/customer/favorites"),
  addFavorite: (serviceId: string) =>
    api.post<{ favorite: { id: string; service_id: string; created_at: string } }>(`/customer/favorites/${serviceId}`),
  removeFavorite: (serviceId: string) =>
    api.delete<{ message: string }>(`/customer/favorites/${serviceId}`),
  getSearchHistory: () =>
    api.get<{ history: { id: string; query_text: string; created_at: string }[] }>("/customer/search-history"),
};

// Admin taxonomy
export const adminApi = {
  listCategories: () =>
    api.get<{ categories: Category[] }>("/admin/categories"),
  createCategory: (data: {
    name: string;
    slug: string;
    parent_id?: string | null;
    sort_order?: number;
  }) => api.post<{ category: Category }>("/admin/categories", data),
  updateCategory: (
    id: string,
    data: Partial<{
      name: string;
      slug: string;
      parent_id: string | null;
      sort_order: number;
    }>,
  ) => api.patch<{ category: Category }>(`/admin/categories/${id}`, data),
  listTags: () => api.get<{ tags: Tag[] }>("/admin/tags"),
  createTag: (data: { name: string }) =>
    api.post<{ tag: Tag }>("/admin/tags", data),
  updateTag: (id: string, data: { name: string }) =>
    api.patch<{ tag: Tag }>(`/admin/tags/${id}`, data),
  listHotKeywords: () =>
    api.get<{ keywords: { id: string; keyword: string; is_hot: boolean; created_at: string; updated_at: string }[] }>("/admin/search-config/hot-keywords"),
  createHotKeyword: (data: { keyword: string; is_hot: boolean }) =>
    api.post<{ keyword: any }>("/admin/search-config/hot-keywords", data),
  updateHotKeyword: (id: string, data: { keyword?: string; is_hot?: boolean }) =>
    api.patch<{ keyword: any }>(`/admin/search-config/hot-keywords/${id}`, data),
  listAutocomplete: () =>
    api.get<{ terms: { id: string; term: string; weight: number; created_at: string; updated_at: string }[] }>("/admin/search-config/autocomplete"),
  createAutocomplete: (data: { term: string; weight: number }) =>
    api.post<{ term: any }>("/admin/search-config/autocomplete", data),
  updateAutocomplete: (id: string, data: { term?: string; weight?: number }) =>
    api.patch<{ term: any }>(`/admin/search-config/autocomplete/${id}`, data),
};

// Provider services
export const providerApi = {
  listServices: () =>
    api.get<{ services: ServiceSummary[] }>("/provider/services"),
  getService: (id: string) =>
    api.get<{ service: ServiceDetail }>(`/provider/services/${id}`),
  createService: (data: {
    category_id: string;
    title: string;
    description?: string;
    price_cents: number;
    tag_ids?: string[];
    status?: string;
  }) => api.post<{ service: ServiceSummary }>("/provider/services", data),
  updateService: (
    id: string,
    data: Partial<{
      category_id: string;
      title: string;
      description: string;
      price_cents: number;
      tag_ids: string[];
      status: string;
    }>,
  ) => api.patch<{ service: ServiceSummary }>(`/provider/services/${id}`, data),
  deleteService: (id: string) =>
    api.delete<{ message: string }>(`/provider/services/${id}`),
  setAvailability: (
    id: string,
    windows: { day_of_week: number; start_time: string; end_time: string }[],
  ) =>
    api.post<{
      availability: {
        id: string;
        day_of_week: number;
        start_time: string;
        end_time: string;
      }[];
    }>(`/provider/services/${id}/availability`, { windows }),
};
