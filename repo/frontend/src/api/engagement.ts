import { api, ApiError, ApiErrorBody } from "./client";

/* ---------- Types ---------- */

export interface Interest {
  id: string;
  customer_id: string;
  provider_id: string;
  service_id: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface StatusEvent {
  id: string;
  old_status: string | null;
  new_status: string;
  created_at: string;
}

export interface Thread {
  thread_id: string;
  other_user_id: string;
  other_name: string;
  last_message: string;
  last_at: string;
  unread_count: number;
}

export interface ChatMessage {
  id: string;
  thread_id: string;
  sender_id: string;
  recipient_id: string;
  body: string;
  created_at: string;
  read_status: string;
}

/* ---------- Idempotent POST helper ---------- */

async function postIdempotent<T>(
  path: string,
  body: unknown,
  key?: string,
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (key) headers["Idempotency-Key"] = key;

  const res = await fetch(`/api/v1${path}`, {
    method: "POST",
    credentials: "include",
    headers,
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let errBody: ApiErrorBody;
    try {
      errBody = await res.json();
    } catch {
      errBody = {
        error: { code: "UNKNOWN", message: res.statusText },
      };
    }
    throw new ApiError(res.status, errBody);
  }

  if (res.status === 204) return undefined as T;
  return res.json();
}

/* ---------- Interest API ---------- */

export const interestApi = {
  customerSubmit: (
    data: { service_id: string; provider_id: string },
    key?: string,
  ) =>
    postIdempotent<{ interest: Interest }>(
      "/customer/interests",
      data,
      key,
    ),

  customerList: () =>
    api.get<{ interests: Interest[] }>("/customer/interests"),

  customerGet: (id: string) =>
    api.get<{ interest: Interest; events: StatusEvent[] }>(
      `/customer/interests/${id}`,
    ),

  customerWithdraw: (id: string) =>
    api.post<{ message: string }>(`/customer/interests/${id}/withdraw`),

  providerList: () =>
    api.get<{ interests: Interest[] }>("/provider/interests"),

  providerAccept: (id: string) =>
    api.post<{ message: string }>(`/provider/interests/${id}/accept`),

  providerDecline: (id: string) =>
    api.post<{ message: string }>(`/provider/interests/${id}/decline`),
};

/* ---------- Message API ---------- */

export const messageApi = {
  customerThreads: () =>
    api.get<{ threads: Thread[] }>("/customer/messages"),

  customerThread: (id: string) =>
    api.get<{ messages: ChatMessage[] }>(`/customer/messages/${id}`),

  customerSend: (threadId: string, body: string, key?: string) =>
    postIdempotent<{ message: ChatMessage }>(
      `/customer/messages/${threadId}`,
      { body },
      key,
    ),

  customerMarkRead: (threadId: string) =>
    api.post<{ message: string }>(`/customer/messages/${threadId}/read`),

  providerThreads: () =>
    api.get<{ threads: Thread[] }>("/provider/messages"),

  providerThread: (id: string) =>
    api.get<{ messages: ChatMessage[] }>(`/provider/messages/${id}`),

  providerSend: (threadId: string, body: string, key?: string) =>
    postIdempotent<{ message: ChatMessage }>(
      `/provider/messages/${threadId}`,
      { body },
      key,
    ),

  providerMarkRead: (threadId: string) =>
    api.post<{ message: string }>(`/provider/messages/${threadId}/read`),
};

/* ---------- Block API ---------- */

export const blockApi = {
  customerBlock: (providerId: string) =>
    api.post<{ message: string }>(`/customer/blocks/${providerId}`),

  customerUnblock: (providerId: string) =>
    api.delete<{ message: string }>(`/customer/blocks/${providerId}`),

  providerBlock: (customerId: string) =>
    api.post<{ message: string }>(`/provider/blocks/${customerId}`),

  providerUnblock: (customerId: string) =>
    api.delete<{ message: string }>(`/provider/blocks/${customerId}`),
};
