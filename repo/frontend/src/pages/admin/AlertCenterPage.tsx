import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { alertsApi, workOrdersApi, onCallApi } from "../../api/alerting";
import { ApiError } from "../../api/client";

const severityColors: Record<string, string> = {
  low: "bg-gray-100 text-gray-800",
  medium: "bg-yellow-100 text-yellow-800",
  high: "bg-orange-100 text-orange-800",
  critical: "bg-red-100 text-red-800",
};

const statusColors: Record<string, string> = {
  new: "bg-blue-100 text-blue-800",
  assigned: "bg-yellow-100 text-yellow-800",
  acknowledged: "bg-green-100 text-green-800",
  resolved: "bg-gray-100 text-gray-800",
};

function AlertCenterPage() {
  const queryClient = useQueryClient();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [assigneeInput, setAssigneeInput] = useState("");
  const [showAssignFor, setShowAssignFor] = useState<string | null>(null);
  const [apiError, setApiError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["admin-alerts"],
    queryFn: alertsApi.list,
  });

  const alerts = data?.alerts ?? [];

  const onCallQuery = useQuery({
    queryKey: ["admin-on-call"],
    queryFn: onCallApi.list,
  });
  const onCallUsers = onCallQuery.data?.on_call_schedules ?? [];

  const detailQuery = useQuery({
    queryKey: ["admin-alert-detail", selectedId],
    queryFn: () => alertsApi.get(selectedId!),
    enabled: !!selectedId,
  });

  const selectedAlert = detailQuery.data?.alert ?? null;
  const selectedAssignments = detailQuery.data?.assignments ?? [];
  const selectedAssignment = selectedAssignments.length > 0 ? selectedAssignments[0] : null;

  const assignMutation = useMutation({
    mutationFn: ({ id, assigneeId }: { id: string; assigneeId: string }) =>
      alertsApi.assign(id, assigneeId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-alerts"] });
      queryClient.invalidateQueries({ queryKey: ["admin-alert-detail"] });
      setShowAssignFor(null);
      setAssigneeInput("");
      flashSuccess("Alert assigned.");
    },
    onError: handleError,
  });

  const acknowledgeMutation = useMutation({
    mutationFn: (id: string) => alertsApi.acknowledge(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-alerts"] });
      queryClient.invalidateQueries({ queryKey: ["admin-alert-detail"] });
      flashSuccess("Alert acknowledged.");
    },
    onError: handleError,
  });

  const createWOMutation = useMutation({
    mutationFn: (alertId: string) => workOrdersApi.create({ alert_id: alertId }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-alerts"] });
      flashSuccess("Work order created.");
    },
    onError: handleError,
  });

  function handleError(err: Error) {
    if (err instanceof ApiError) {
      setApiError(err.message);
    } else {
      setApiError(err.message);
    }
  }

  function flashSuccess(msg: string) {
    setSuccessMsg(msg);
    setTimeout(() => setSuccessMsg(null), 3000);
  }

  function handleAssign(alertId: string) {
    if (!assigneeInput.trim()) return;
    assignMutation.mutate({ id: alertId, assigneeId: assigneeInput.trim() });
  }

  return (
    <div>
      <h1 className="mb-6 text-2xl font-bold text-gray-900">Alert Center</h1>

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

      <div className="flex gap-6">
        {/* Alert list */}
        <div className="flex-1">
          {isLoading ? (
            <p className="text-gray-500">Loading...</p>
          ) : alerts.length === 0 ? (
            <div className="rounded-lg bg-white p-8 text-center shadow-sm">
              <p className="text-gray-500">No alerts.</p>
            </div>
          ) : (
            <div className="space-y-3">
              {alerts.map((alert) => (
                <div
                  key={alert.id}
                  onClick={() => setSelectedId(alert.id)}
                  className={`cursor-pointer rounded-lg bg-white p-4 shadow-sm transition hover:shadow-md ${
                    selectedId === alert.id
                      ? "ring-2 ring-blue-500"
                      : ""
                  }`}
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <span
                        className={`inline-flex rounded-full px-2 py-1 text-xs font-semibold ${severityColors[alert.severity] ?? "bg-gray-100 text-gray-800"}`}
                      >
                        {alert.severity}
                      </span>
                      <span
                        className={`inline-flex rounded-full px-2 py-1 text-xs font-semibold ${statusColors[alert.status] ?? "bg-gray-100 text-gray-800"}`}
                      >
                        {alert.status}
                      </span>
                    </div>
                    <span className="text-xs text-gray-400">
                      {new Date(alert.created_at).toLocaleString()}
                    </span>
                  </div>
                  <p className="mt-2 text-sm font-medium text-gray-900">
                    {alert.rule_name ?? `Rule ${alert.rule_id}`}
                  </p>
                  {alert.data && (
                    <p className="mt-1 text-xs text-gray-500">
                      {typeof alert.data === "string"
                        ? alert.data
                        : JSON.stringify(alert.data).slice(0, 120)}
                    </p>
                  )}

                  <div className="mt-3 flex items-center gap-2">
                    {showAssignFor === alert.id ? (
                      <div className="flex items-center gap-2">
                        <select
                          value={assigneeInput}
                          onChange={(e) => setAssigneeInput(e.target.value)}
                          onClick={(e) => e.stopPropagation()}
                          className="rounded-md border border-gray-300 px-2 py-1 text-xs focus:border-blue-500 focus:outline-none"
                          data-testid="assignee-select"
                        >
                          <option value="">Select on-call user</option>
                          {onCallUsers.length === 0 && (
                            <option value="" disabled>No active on-call users</option>
                          )}
                          {onCallUsers.map((oc) => (
                            <option key={oc.id} value={oc.user_id}>
                              T{oc.tier} — {oc.user_id.slice(0, 8)}...
                            </option>
                          ))}
                        </select>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            handleAssign(alert.id);
                          }}
                          className="rounded bg-blue-600 px-2 py-1 text-xs text-white hover:bg-blue-700"
                        >
                          Confirm Assign
                        </button>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            setShowAssignFor(null);
                          }}
                          className="text-xs text-gray-500 hover:text-gray-700"
                        >
                          Cancel
                        </button>
                      </div>
                    ) : (
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          setShowAssignFor(alert.id);
                          setAssigneeInput("");
                        }}
                        className="rounded bg-blue-50 px-2 py-1 text-xs font-medium text-blue-600 hover:bg-blue-100"
                      >
                        Assign
                      </button>
                    )}
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        acknowledgeMutation.mutate(alert.id);
                      }}
                      className="rounded bg-green-50 px-2 py-1 text-xs font-medium text-green-600 hover:bg-green-100"
                    >
                      Acknowledge
                    </button>
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        createWOMutation.mutate(alert.id);
                      }}
                      className="rounded bg-purple-50 px-2 py-1 text-xs font-medium text-purple-600 hover:bg-purple-100"
                    >
                      Create Work Order
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Detail panel */}
        {selectedId && selectedAlert && (
          <div className="w-96 shrink-0 rounded-lg bg-white p-6 shadow-sm">
            <h2 className="text-lg font-semibold text-gray-900">
              Alert Detail
            </h2>
            <div className="mt-4 space-y-3">
              <div>
                <span className="text-xs font-medium text-gray-500">ID</span>
                <p className="text-sm text-gray-900">{selectedAlert.id}</p>
              </div>
              <div>
                <span className="text-xs font-medium text-gray-500">
                  Severity
                </span>
                <p>
                  <span
                    className={`inline-flex rounded-full px-2 py-1 text-xs font-semibold ${severityColors[selectedAlert.severity] ?? "bg-gray-100 text-gray-800"}`}
                  >
                    {selectedAlert.severity}
                  </span>
                </p>
              </div>
              <div>
                <span className="text-xs font-medium text-gray-500">
                  Status
                </span>
                <p>
                  <span
                    className={`inline-flex rounded-full px-2 py-1 text-xs font-semibold ${statusColors[selectedAlert.status] ?? "bg-gray-100 text-gray-800"}`}
                  >
                    {selectedAlert.status}
                  </span>
                </p>
              </div>
              <div>
                <span className="text-xs font-medium text-gray-500">
                  Rule
                </span>
                <p className="text-sm text-gray-900">
                  {selectedAlert.rule_name ?? selectedAlert.rule_id}
                </p>
              </div>
              <div>
                <span className="text-xs font-medium text-gray-500">
                  Created
                </span>
                <p className="text-sm text-gray-900">
                  {new Date(selectedAlert.created_at).toLocaleString()}
                </p>
              </div>
              {selectedAlert.resolved_at && (
                <div>
                  <span className="text-xs font-medium text-gray-500">
                    Resolved
                  </span>
                  <p className="text-sm text-gray-900">
                    {new Date(selectedAlert.resolved_at).toLocaleString()}
                  </p>
                </div>
              )}
              {selectedAlert.data && (
                <div>
                  <span className="text-xs font-medium text-gray-500">
                    Data
                  </span>
                  <pre className="mt-1 rounded bg-gray-50 p-2 text-xs text-gray-700">
                    {JSON.stringify(selectedAlert.data, null, 2)}
                  </pre>
                </div>
              )}
              {selectedAssignment && (
                <div>
                  <span className="text-xs font-medium text-gray-500">
                    Assignment
                  </span>
                  <p className="text-sm text-gray-900">
                    Assignee: {selectedAssignment.assignee_id}
                  </p>
                  <p className="text-xs text-gray-500">
                    Assigned:{" "}
                    {new Date(selectedAssignment.assigned_at).toLocaleString()}
                  </p>
                  {selectedAssignment.acknowledged_at && (
                    <p className="text-xs text-gray-500">
                      Acknowledged:{" "}
                      {new Date(
                        selectedAssignment.acknowledged_at,
                      ).toLocaleString()}
                    </p>
                  )}
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export default AlertCenterPage;
