import { useState, useRef, useCallback } from "react";
import { useParams, Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { workOrdersApi } from "../../api/alerting";
import { ApiError } from "../../api/client";

const statusColors: Record<string, string> = {
  new: "bg-gray-100 text-gray-800",
  dispatched: "bg-blue-100 text-blue-800",
  acknowledged: "bg-yellow-100 text-yellow-800",
  in_progress: "bg-orange-100 text-orange-800",
  resolved: "bg-green-100 text-green-800",
  post_incident_review: "bg-purple-100 text-purple-800",
  closed: "bg-gray-100 text-gray-800",
};

const nextAction: Record<string, { label: string; key: string }> = {
  new: { label: "Dispatch", key: "dispatch" },
  dispatched: { label: "Acknowledge", key: "acknowledge" },
  acknowledged: { label: "Start", key: "start" },
  in_progress: { label: "Resolve", key: "resolve" },
  resolved: { label: "Post-Incident Review", key: "postIncidentReview" },
  post_incident_review: { label: "Close", key: "close" },
};

function WorkOrderDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [apiError, setApiError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["admin-work-order-detail", id],
    queryFn: () => workOrdersApi.get(id!),
    enabled: !!id,
  });

  const workOrder = data?.work_order ?? null;
  const events = data?.events ?? [];
  const evidence = data?.evidence ?? [];

  const transitionMutation = useMutation({
    mutationFn: async ({ action }: { action: string }) => {
      const api = workOrdersApi as any;
      if (action === "dispatch") return api.dispatch(id!);
      if (action === "acknowledge") return api.acknowledge(id!);
      if (action === "start") return api.start(id!);
      if (action === "resolve") return api.resolve(id!);
      if (action === "postIncidentReview") return api.postIncidentReview(id!);
      if (action === "close") return api.close(id!);
      throw new Error("Unknown action");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["admin-work-order-detail", id],
      });
      queryClient.invalidateQueries({ queryKey: ["admin-work-orders"] });
      flashSuccess("Status updated.");
    },
    onError: handleError,
  });

  const uploadMutation = useMutation({
    mutationFn: (file: File) => workOrdersApi.uploadEvidence(id!, file),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["admin-work-order-detail", id],
      });
      flashSuccess("Evidence uploaded.");
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

  const handleFileSelect = useCallback(
    (files: FileList | null) => {
      if (!files || files.length === 0) return;
      setApiError(null);
      uploadMutation.mutate(files[0]);
    },
    [uploadMutation],
  );

  if (isLoading) {
    return <p className="text-gray-500">Loading...</p>;
  }

  if (!workOrder) {
    return (
      <div className="rounded-lg bg-white p-8 text-center shadow-sm">
        <p className="text-gray-500">Work order not found.</p>
        <Link
          to="/admin/work-orders"
          className="mt-2 inline-block text-sm text-blue-600 hover:text-blue-800"
        >
          Back to Work Orders
        </Link>
      </div>
    );
  }

  const action = nextAction[workOrder.status];

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <Link
            to="/admin/work-orders"
            className="text-sm text-blue-600 hover:text-blue-800"
          >
            &larr; Back to Work Orders
          </Link>
          <h1 className="mt-2 text-2xl font-bold text-gray-900">
            Work Order Detail
          </h1>
        </div>
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

      {/* Info card */}
      <div className="mb-6 rounded-lg bg-white p-6 shadow-sm">
        <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
          <div>
            <span className="text-xs font-medium text-gray-500">ID</span>
            <p className="text-sm font-mono text-gray-900">{workOrder.id}</p>
          </div>
          <div>
            <span className="text-xs font-medium text-gray-500">Status</span>
            <p>
              <span
                className={`inline-flex rounded-full px-2 py-1 text-xs font-semibold ${statusColors[workOrder.status] ?? "bg-gray-100 text-gray-800"}`}
              >
                {workOrder.status}
              </span>
            </p>
          </div>
          <div>
            <span className="text-xs font-medium text-gray-500">
              Assigned To
            </span>
            <p className="text-sm text-gray-900">
              {workOrder.assigned_to ?? "-"}
            </p>
          </div>
          <div>
            <span className="text-xs font-medium text-gray-500">Alert</span>
            <p className="text-sm text-gray-900">
              {workOrder.alert_id ?? "-"}
            </p>
          </div>
          <div>
            <span className="text-xs font-medium text-gray-500">Created</span>
            <p className="text-sm text-gray-900">
              {new Date(workOrder.created_at).toLocaleString()}
            </p>
          </div>
          <div>
            <span className="text-xs font-medium text-gray-500">Updated</span>
            <p className="text-sm text-gray-900">
              {new Date(workOrder.updated_at).toLocaleString()}
            </p>
          </div>
        </div>

        {action && (
          <div className="mt-6">
            <button
              onClick={() =>
                transitionMutation.mutate({ action: action.key })
              }
              disabled={transitionMutation.isPending}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {transitionMutation.isPending
                ? "Processing..."
                : action.label}
            </button>
          </div>
        )}
      </div>

      {/* Timeline */}
      <div className="mb-6 rounded-lg bg-white p-6 shadow-sm">
        <h2 className="mb-4 text-lg font-semibold text-gray-900">Timeline</h2>
        {events.length === 0 ? (
          <p className="text-sm text-gray-500">No events yet.</p>
        ) : (
          <div className="space-y-3">
            {events.map((event) => (
              <div
                key={event.id}
                className="flex items-start gap-3 border-l-2 border-gray-200 pl-4"
              >
                <div className="flex-1">
                  <p className="text-sm text-gray-900">
                    {event.old_status ? (
                      <>
                        <span className="font-medium">{event.old_status}</span>
                        {" \u2192 "}
                        <span className="font-medium">{event.new_status}</span>
                      </>
                    ) : (
                      <span className="font-medium">{event.new_status}</span>
                    )}
                  </p>
                  <p className="text-xs text-gray-500">
                    {new Date(event.created_at).toLocaleString()}
                    {event.actor_id && ` by ${event.actor_id}`}
                  </p>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Evidence */}
      <div className="rounded-lg bg-white p-6 shadow-sm">
        <h2 className="mb-4 text-lg font-semibold text-gray-900">Evidence</h2>

        <div className="mb-4">
          <button
            onClick={() => fileInputRef.current?.click()}
            className="rounded-md bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200"
          >
            Upload Evidence
          </button>
          <input
            ref={fileInputRef}
            type="file"
            className="hidden"
            data-testid="evidence-file-input"
            onChange={(e) => handleFileSelect(e.target.files)}
          />
          {uploadMutation.isPending && (
            <span className="ml-2 text-sm text-blue-600">Uploading...</span>
          )}
        </div>

        {evidence.length === 0 ? (
          <p className="text-sm text-gray-500">No evidence uploaded yet.</p>
        ) : (
          <div className="overflow-hidden rounded-lg border border-gray-200">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">
                    File
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">
                    Uploaded By
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">
                    Date
                  </th>
                  <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">
                    Retention Expires
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {evidence.map((ev) => (
                  <tr key={ev.id}>
                    <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-900">
                      {ev.file_path.split("/").pop() ?? ev.file_path}
                    </td>
                    <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-500">
                      {ev.uploaded_by ?? "-"}
                    </td>
                    <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-500">
                      {new Date(ev.created_at).toLocaleDateString()}
                    </td>
                    <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-500">
                      {new Date(ev.retention_expires_at).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

export default WorkOrderDetailPage;
