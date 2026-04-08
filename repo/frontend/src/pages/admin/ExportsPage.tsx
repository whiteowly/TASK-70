import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { exportsApi } from "../../api/operations";
import type { ExportJob } from "../../api/operations";

function statusBadge(status: string) {
  const colors: Record<string, string> = {
    pending: "bg-yellow-100 text-yellow-800",
    completed: "bg-green-100 text-green-800",
    failed: "bg-red-100 text-red-800",
  };
  return (
    <span
      className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-medium ${colors[status] ?? "bg-gray-100 text-gray-800"}`}
    >
      {status}
    </span>
  );
}

function ExportsPage() {
  const queryClient = useQueryClient();
  const [exportType, setExportType] = useState("user_growth");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [success, setSuccess] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["admin-exports"],
    queryFn: () => exportsApi.list(),
  });

  const exports: ExportJob[] = data?.exports ?? [];

  const createMutation = useMutation({
    mutationFn: () =>
      exportsApi.create({
        export_type: exportType,
        from: from || undefined,
        to: to || undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-exports"] });
      setSuccess("Export requested successfully.");
      setError(null);
      setTimeout(() => setSuccess(null), 3000);
    },
    onError: (err: unknown) => {
      setSuccess(null);
      setError(err instanceof Error ? err.message : "Failed to create export");
    },
  });

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">Exports</h1>
      <p className="mt-1 text-gray-600">Request and download data exports.</p>

      {error && (
        <div className="mt-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
          {error}
        </div>
      )}
      {success && (
        <div className="mt-4 rounded-md bg-green-50 p-3 text-sm text-green-700">
          {success}
        </div>
      )}

      {/* Request Export */}
      <div className="mt-6 rounded-lg bg-white p-6 shadow">
        <h2 className="text-lg font-semibold text-gray-900">Request Export</h2>
        <div className="mt-4 flex flex-wrap items-end gap-4">
          <div>
            <label className="block text-xs text-gray-500">Export Type</label>
            <select
              value={exportType}
              onChange={(e) => setExportType(e.target.value)}
              className="mt-1 rounded border border-gray-300 px-3 py-1.5 text-sm"
            >
              <option value="user_growth">User Growth</option>
              <option value="conversion">Conversion</option>
              <option value="provider_utilization">
                Provider Utilization
              </option>
            </select>
          </div>
          <div>
            <label className="block text-xs text-gray-500">From</label>
            <input
              type="date"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
              className="mt-1 rounded border border-gray-300 px-3 py-1.5 text-sm"
            />
          </div>
          <div>
            <label className="block text-xs text-gray-500">To</label>
            <input
              type="date"
              value={to}
              onChange={(e) => setTo(e.target.value)}
              className="mt-1 rounded border border-gray-300 px-3 py-1.5 text-sm"
            />
          </div>
          <button
            onClick={() => createMutation.mutate()}
            disabled={createMutation.isPending}
            className="rounded bg-blue-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {createMutation.isPending ? "Requesting..." : "Request Export"}
          </button>
        </div>
      </div>

      {/* Export list */}
      <div className="mt-8">
        <h2 className="text-lg font-semibold text-gray-900">Export History</h2>
        <div className="mt-4">
          {isLoading ? (
            <p className="text-sm text-gray-500">Loading exports...</p>
          ) : exports.length === 0 ? (
            <p className="text-sm text-gray-500">No exports yet.</p>
          ) : (
            <div className="overflow-hidden rounded-lg bg-white shadow">
              <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                  <tr>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">
                      Type
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">
                      Status
                    </th>
                    <th className="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">
                      Created
                    </th>
                    <th className="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200">
                  {exports.map((exp) => (
                    <tr key={exp.id}>
                      <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-900">
                        {exp.export_type}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 text-sm">
                        {statusBadge(exp.status)}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 text-sm text-gray-500">
                        {new Date(exp.created_at).toLocaleDateString()}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3 text-right text-sm">
                        {exp.status === "completed" ? (
                          <a
                            href={exportsApi.downloadUrl(exp.id)}
                            className="text-blue-600 hover:text-blue-800"
                          >
                            Download
                          </a>
                        ) : (
                          <span className="text-gray-400">--</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

export default ExportsPage;
