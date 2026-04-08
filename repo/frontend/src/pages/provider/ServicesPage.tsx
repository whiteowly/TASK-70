import { Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { providerApi } from "../../api/catalog";

function ServicesPage() {
  const queryClient = useQueryClient();

  const { data, isLoading, error } = useQuery({
    queryKey: ["provider-services"],
    queryFn: providerApi.listServices,
  });

  const services = data?.services ?? [];

  const deleteMutation = useMutation({
    mutationFn: (id: string) => providerApi.deleteService(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["provider-services"] });
    },
  });

  function handleDelete(id: string, title: string) {
    if (window.confirm(`Delete service "${title}"? This cannot be undone.`)) {
      deleteMutation.mutate(id);
    }
  }

  function formatPrice(cents: number): string {
    return `$${(cents / 100).toFixed(2)}`;
  }

  function statusBadge(status: string) {
    const isActive = status === "active";
    return (
      <span
        className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${
          isActive
            ? "bg-green-100 text-green-800"
            : "bg-gray-100 text-gray-800"
        }`}
      >
        {status}
      </span>
    );
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">My Services</h1>
        <Link
          to="/provider/services/new"
          className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
        >
          New Service
        </Link>
      </div>

      {error && (
        <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
          {(error as Error).message}
        </div>
      )}

      {isLoading ? (
        <p className="text-gray-500">Loading...</p>
      ) : services.length === 0 ? (
        <div className="rounded-lg bg-white p-8 text-center shadow-sm">
          <p className="text-gray-500">No services yet. Create your first one.</p>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {services.map((svc) => (
            <div
              key={svc.id}
              className="rounded-lg bg-white p-5 shadow-sm"
            >
              <div className="mb-2 flex items-start justify-between">
                <h3 className="text-base font-semibold text-gray-900">
                  {svc.title}
                </h3>
                {statusBadge(svc.status)}
              </div>
              <p className="mb-1 text-sm text-gray-500">
                {svc.category?.name ?? "Uncategorized"}
              </p>
              <p className="mb-3 text-lg font-bold text-gray-900">
                {formatPrice(svc.price_cents)}
              </p>
              {svc.tags.length > 0 && (
                <div className="mb-3 flex flex-wrap gap-1">
                  {svc.tags.map((tag) => (
                    <span
                      key={tag.id}
                      className="inline-block rounded bg-blue-50 px-2 py-0.5 text-xs text-blue-700"
                    >
                      {tag.name}
                    </span>
                  ))}
                </div>
              )}
              <div className="flex gap-3 border-t pt-3">
                <Link
                  to={`/provider/services/${svc.id}/edit`}
                  className="text-sm font-medium text-blue-600 hover:text-blue-800"
                >
                  Edit
                </Link>
                <button
                  onClick={() => handleDelete(svc.id, svc.title)}
                  className="text-sm font-medium text-red-600 hover:text-red-800"
                >
                  Delete
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export default ServicesPage;
