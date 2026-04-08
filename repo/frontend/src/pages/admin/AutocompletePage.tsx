import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { adminApi } from "../../api/catalog";
import { ApiError } from "../../api/client";

interface AutocompleteForm {
  term: string;
  weight: string;
}

const emptyForm: AutocompleteForm = { term: "", weight: "1" };

function AutocompletePage() {
  const queryClient = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<AutocompleteForm>(emptyForm);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [apiError, setApiError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["admin-autocomplete"],
    queryFn: adminApi.listAutocomplete,
  });

  const terms = data?.terms ?? [];

  const createMutation = useMutation({
    mutationFn: (d: { term: string; weight: number }) =>
      adminApi.createAutocomplete(d),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-autocomplete"] });
      resetForm();
      flashSuccess("Autocomplete term created.");
    },
    onError: handleError,
  });

  const updateMutation = useMutation({
    mutationFn: ({
      id,
      data: d,
    }: {
      id: string;
      data: { term?: string; weight?: number };
    }) => adminApi.updateAutocomplete(id, d),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin-autocomplete"] });
      resetForm();
      flashSuccess("Autocomplete term updated.");
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

    const payload = { term: form.term, weight: Number(form.weight) || 1 };

    if (editingId) {
      updateMutation.mutate({ id: editingId, data: payload });
    } else {
      createMutation.mutate(payload);
    }
  }

  function startEdit(t: { id: string; term: string; weight: number }) {
    setEditingId(t.id);
    setShowForm(true);
    setForm({ term: t.term, weight: String(t.weight) });
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
        <h1 className="text-2xl font-bold text-gray-900">
          Autocomplete Terms
        </h1>
        {!showForm && (
          <button
            onClick={() => {
              setShowForm(true);
              setEditingId(null);
              setForm(emptyForm);
            }}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          >
            Add Term
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
            {editingId ? "Edit Term" : "New Term"}
          </h2>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label
                htmlFor="ac-term"
                className="block text-sm font-medium text-gray-700"
              >
                Term <span className="text-red-500">*</span>
              </label>
              <input
                id="ac-term"
                type="text"
                value={form.term}
                onChange={(e) => setForm({ ...form, term: e.target.value })}
                className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                required
              />
              {fieldErrors.term && (
                <p className="mt-1 text-sm text-red-600">{fieldErrors.term}</p>
              )}
            </div>
            <div>
              <label
                htmlFor="ac-weight"
                className="block text-sm font-medium text-gray-700"
              >
                Weight <span className="text-red-500">*</span>
              </label>
              <input
                id="ac-weight"
                type="number"
                min="0"
                value={form.weight}
                onChange={(e) => setForm({ ...form, weight: e.target.value })}
                className="mt-1 block w-32 rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                required
              />
              {fieldErrors.weight && (
                <p className="mt-1 text-sm text-red-600">
                  {fieldErrors.weight}
                </p>
              )}
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
      ) : terms.length === 0 ? (
        <div className="rounded-lg bg-white p-8 text-center shadow-sm">
          <p className="text-gray-500">
            No autocomplete terms yet. Create the first one.
          </p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg bg-white shadow-sm">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Term
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500">
                  Weight
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
              {terms.map((t) => (
                <tr key={t.id} className="hover:bg-gray-50">
                  <td className="whitespace-nowrap px-6 py-4 text-sm font-medium text-gray-900">
                    {t.term}
                  </td>
                  <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500">
                    {t.weight}
                  </td>
                  <td className="whitespace-nowrap px-6 py-4 text-sm text-gray-500">
                    {formatDate(t.created_at)}
                  </td>
                  <td className="whitespace-nowrap px-6 py-4 text-right text-sm">
                    <button
                      onClick={() => startEdit(t)}
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

export default AutocompletePage;
