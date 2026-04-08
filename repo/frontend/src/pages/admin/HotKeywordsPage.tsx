import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { adminApi } from "../../api/catalog";
import { ApiError } from "../../api/client";

interface HotKeywordForm {
  keyword: string;
  is_hot: boolean;
}

const emptyForm: HotKeywordForm = { keyword: "", is_hot: true };

function HotKeywordsPage() {
  const queryClient = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<HotKeywordForm>(emptyForm);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [apiError, setApiError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["admin-hot-keywords"],
    queryFn: adminApi.listHotKeywords,
  });

  const keywords = data?.keywords ?? [];

  const createMutation = useMutation({
    mutationFn: (d: { keyword: string; is_hot: boolean }) =>
      adminApi.createHotKeyword(d),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-hot-keywords"] });
      resetForm();
      flashSuccess("Hot keyword created.");
    },
    onError: handleError,
  });

  const updateMutation = useMutation({
    mutationFn: ({
      id,
      data: d,
    }: {
      id: string;
      data: { keyword?: string; is_hot?: boolean };
    }) => adminApi.updateHotKeyword(id, d),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-hot-keywords"] });
      resetForm();
      flashSuccess("Hot keyword updated.");
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

  function resetForm() {
    setShowForm(false);
    setEditingId(null);
    setForm(emptyForm);
    setFieldErrors({});
    setApiError(null);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setFieldErrors({});
    setApiError(null);

    if (editingId) {
      updateMutation.mutate({
        id: editingId,
        data: { keyword: form.keyword, is_hot: form.is_hot },
      });
    } else {
      createMutation.mutate({ keyword: form.keyword, is_hot: form.is_hot });
    }
  }

  function startEdit(kw: {
    id: string;
    keyword: string;
    is_hot: boolean;
  }) {
    setEditingId(kw.id);
    setShowForm(true);
    setForm({ keyword: kw.keyword, is_hot: kw.is_hot });
    setFieldErrors({});
    setApiError(null);
  }

  function formatDate(dateStr: string): string {
    try {
      return new Date(dateStr).toLocaleDateString();
    } catch {
      return dateStr;
    }
  }

  const isMutating = createMutation.isPending || updateMutation.isPending;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Hot Keywords</h1>
        {!showForm && (
          <button
            onClick={() => {
              setShowForm(true);
              setEditingId(null);
              setForm(emptyForm);
            }}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          >
            Add Keyword
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
            {editingId ? "Edit Keyword" : "New Keyword"}
          </h2>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label
                htmlFor="kw-keyword"
                className="block text-sm font-medium text-gray-700"
              >
                Keyword <span className="text-red-500">*</span>
              </label>
              <input
                id="kw-keyword"
                type="text"
                value={form.keyword}
                onChange={(e) => setForm({ ...form, keyword: e.target.value })}
                className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                required
              />
              {fieldErrors.keyword && (
                <p className="mt-1 text-sm text-red-600">
                  {fieldErrors.keyword}
                </p>
              )}
            </div>
            <div>
              <label className="flex items-center gap-2 text-sm text-gray-700">
                <input
                  type="checkbox"
                  checked={form.is_hot}
                  onChange={(e) =>
                    setForm({ ...form, is_hot: e.target.checked })
                  }
                  className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                />
                Is Hot
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
                onClick={resetForm}
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
      ) : keywords.length === 0 ? (
        <div className="rounded-lg bg-white p-8 text-center shadow-sm">
          <p className="text-gray-500">
            No hot keywords yet. Create the first one.
          </p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg bg-white shadow-sm">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Keyword
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Hot
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Created
                </th>
                <th className="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200">
              {keywords.map((kw) => (
                <tr key={kw.id} className="hover:bg-gray-50">
                  <td className="whitespace-nowrap px-6 py-4 text-sm font-medium text-gray-900">
                    {kw.keyword}
                  </td>
                  <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500">
                    <span
                      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${
                        kw.is_hot
                          ? "bg-green-100 text-green-800"
                          : "bg-gray-100 text-gray-800"
                      }`}
                    >
                      {kw.is_hot ? "Yes" : "No"}
                    </span>
                  </td>
                  <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500">
                    {formatDate(kw.created_at)}
                  </td>
                  <td className="whitespace-nowrap px-6 py-4 text-right text-sm">
                    <button
                      onClick={() => startEdit(kw)}
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

export default HotKeywordsPage;
