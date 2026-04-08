import { useState, useEffect } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { useQuery, useMutation } from "@tanstack/react-query";
import { catalogApi, providerApi } from "../../api/catalog";
import { ApiError } from "../../api/client";

interface ServiceForm {
  title: string;
  description: string;
  category_id: string;
  price: string;
  tag_ids: string[];
  status: string;
}

const emptyForm: ServiceForm = {
  title: "",
  description: "",
  category_id: "",
  price: "",
  tag_ids: [],
  status: "active",
};

function ServiceFormPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const isEdit = Boolean(id);

  const [form, setForm] = useState<ServiceForm>(emptyForm);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [apiError, setApiError] = useState<string | null>(null);

  const { data: categoriesData } = useQuery({
    queryKey: ["catalog-categories"],
    queryFn: catalogApi.listCategories,
  });

  const { data: tagsData } = useQuery({
    queryKey: ["catalog-tags"],
    queryFn: catalogApi.listTags,
  });

  const { data: serviceData } = useQuery({
    queryKey: ["provider-service", id],
    queryFn: () => providerApi.getService(id!),
    enabled: isEdit,
  });

  const categories = categoriesData?.categories ?? [];
  const tags = tagsData?.tags ?? [];

  useEffect(() => {
    if (serviceData?.service) {
      const svc = serviceData.service;
      setForm({
        title: svc.title,
        description: svc.description ?? "",
        category_id: svc.category?.id ?? "",
        price: String(svc.price_cents / 100),
        tag_ids: svc.tags.map((t) => t.id),
        status: svc.status,
      });
    }
  }, [serviceData]);

  const createMutation = useMutation({
    mutationFn: providerApi.createService,
    onSuccess: () => navigate("/provider/services"),
    onError: handleError,
  });

  const updateMutation = useMutation({
    mutationFn: (data: Parameters<typeof providerApi.updateService>[1]) =>
      providerApi.updateService(id!, data),
    onSuccess: () => navigate("/provider/services"),
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

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setFieldErrors({});
    setApiError(null);

    const priceCents = Math.round(parseFloat(form.price) * 100);

    const payload = {
      title: form.title,
      description: form.description || undefined,
      category_id: form.category_id,
      price_cents: priceCents,
      tag_ids: form.tag_ids,
      status: form.status,
    };

    if (isEdit) {
      updateMutation.mutate(payload);
    } else {
      createMutation.mutate(payload);
    }
  }

  function toggleTag(tagId: string) {
    setForm((prev) => ({
      ...prev,
      tag_ids: prev.tag_ids.includes(tagId)
        ? prev.tag_ids.filter((t) => t !== tagId)
        : [...prev.tag_ids, tagId],
    }));
  }

  const isMutating = createMutation.isPending || updateMutation.isPending;

  return (
    <div className="mx-auto max-w-2xl">
      <h1 className="mb-6 text-2xl font-bold text-gray-900">
        {isEdit ? "Edit Service" : "New Service"}
      </h1>

      {apiError && (
        <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
          {apiError}
        </div>
      )}

      <div className="rounded-lg bg-white p-6 shadow-sm">
        <form onSubmit={handleSubmit} className="space-y-5">
          <div>
            <label htmlFor="svc-title" className="block text-sm font-medium text-gray-700">
              Title <span className="text-red-500">*</span>
            </label>
            <input
              id="svc-title"
              type="text"
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
              className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              required
            />
            {fieldErrors.title && <p className="mt-1 text-sm text-red-600">{fieldErrors.title}</p>}
          </div>

          <div>
            <label htmlFor="svc-desc" className="block text-sm font-medium text-gray-700">
              Description
            </label>
            <textarea
              id="svc-desc"
              rows={3}
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
            {fieldErrors.description && <p className="mt-1 text-sm text-red-600">{fieldErrors.description}</p>}
          </div>

          <div>
            <label htmlFor="svc-category" className="block text-sm font-medium text-gray-700">
              Category <span className="text-red-500">*</span>
            </label>
            <select
              id="svc-category"
              value={form.category_id}
              onChange={(e) => setForm({ ...form, category_id: e.target.value })}
              className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              required
            >
              <option value="">Select a category</option>
              {categories.map((cat) => (
                <option key={cat.id} value={cat.id}>
                  {cat.name}
                </option>
              ))}
            </select>
            {fieldErrors.category_id && <p className="mt-1 text-sm text-red-600">{fieldErrors.category_id}</p>}
          </div>

          <div>
            <label htmlFor="svc-price" className="block text-sm font-medium text-gray-700">
              Price ($) <span className="text-red-500">*</span>
            </label>
            <input
              id="svc-price"
              type="number"
              step="0.01"
              min="0"
              value={form.price}
              onChange={(e) => setForm({ ...form, price: e.target.value })}
              className="mt-1 block w-48 rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              required
            />
            {fieldErrors.price_cents && <p className="mt-1 text-sm text-red-600">{fieldErrors.price_cents}</p>}
          </div>

          <div>
            <span className="block text-sm font-medium text-gray-700">Tags</span>
            {tags.length === 0 ? (
              <p className="mt-1 text-sm text-gray-400">No tags available</p>
            ) : (
              <div className="mt-2 flex flex-wrap gap-3">
                {tags.map((tag) => (
                  <label key={tag.id} className="flex items-center gap-1.5 text-sm text-gray-700">
                    <input
                      type="checkbox"
                      checked={form.tag_ids.includes(tag.id)}
                      onChange={() => toggleTag(tag.id)}
                      className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                    />
                    {tag.name}
                  </label>
                ))}
              </div>
            )}
          </div>

          <div>
            <label htmlFor="svc-status" className="block text-sm font-medium text-gray-700">
              Status
            </label>
            <select
              id="svc-status"
              value={form.status}
              onChange={(e) => setForm({ ...form, status: e.target.value })}
              className="mt-1 block w-48 rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            >
              <option value="active">Active</option>
              <option value="inactive">Inactive</option>
            </select>
          </div>

          <div className="flex gap-3 border-t pt-5">
            <button
              type="submit"
              disabled={isMutating}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {isMutating ? "Saving..." : "Save"}
            </button>
            <Link
              to="/provider/services"
              className="rounded-md bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200"
            >
              Cancel
            </Link>
          </div>
        </form>
      </div>

      {isEdit && (
        <div className="mt-4">
          <Link
            to={`/provider/services/${id}/availability`}
            className="text-sm font-medium text-blue-600 hover:text-blue-800"
          >
            Manage Availability
          </Link>
        </div>
      )}
    </div>
  );
}

export default ServiceFormPage;
