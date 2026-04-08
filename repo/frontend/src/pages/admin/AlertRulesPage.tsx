import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { alertRulesApi, AlertRule } from "../../api/alerting";
import { ApiError } from "../../api/client";

const severityColors: Record<string, string> = {
  low: "bg-gray-100 text-gray-800",
  medium: "bg-yellow-100 text-yellow-800",
  high: "bg-orange-100 text-orange-800",
  critical: "bg-red-100 text-red-800",
};

interface RuleForm {
  name: string;
  metric: string;
  threshold: string;
  severity: string;
  quiet_hours_start: string;
  quiet_hours_end: string;
  enabled: boolean;
}

const emptyForm: RuleForm = {
  name: "",
  metric: "unresolved_interests",
  threshold: "5",
  severity: "medium",
  quiet_hours_start: "",
  quiet_hours_end: "",
  enabled: true,
};

function AlertRulesPage() {
  const queryClient = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<RuleForm>(emptyForm);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [apiError, setApiError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["admin-alert-rules"],
    queryFn: alertRulesApi.list,
  });

  const rules = data?.alert_rules ?? [];

  const createMutation = useMutation({
    mutationFn: (d: {
      name: string;
      condition: any;
      severity: string;
      quiet_hours_start?: string;
      quiet_hours_end?: string;
      enabled: boolean;
    }) => alertRulesApi.create(d),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-alert-rules"] });
      setShowForm(false);
      setForm(emptyForm);
      setFieldErrors({});
      setApiError(null);
      flashSuccess("Alert rule created.");
    },
    onError: handleError,
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data: d }: { id: string; data: Partial<AlertRule> }) =>
      alertRulesApi.update(id, d),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-alert-rules"] });
      setEditingId(null);
      setShowForm(false);
      setForm(emptyForm);
      setFieldErrors({});
      setApiError(null);
      flashSuccess("Alert rule updated.");
    },
    onError: handleError,
  });

  function handleError(err: Error) {
    if (err instanceof ApiError) {
      setApiError(err.message);
      const fe: Record<string, string> = {};
      for (const [field, msgs] of Object.entries(err.fieldErrors)) {
        fe[field] = msgs.join(" ");
      }
      setFieldErrors(fe);
    } else {
      setApiError(err.message);
    }
  }

  function flashSuccess(msg: string) {
    setSuccessMsg(msg);
    setTimeout(() => setSuccessMsg(null), 3000);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setFieldErrors({});
    setApiError(null);

    const payload = {
      name: form.name,
      condition: { metric: form.metric, threshold: Number(form.threshold) },
      severity: form.severity,
      quiet_hours_start: form.quiet_hours_start || undefined,
      quiet_hours_end: form.quiet_hours_end || undefined,
      enabled: form.enabled,
    };

    if (editingId) {
      updateMutation.mutate({ id: editingId, data: payload });
    } else {
      createMutation.mutate(payload);
    }
  }

  function startEdit(rule: AlertRule) {
    setEditingId(rule.id);
    setShowForm(true);
    setForm({
      name: rule.name,
      metric: rule.condition?.metric ?? "unresolved_interests",
      threshold: String(rule.condition?.threshold ?? 5),
      severity: rule.severity,
      quiet_hours_start: rule.quiet_hours_start ?? "",
      quiet_hours_end: rule.quiet_hours_end ?? "",
      enabled: rule.enabled,
    });
    setFieldErrors({});
    setApiError(null);
  }

  function handleToggleEnabled(rule: AlertRule) {
    updateMutation.mutate({
      id: rule.id,
      data: { enabled: !rule.enabled },
    });
  }

  function cancelForm() {
    setShowForm(false);
    setEditingId(null);
    setForm(emptyForm);
    setFieldErrors({});
    setApiError(null);
  }

  const isMutating = createMutation.isPending || updateMutation.isPending;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Alert Rules</h1>
        {!showForm && (
          <button
            onClick={() => {
              setShowForm(true);
              setEditingId(null);
              setForm(emptyForm);
            }}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          >
            Add Rule
          </button>
        )}
      </div>

      {successMsg && (
        <div className="mb-4 rounded-md bg-green-50 p-3 text-sm text-green-700">
          {successMsg}
        </div>
      )}

      {apiError && (
        <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
          {apiError}
        </div>
      )}

      {showForm && (
        <div className="mb-6 rounded-lg bg-white p-6 shadow-sm">
          <h2 className="mb-4 text-lg font-semibold text-gray-800">
            {editingId ? "Edit Rule" : "New Alert Rule"}
          </h2>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label
                htmlFor="rule-name"
                className="block text-sm font-medium text-gray-700"
              >
                Name <span className="text-red-500">*</span>
              </label>
              <input
                id="rule-name"
                type="text"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                required
              />
              {fieldErrors.name && (
                <p className="mt-1 text-sm text-red-600">{fieldErrors.name}</p>
              )}
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label
                  htmlFor="rule-metric"
                  className="block text-sm font-medium text-gray-700"
                >
                  Metric
                </label>
                <select
                  id="rule-metric"
                  value={form.metric}
                  onChange={(e) => setForm({ ...form, metric: e.target.value })}
                  className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                >
                  <option value="unresolved_interests">Unresolved Interests (3+ days old)</option>
                  <option value="low_provider_utilization">Low Provider Utilization (0 active services)</option>
                  <option value="overdue_work_orders">Overdue Work Orders (24+ hours)</option>
                </select>
              </div>
              <div>
                <label
                  htmlFor="rule-threshold"
                  className="block text-sm font-medium text-gray-700"
                >
                  Threshold
                </label>
                <input
                  id="rule-threshold"
                  type="number"
                  value={form.threshold}
                  onChange={(e) =>
                    setForm({ ...form, threshold: e.target.value })
                  }
                  className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                  required
                />
              </div>
            </div>

            <div>
              <label
                htmlFor="rule-severity"
                className="block text-sm font-medium text-gray-700"
              >
                Severity
              </label>
              <select
                id="rule-severity"
                value={form.severity}
                onChange={(e) => setForm({ ...form, severity: e.target.value })}
                className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              >
                <option value="low">Low</option>
                <option value="medium">Medium</option>
                <option value="high">High</option>
                <option value="critical">Critical</option>
              </select>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label
                  htmlFor="rule-quiet-start"
                  className="block text-sm font-medium text-gray-700"
                >
                  Quiet Hours Start
                </label>
                <input
                  id="rule-quiet-start"
                  type="time"
                  value={form.quiet_hours_start}
                  onChange={(e) =>
                    setForm({ ...form, quiet_hours_start: e.target.value })
                  }
                  className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
              <div>
                <label
                  htmlFor="rule-quiet-end"
                  className="block text-sm font-medium text-gray-700"
                >
                  Quiet Hours End
                </label>
                <input
                  id="rule-quiet-end"
                  type="time"
                  value={form.quiet_hours_end}
                  onChange={(e) =>
                    setForm({ ...form, quiet_hours_end: e.target.value })
                  }
                  className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                />
              </div>
            </div>

            <div className="flex items-center gap-2">
              <input
                id="rule-enabled"
                type="checkbox"
                checked={form.enabled}
                onChange={(e) =>
                  setForm({ ...form, enabled: e.target.checked })
                }
                className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <label
                htmlFor="rule-enabled"
                className="text-sm font-medium text-gray-700"
              >
                Enabled
              </label>
            </div>

            <div className="flex gap-3">
              <button
                type="submit"
                disabled={isMutating}
                className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
              >
                {isMutating ? "Saving..." : "Save"}
              </button>
              <button
                type="button"
                onClick={cancelForm}
                className="rounded-md bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200"
              >
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {isLoading ? (
        <p className="text-gray-500">Loading...</p>
      ) : rules.length === 0 ? (
        <div className="rounded-lg bg-white p-8 text-center shadow-sm">
          <p className="text-gray-500">
            No alert rules yet. Create the first one.
          </p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg bg-white shadow-sm">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Name
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Severity
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Enabled
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Quiet Hours
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Condition
                </th>
                <th className="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200">
              {rules.map((rule) => (
                <tr key={rule.id} className="hover:bg-gray-50">
                  <td className="whitespace-nowrap px-6 py-4 text-sm font-medium text-gray-900">
                    {rule.name}
                  </td>
                  <td className="whitespace-nowrap px-6 py-4 text-sm">
                    <span
                      className={`inline-flex rounded-full px-2 py-1 text-xs font-semibold ${severityColors[rule.severity] ?? "bg-gray-100 text-gray-800"}`}
                    >
                      {rule.severity}
                    </span>
                  </td>
                  <td className="whitespace-nowrap px-6 py-4 text-sm">
                    <button
                      onClick={() => handleToggleEnabled(rule)}
                      className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                        rule.enabled ? "bg-blue-600" : "bg-gray-300"
                      }`}
                      aria-label={
                        rule.enabled ? "Disable rule" : "Enable rule"
                      }
                    >
                      <span
                        className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                          rule.enabled ? "translate-x-6" : "translate-x-1"
                        }`}
                      />
                    </button>
                  </td>
                  <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500">
                    {rule.quiet_hours_start && rule.quiet_hours_end
                      ? `${rule.quiet_hours_start} - ${rule.quiet_hours_end}`
                      : "-"}
                  </td>
                  <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500">
                    {rule.condition?.metric} &gt; {rule.condition?.threshold}
                  </td>
                  <td className="whitespace-nowrap px-6 py-4 text-right text-sm">
                    <button
                      onClick={() => startEdit(rule)}
                      className="font-medium text-blue-600 hover:text-blue-800"
                    >
                      Edit
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

export default AlertRulesPage;
