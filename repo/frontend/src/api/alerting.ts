import { api, ApiError } from "./client";

export interface AlertRule {
  id: string;
  name: string;
  condition: any;
  severity: string;
  quiet_hours_start: string | null;
  quiet_hours_end: string | null;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface Alert {
  id: string;
  rule_id: string;
  rule_name?: string;
  severity: string;
  status: string;
  data: any;
  created_at: string;
  resolved_at: string | null;
}

export interface AlertAssignment {
  id: string;
  alert_id: string;
  assignee_id: string;
  assigned_at: string;
  acknowledged_at: string | null;
}

export interface WorkOrder {
  id: string;
  alert_id: string | null;
  status: string;
  assigned_to: string | null;
  created_at: string;
  updated_at: string;
}

export interface WorkOrderEvent {
  id: string;
  work_order_id: string;
  old_status: string | null;
  new_status: string;
  actor_id: string | null;
  created_at: string;
}

export interface WOEvidence {
  id: string;
  work_order_id: string;
  file_path: string;
  uploaded_by: string | null;
  created_at: string;
  retention_expires_at: string;
}

export const alertRulesApi = {
  list: () => api.get<{ alert_rules: AlertRule[] }>("/admin/alert-rules"),
  create: (data: {
    name: string;
    condition: any;
    severity: string;
    quiet_hours_start?: string;
    quiet_hours_end?: string;
    enabled: boolean;
  }) => api.post<{ alert_rule: AlertRule }>("/admin/alert-rules", data),
  update: (id: string, data: Partial<AlertRule>) =>
    api.patch<{ alert_rule: AlertRule }>(`/admin/alert-rules/${id}`, data),
};

export interface OnCallSchedule {
  id: string;
  user_id: string;
  tier: number;
  start_time: string;
  end_time: string;
  created_at: string;
}

export const onCallApi = {
  list: () => api.get<{ on_call_schedules: OnCallSchedule[] }>("/admin/on-call"),
  create: (data: { user_id: string; tier: number; start_time: string; end_time: string }) =>
    api.post<{ on_call_schedule: OnCallSchedule }>("/admin/on-call", data),
};

export const alertsApi = {
  list: () => api.get<{ alerts: Alert[] }>("/admin/alerts"),
  get: (id: string) =>
    api.get<{ alert: Alert; assignments: AlertAssignment[] }>(
      `/admin/alerts/${id}`,
    ),
  assign: (id: string, assigneeId: string) =>
    api.post<{ assignment: AlertAssignment }>(`/admin/alerts/${id}/assign`, {
      assignee_id: assigneeId,
    }),
  acknowledge: (id: string) =>
    api.post<{ message: string }>(`/admin/alerts/${id}/acknowledge`),
};

export const workOrdersApi = {
  create: (data: { alert_id?: string; assigned_to?: string }) =>
    api.post<{ work_order: WorkOrder }>("/admin/work-orders", data),
  list: () => api.get<{ work_orders: WorkOrder[] }>("/admin/work-orders"),
  get: (id: string) =>
    api.get<{
      work_order: WorkOrder;
      events: WorkOrderEvent[];
      evidence: WOEvidence[];
    }>(`/admin/work-orders/${id}`),
  dispatch: (id: string, data?: { assigned_to?: string }) =>
    api.post<{ work_order: WorkOrder }>(
      `/admin/work-orders/${id}/dispatch`,
      data || {},
    ),
  acknowledge: (id: string) =>
    api.post<{ work_order: WorkOrder }>(
      `/admin/work-orders/${id}/acknowledge`,
    ),
  start: (id: string) =>
    api.post<{ work_order: WorkOrder }>(`/admin/work-orders/${id}/start`),
  resolve: (id: string) =>
    api.post<{ work_order: WorkOrder }>(`/admin/work-orders/${id}/resolve`),
  postIncidentReview: (id: string) =>
    api.post<{ work_order: WorkOrder }>(
      `/admin/work-orders/${id}/post-incident-review`,
    ),
  close: (id: string) =>
    api.post<{ work_order: WorkOrder }>(`/admin/work-orders/${id}/close`),
  uploadEvidence: async (id: string, file: File) => {
    const fd = new FormData();
    fd.append("file", file);
    const res = await fetch(`/api/v1/admin/work-orders/${id}/evidence`, {
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
  listEvidence: (id: string) =>
    api.get<{ evidence: WOEvidence[] }>(
      `/admin/work-orders/${id}/evidence`,
    ),
};
