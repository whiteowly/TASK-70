import { Link } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { catalogApi, customerApi } from "../../api/catalog";
import { useState, useEffect, useCallback } from "react";

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

function FavoritesPage() {
  const queryClient = useQueryClient();
  const [removingIds, setRemovingIds] = useState<Set<string>>(new Set());

  const {
    data: favData,
    isLoading: favsLoading,
    error: favsError,
  } = useQuery({
    queryKey: ["customer-favorites"],
    queryFn: customerApi.getFavorites,
  });

  const favorites = favData?.favorites ?? [];
  const serviceIds = favorites.map((f) => f.service_id);

  // Load each service individually
  const [services, setServices] = useState<
    { id: string; title: string; price_cents: number; rating_avg: string; provider: { id: string; business_name: string }; category: { id: string; name: string } | null; tags: { id: string; name: string }[]; description: string | null }[]
  >([]);
  const [servicesLoading, setServicesLoading] = useState(false);

  useEffect(() => {
    if (serviceIds.length === 0) {
      setServices([]);
      return;
    }
    setServicesLoading(true);
    Promise.all(serviceIds.map((id) => catalogApi.getService(id)))
      .then((results) => {
        const svcs = results
          .map((r) => r.service)
          .filter((s): s is NonNullable<typeof s> => s != null);
        setServices(svcs);
      })
      .catch(() => {
        setServices([]);
      })
      .finally(() => setServicesLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [favData]);

  const handleRemove = useCallback(
    async (serviceId: string) => {
      setRemovingIds((prev) => new Set(prev).add(serviceId));
      try {
        await customerApi.removeFavorite(serviceId);
        queryClient.invalidateQueries({ queryKey: ["customer-favorites"] });
      } catch {
        // ignore
      } finally {
        setRemovingIds((prev) => {
          const next = new Set(prev);
          next.delete(serviceId);
          return next;
        });
      }
    },
    [queryClient],
  );

  const isLoading = favsLoading || servicesLoading;

  return (
    <div>
      <h1 className="mb-6 text-2xl font-bold text-gray-900">My Favorites</h1>

      {favsError && (
        <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">
          {(favsError as Error).message}
        </div>
      )}

      {isLoading ? (
        <p className="text-gray-500">Loading...</p>
      ) : services.length === 0 ? (
        <div className="rounded-lg bg-white p-8 text-center shadow-sm">
          <p className="text-gray-500">No favorites yet.</p>
          <Link
            to="/customer/catalog"
            className="mt-3 inline-block text-sm text-blue-600 hover:text-blue-800"
          >
            Browse services
          </Link>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {services.map((svc) => (
            <div
              key={svc.id}
              className="flex flex-col rounded-lg bg-white p-5 shadow-sm"
            >
              <Link to={`/customer/catalog/${svc.id}`} className="flex-1">
                <h3 className="mb-1 text-base font-semibold text-gray-900">
                  {svc.title}
                </h3>
                <p className="mb-1 text-sm text-gray-500">
                  {svc.provider.business_name}
                </p>
                <p className="mb-1 text-sm text-gray-500">
                  {svc.category?.name ?? "Uncategorized"}
                </p>
                <p className="mb-2 text-lg font-bold text-gray-900">
                  {formatPrice(svc.price_cents)}
                </p>
                <div className="mb-2 text-sm text-yellow-600">
                  Rating: {svc.rating_avg}
                </div>
                {svc.tags.length > 0 && (
                  <div className="flex flex-wrap gap-1">
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
              </Link>
              <div className="mt-3 border-t border-gray-100 pt-3">
                <button
                  onClick={() => handleRemove(svc.id)}
                  disabled={removingIds.has(svc.id)}
                  className="rounded-md bg-red-50 px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-100 disabled:opacity-50"
                >
                  {removingIds.has(svc.id) ? "Removing..." : "Remove"}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export default FavoritesPage;
